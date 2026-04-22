package nexus3

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestDownloadRepositoryBackupMaven(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/service/rest/v1/components", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("repository"); got != "maven-releases" {
			t.Fatalf("repository query = %q, want %q", got, "maven-releases")
		}

		token := r.URL.Query().Get("continuationToken")
		switch token {
		case "":
			writeJSON(t, w, map[string]any{
				"items": []any{
					map[string]any{
						"group":   "com.example",
						"name":    "app",
						"version": "1.0.0",
						"assets": []any{
							map[string]any{
								"downloadUrl": serverRelativeURL(r, "/downloads/app-1.0.0.jar"),
								"path":        "/com/example/app/1.0.0/app-1.0.0.jar",
								"maven2": map[string]any{
									"extension": "jar",
								},
							},
						},
					},
				},
				"continuationToken": "next",
			})
		case "next":
			writeJSON(t, w, map[string]any{
				"items": []any{
					map[string]any{
						"group":   "com.example",
						"name":    "app",
						"version": "1.0.0",
						"assets": []any{
							map[string]any{
								"downloadUrl": serverRelativeURL(r, "/downloads/app-1.0.0.pom"),
								"path":        "/com/example/app/1.0.0/app-1.0.0.pom",
								"maven2": map[string]any{
									"extension": "pom",
								},
							},
						},
					},
				},
				"continuationToken": nil,
			})
		default:
			t.Fatalf("unexpected continuation token %q", token)
		}
	})
	mux.HandleFunc("/downloads/app-1.0.0.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("jar-content"))
	})
	mux.HandleFunc("/downloads/app-1.0.0.pom", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<project/>"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := mustNewClient(t, server)
	outputDir := t.TempDir()

	err := DownloadRepositoryBackup(context.Background(), client, DownloadRepositoryOptions{
		Target: Target{
			BaseURL:    server.URL,
			Repository: "maven-releases",
		},
		Format:    FormatMaven,
		OutputDir: outputDir,
		Workers:   2,
	})
	if err != nil {
		t.Fatalf("DownloadRepositoryBackup returned error: %v", err)
	}

	assertFileContent(t, filepath.Join(outputDir, "assets", "com", "example", "app", "1.0.0", "app-1.0.0.jar"), "jar-content")
	assertFileContent(t, filepath.Join(outputDir, "assets", "com", "example", "app", "1.0.0", "app-1.0.0.pom"), "<project/>")

	manifest := readManifest(t, outputDir)
	if manifest.Format != FormatMaven {
		t.Fatalf("manifest.Format = %q, want %q", manifest.Format, FormatMaven)
	}
	if len(manifest.Components) != 2 {
		t.Fatalf("manifest.Components len = %d, want 2", len(manifest.Components))
	}
}

