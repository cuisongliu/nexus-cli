package nexus2

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	nx3 "nexus-cli/internal/nexus3"
)

func TestImportBackupMaven(t *testing.T) {
	var uploaded = map[string]string{}
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		mu.Lock()
		uploaded[r.URL.Path] = string(body)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	inputDir := t.TempDir()
	assetDir := filepath.Join(inputDir, "assets", "com", "example", "app", "1.0.0")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	jarPath := filepath.Join(assetDir, "app-1.0.0.jar")
	pomPath := filepath.Join(assetDir, "app-1.0.0.pom")
	if err := os.WriteFile(jarPath, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile(jar) returned error: %v", err)
	}
	if err := os.WriteFile(pomPath, []byte("pom"), 0o644); err != nil {
		t.Fatalf("WriteFile(pom) returned error: %v", err)
	}

	manifest := nx3.BackupManifest{
		SchemaVersion: 1,
		Format:        nx3.FormatMaven,
		Repository:    "maven-releases",
		Components: []nx3.BackupComponent{
			{
				Group:   "com.example",
				Name:    "app",
				Version: "1.0.0",
				Assets: []nx3.BackupAsset{
					{
						RepositoryPath: "com/example/app/1.0.0/app-1.0.0.jar",
						File:           "assets/com/example/app/1.0.0/app-1.0.0.jar",
					},
					{
						RepositoryPath: "com/example/app/1.0.0/app-1.0.0.pom",
						File:           "assets/com/example/app/1.0.0/app-1.0.0.pom",
					},
				},
			},
		},
	}
	writeManifestFixture(t, inputDir, manifest)

	client := mustClient(t, server)
	err := ImportBackup(context.Background(), client, ImportOptions{
		Target: Target{
			BaseURL:    server.URL,
			Repository: "releases",
		},
		Format:    nx3.FormatMaven,
		InputPath: inputDir,
		Workers:   2,
	})
	if err != nil {
		t.Fatalf("ImportBackup returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if uploaded["/content/repositories/releases/com/example/app/1.0.0/app-1.0.0.jar"] != "jar" {
		t.Fatalf("jar upload missing or wrong: %v", uploaded)
	}
	if uploaded["/content/repositories/releases/com/example/app/1.0.0/app-1.0.0.pom"] != "pom" {
		t.Fatalf("pom upload missing or wrong: %v", uploaded)
	}
}

func TestImportBackupNPM(t *testing.T) {
	var requestPath string
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		requestPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	inputDir := t.TempDir()
	tarballPath := filepath.Join(inputDir, "demo-pkg-1.0.0.tgz")
	writeNPMPackageTarball(t, tarballPath, `{"name":"demo-pkg","version":"1.0.0","main":"index.js"}`, "module.exports='ok';")

	client := mustClient(t, server)
	err := ImportBackup(context.Background(), client, ImportOptions{
		Target: Target{
			BaseURL:    server.URL,
			Repository: "npm-hosted",
		},
		Format:    nx3.FormatNPM,
		InputPath: tarballPath,
		Workers:   1,
	})
	if err != nil {
		t.Fatalf("ImportBackup returned error: %v", err)
	}

	if requestPath != "/content/repositories/npm-hosted/demo-pkg" {
		t.Fatalf("requestPath = %q, want %q", requestPath, "/content/repositories/npm-hosted/demo-pkg")
	}
	if payload["name"] != "demo-pkg" {
		t.Fatalf("payload[name] = %v, want %q", payload["name"], "demo-pkg")
	}
}

func TestDownloadMavenComponent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/content/repositories/snapshots/com/example/app/1.0.0/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
<html><body>
  <a href="../">Parent Directory</a>
  <a href="app-1.0.0.jar">app-1.0.0.jar</a>
  <a href="app-1.0.0.pom">app-1.0.0.pom</a>
</body></html>`))
	})
	mux.HandleFunc("/content/repositories/snapshots/com/example/app/1.0.0/app-1.0.0.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("jar-content"))
	})
	mux.HandleFunc("/content/repositories/snapshots/com/example/app/1.0.0/app-1.0.0.pom", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<project/>"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := mustClient(t, server)
	outputDir := t.TempDir()

	err := DownloadMavenComponent(context.Background(), client, DownloadMavenComponentOptions{
		Target: Target{
			BaseURL:    server.URL,
			Repository: "snapshots",
		},
		GroupID:    "com.example",
		ArtifactID: "app",
		Version:    "1.0.0",
		OutputDir:  outputDir,
		Workers:    2,
	})
	if err != nil {
		t.Fatalf("DownloadMavenComponent returned error: %v", err)
	}

	assertFileContent(t, filepath.Join(outputDir, "assets", "com", "example", "app", "1.0.0", "app-1.0.0.jar"), "jar-content")
	assertFileContent(t, filepath.Join(outputDir, "assets", "com", "example", "app", "1.0.0", "app-1.0.0.pom"), "<project/>")
}

func TestDownloadNPMComponent(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/content/repositories/npm-hosted/demo-pkg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "name": "demo-pkg",
  "versions": {
    "1.0.0": {
      "name": "demo-pkg",
      "version": "1.0.0",
      "dist": {
        "tarball": "` + server.URL + `/content/repositories/npm-hosted/demo-pkg/-/demo-pkg-1.0.0.tgz"
      }
    }
  }
}`))
	})
	mux.HandleFunc("/content/repositories/npm-hosted/demo-pkg/-/demo-pkg-1.0.0.tgz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tgz-content"))
	})

	server = httptest.NewServer(mux)
	defer server.Close()

	client := mustClient(t, server)
	outputDir := t.TempDir()

	err := DownloadNPMComponent(context.Background(), client, DownloadNPMComponentOptions{
		Target: Target{
			BaseURL:    server.URL,
			Repository: "npm-hosted",
		},
		Name:      "demo-pkg",
		Version:   "1.0.0",
		OutputDir: outputDir,
		Workers:   1,
	})
	if err != nil {
		t.Fatalf("DownloadNPMComponent returned error: %v", err)
	}

	assertFileContent(t, filepath.Join(outputDir, "assets", "demo-pkg", "-", "demo-pkg-1.0.0.tgz"), "tgz-content")
}

func mustClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := NewClient(server.URL, "admin", "admin123", server.Client())
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	return client
}

func writeManifestFixture(t *testing.T, root string, manifest nx3.BackupManifest) {
	t.Helper()
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, nx3.ManifestFilename), append(body, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest) returned error: %v", err)
	}
}

func writeNPMPackageTarball(t *testing.T, filename, packageJSON, indexJS string) {
	t.Helper()

	file, err := os.Create(filename)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	writeTarEntry(t, tarWriter, "package/package.json", []byte(packageJSON))
	writeTarEntry(t, tarWriter, "package/index.js", []byte(indexJS))
}

func assertFileContent(t *testing.T, filename, want string) {
	t.Helper()
	body, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile(%s) returned error: %v", filename, err)
	}
	if string(body) != want {
		t.Fatalf("file %s = %q, want %q", filename, string(body), want)
	}
}

func writeTarEntry(t *testing.T, tw *tar.Writer, name string, body []byte) {
	t.Helper()
	header := &tar.Header{
		Name: name,
		Mode: 0o644,
		Size: int64(len(body)),
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("WriteHeader returned error: %v", err)
	}
	if _, err := io.Copy(tw, bytes.NewReader(body)); err != nil {
		t.Fatalf("Copy returned error: %v", err)
	}
}
