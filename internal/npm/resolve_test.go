package npm

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCollectResolvedURLsFromPackageLock(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "package-lock.json")
	content := `{
  "packages": {
    "": {
      "name": "demo"
    },
    "node_modules/a": {
      "resolved": "https://registry.npmjs.org/a/-/a-1.0.0.tgz"
    }
  },
  "dependencies": {
    "b": {
      "resolved": "https://registry.npmjs.org/b/-/b-2.0.0.tgz"
    }
  }
}`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write package-lock.json: %v", err)
	}

	got, err := CollectResolvedURLs(file)
	if err != nil {
		t.Fatalf("CollectResolvedURLs returned error: %v", err)
	}

	want := []string{
		"https://registry.npmjs.org/a/-/a-1.0.0.tgz",
		"https://registry.npmjs.org/b/-/b-2.0.0.tgz",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CollectResolvedURLs = %v, want %v", got, want)
	}
}

func TestParseYarnResolvedURLs(t *testing.T) {
	input := `
"ansi-styles@^4.1.0":
  version "4.3.0"
  resolved "https://registry.npmjs.org/ansi-styles/-/ansi-styles-4.3.0.tgz#abc"
`
	got, err := parseYarnResolvedURLs(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseYarnResolvedURLs returned error: %v", err)
	}

	want := []string{"https://registry.npmjs.org/ansi-styles/-/ansi-styles-4.3.0.tgz#abc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseYarnResolvedURLs = %v, want %v", got, want)
	}
}

func TestFilenameFromURL(t *testing.T) {
	got, err := filenameFromURL("https://registry.npmjs.org/@scope/pkg/-/pkg-1.0.0.tgz?cache=1#sha")
	if err != nil {
		t.Fatalf("filenameFromURL returned error: %v", err)
	}
	if got != "pkg-1.0.0.tgz" {
		t.Fatalf("filenameFromURL = %q, want %q", got, "pkg-1.0.0.tgz")
	}
}

func TestCollectPackSpecs(t *testing.T) {
	root := t.TempDir()
	pkgDirs := []string{
		filepath.Join(root, "node_modules", "a"),
		filepath.Join(root, "node_modules", "b"),
		filepath.Join(root, "node_modules", "dup"),
	}
	payloads := []string{
		`{"name":"a","version":"1.0.0"}`,
		`{"name":"b","version":"2.0.0"}`,
		`{"name":"a","version":"1.0.0"}`,
	}

	for i, dir := range pkgDirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(payloads[i]), 0o644); err != nil {
			t.Fatalf("write package.json: %v", err)
		}
	}

	specs, err := collectPackSpecs(filepath.Join(root, "node_modules"))
	if err != nil {
		t.Fatalf("collectPackSpecs returned error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("collectPackSpecs len = %d, want 2", len(specs))
	}
}
