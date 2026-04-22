package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootUsageSections(t *testing.T) {
	usage := rootUsage()
	for _, want := range []string{
		"Local npm tools:",
		"Nexus3 commands:",
		"npm-cache",
		"npm-pkg",
		"backup-download",
		"component-upload",
	} {
		if !strings.Contains(usage, want) {
			t.Fatalf("rootUsage() missing %q\n%s", want, usage)
		}
	}
}

func TestRunNpmCacheDownloadsTarball(t *testing.T) {
	server := newTarballServer(t)
	defer server.Close()

	root := t.TempDir()
	lockFile := filepath.Join(root, "package-lock.json")
	storeDir := filepath.Join(root, "store")

	lockJSON := `{
  "packages": {
    "node_modules/demo": {
      "resolved": "` + server.URL + `/demo/-/demo-1.0.0.tgz"
    }
  }
}`
	if err := os.WriteFile(lockFile, []byte(lockJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(package-lock.json) returned error: %v", err)
	}

	if err := run([]string{"npm-cache", lockFile, storeDir}); err != nil {
		t.Fatalf("run(npm-cache) returned error: %v", err)
	}

	filename := filepath.Join(storeDir, "demo-1.0.0.tgz")
	body, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile(%s) returned error: %v", filename, err)
	}
	if string(body) != "tarball-body" {
		t.Fatalf("downloaded tarball = %q, want %q", string(body), "tarball-body")
	}
}

func TestRunNpmPkgUsage(t *testing.T) {
	err := run([]string{"npm-pkg"})
	if err == nil {
		t.Fatal("run(npm-pkg) expected error for missing args")
	}
	if !strings.Contains(err.Error(), "Usage:") {
		t.Fatalf("run(npm-pkg) error = %q, want usage", err.Error())
	}
}
