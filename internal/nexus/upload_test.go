package nexus

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRelativeUploadPath(t *testing.T) {
	repoPath := filepath.Join("tmp", "repo")
	filePath := filepath.Join("tmp", "repo", "com", "example", "demo", "1.0.0", "demo-1.0.0.jar")

	rel, err := relativeUploadPath(repoPath, filePath)
	if err != nil {
		t.Fatalf("relativeUploadPath returned error: %v", err)
	}

	want := "com/example/demo/1.0.0/demo-1.0.0.jar"
	if rel != want {
		t.Fatalf("relativeUploadPath = %q, want %q", rel, want)
	}
}

func TestCollectUploadFiles(t *testing.T) {
	root := t.TempDir()
	files := []string{
		filepath.Join(root, "a.jar"),
		filepath.Join(root, "b.pom"),
		filepath.Join(root, "nested", "c.jar"),
		filepath.Join(root, "nested", "ignore.txt"),
	}

	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(file), err)
		}
		if err := os.WriteFile(file, []byte("content"), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}

	got, err := collectUploadFiles(root)
	if err != nil {
		t.Fatalf("collectUploadFiles returned error: %v", err)
	}

	for i := range got {
		got[i] = filepath.Base(got[i])
	}
	slices.Sort(got)

	want := []string{"a.jar", "b.pom", "c.jar"}
	if !slices.Equal(got, want) {
		t.Fatalf("collectUploadFiles = %v, want %v", got, want)
	}
}
