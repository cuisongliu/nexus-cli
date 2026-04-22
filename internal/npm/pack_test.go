package npm

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestPackTarballsUsesDetectedPackages(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	store := filepath.Join(root, "store")

	pkgA := filepath.Join(project, "node_modules", "a")
	pkgB := filepath.Join(project, "node_modules", "nested", "b")
	for _, dir := range []string{pkgA, pkgB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) returned error: %v", dir, err)
		}
	}

	if err := os.WriteFile(filepath.Join(pkgA, "package.json"), []byte(`{"name":"a","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(packageA) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgB, "package.json"), []byte(`{"name":"b","version":"2.0.0"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(packageB) returned error: %v", err)
	}

	var packedDirs []string
	originalRunner := npmPackRunner
	npmPackRunner = func(ctx context.Context, packDir, storePath string) (string, error) {
		packedDirs = append(packedDirs, packDir)
		filename := filepath.Join(storePath, filepath.Base(packDir)+".tgz")
		if err := os.WriteFile(filename, []byte("packed"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) returned error: %v", filename, err)
		}
		return filepath.Base(filename) + "\n", nil
	}
	defer func() {
		npmPackRunner = originalRunner
	}()

	var stdout strings.Builder
	err := PackTarballs(context.Background(), project, store, PackOptions{
		Stdout: &stdout,
		Stderr: &stdout,
	})
	if err != nil {
		t.Fatalf("PackTarballs returned error: %v", err)
	}

	slices.Sort(packedDirs)
	wantDirs := []string{pkgA, pkgB}
	if !reflect.DeepEqual(packedDirs, wantDirs) {
		t.Fatalf("packedDirs = %v, want %v", packedDirs, wantDirs)
	}

	for _, filename := range []string{"a.tgz", "b.tgz"} {
		if _, err := os.Stat(filepath.Join(store, filename)); err != nil {
			t.Fatalf("Stat(%s) returned error: %v", filename, err)
		}
	}
	if !strings.Contains(stdout.String(), "all packs completed") {
		t.Fatalf("stdout = %q, want completion message", stdout.String())
	}
}