func TestDownloadNPMComponentOnlyDownloadsTarballs(t *testing.T) {
	var mu sync.Mutex
	var downloadHits []string

	mux := http.NewServeMux()
	mux.HandleFunc("/service/rest/v1/search", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"items": []any{
				map[string]any{
					"name":    "demo-pkg",
					"version": "1.2.3",
					"assets": []any{
						map[string]any{
							"downloadUrl": serverRelativeURL(r, "/downloads/demo-pkg"),
							"path":        "/demo-pkg",
						},
						map[string]any{
							"downloadUrl": serverRelativeURL(r, "/downloads/demo-pkg-1.2.3.tgz"),
							"path":        "/demo-pkg/-/demo-pkg-1.2.3.tgz",
						},
					},
				},
			},
			"continuationToken": nil,
		})
	})
	mux.HandleFunc("/downloads/demo-pkg", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		downloadHits = append(downloadHits, r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte("{}"))
	})
	mux.HandleFunc("/downloads/demo-pkg-1.2.3.tgz", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		downloadHits = append(downloadHits, r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte("tgz-content"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := mustNewClient(t, server)
	outputDir := t.TempDir()

	err := DownloadNPMComponent(context.Background(), client, DownloadNPMComponentOptions{
		Target: Target{
			BaseURL:    server.URL,
			Repository: "npm-hosted",
		},
		Name:      "demo-pkg",
		Version:   "1.2.3",
		OutputDir: outputDir,
		Workers:   2,
	})
	if err != nil {
		t.Fatalf("DownloadNPMComponent returned error: %v", err)
	}

	mu.Lock()
	gotHits := append([]string(nil), downloadHits...)
	mu.Unlock()
	wantHits := []string{"/downloads/demo-pkg-1.2.3.tgz"}
	if !reflect.DeepEqual(gotHits, wantHits) {
		t.Fatalf("download hits = %v, want %v", gotHits, wantHits)
	}

	assertFileContent(t, filepath.Join(outputDir, "assets", "demo-pkg", "-", "demo-pkg-1.2.3.tgz"), "tgz-content")
	manifest := readManifest(t, outputDir)
	if len(manifest.Components) != 1 {
		t.Fatalf("manifest.Components len = %d, want 1", len(manifest.Components))
	}
	if len(manifest.Components[0].Assets) != 1 {
		t.Fatalf("manifest.Components[0].Assets len = %d, want 1", len(manifest.Components[0].Assets))
	}
}

func TestUploadBackupMavenUsesManifestPaths(t *testing.T) {
	var mu sync.Mutex
	uploaded := make(map[string]string)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		mu.Lock()
		uploaded[r.URL.Path] = string(body)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	inputDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(inputDir, "assets", "com", "example", "app", "1.0.0"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	jarPath := filepath.Join(inputDir, "assets", "com", "example", "app", "1.0.0", "app-1.0.0.jar")
	pomPath := filepath.Join(inputDir, "assets", "com", "example", "app", "1.0.0", "app-1.0.0.pom")
	if err := os.WriteFile(jarPath, []byte("jar"), 0o644); err != nil {
		t.Fatalf("write jar: %v", err)
	}
	if err := os.WriteFile(pomPath, []byte("pom"), 0o644); err != nil {
		t.Fatalf("write pom: %v", err)
	}

	manifest := BackupManifest{
		SchemaVersion: 1,
		Format:        FormatMaven,
		Repository:    "maven-releases",
		Components: []BackupComponent{
			{
				Group:   "com.example",
				Name:    "app",
				Version: "1.0.0",
				Assets: []BackupAsset{
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

	client := mustNewClient(t, server)
	err := UploadBackup(context.Background(), client, UploadOptions{
		Target: Target{
			BaseURL:    server.URL,
			Repository: "maven-releases",
		},
		Format:    FormatMaven,
		InputPath: inputDir,
		Workers:   2,
	})
	if err != nil {
		t.Fatalf("UploadBackup returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if uploaded["/repository/maven-releases/com/example/app/1.0.0/app-1.0.0.jar"] != "jar" {
		t.Fatalf("jar upload missing or incorrect: %v", uploaded)
	}
	if uploaded["/repository/maven-releases/com/example/app/1.0.0/app-1.0.0.pom"] != "pom" {
		t.Fatalf("pom upload missing or incorrect: %v", uploaded)
	}
}

func TestUploadBackupNPMUsesComponentAPI(t *testing.T) {
	var uploadedFilename string
	var uploadedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/service/rest/v1/components" {
			t.Fatalf("path = %s, want %s", r.URL.Path, "/service/rest/v1/components")
		}
		if got := r.URL.Query().Get("repository"); got != "npm-hosted" {
			t.Fatalf("repository query = %q, want %q", got, "npm-hosted")
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("ParseMultipartForm returned error: %v", err)
		}
		file, header, err := r.FormFile("npm.asset")
		if err != nil {
			t.Fatalf("FormFile returned error: %v", err)
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		uploadedFilename = header.Filename
		uploadedBody = string(body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	inputDir := t.TempDir()
	tarballPath := filepath.Join(inputDir, "demo-pkg-1.2.3.tgz")
	if err := os.WriteFile(tarballPath, []byte("tarball"), 0o644); err != nil {
		t.Fatalf("write tarball: %v", err)
	}

	client := mustNewClient(t, server)
	err := UploadBackup(context.Background(), client, UploadOptions{
		Target: Target{
			BaseURL:    server.URL,
			Repository: "npm-hosted",
		},
		Format:    FormatNPM,
		InputPath: tarballPath,
		Workers:   1,
	})
	if err != nil {
		t.Fatalf("UploadBackup returned error: %v", err)
	}

	if uploadedFilename != "demo-pkg-1.2.3.tgz" {
		t.Fatalf("uploaded filename = %q, want %q", uploadedFilename, "demo-pkg-1.2.3.tgz")
	}
	if uploadedBody != "tarball" {
		t.Fatalf("uploaded body = %q, want %q", uploadedBody, "tarball")
	}
}

func mustNewClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := NewClient(server.URL, "user", "pass", server.Client())
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	return client
}

func serverRelativeURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
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

func readManifest(t *testing.T, root string) BackupManifest {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, ManifestFilename))
	if err != nil {
		t.Fatalf("ReadFile(manifest) returned error: %v", err)
	}
	var manifest BackupManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("Unmarshal(manifest) returned error: %v", err)
	}
	return manifest
}

func writeManifestFixture(t *testing.T, root string, manifest BackupManifest) {
	t.Helper()
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ManifestFilename), append(body, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest) returned error: %v", err)
	}
}
