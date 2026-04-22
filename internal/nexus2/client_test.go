package nexus2

import "testing"

func TestResolveTargetFromBaseURL(t *testing.T) {
	target, err := ResolveTarget("https://example.com", "releases", "")
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if target.BaseURL != "https://example.com" {
		t.Fatalf("BaseURL = %q, want %q", target.BaseURL, "https://example.com")
	}
	if target.Repository != "releases" {
		t.Fatalf("Repository = %q, want %q", target.Repository, "releases")
	}
}

func TestResolveTargetFromRepositoryURL(t *testing.T) {
	target, err := ResolveTarget("", "", "https://example.com/content/repositories/npm-hosted")
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if target.BaseURL != "https://example.com" {
		t.Fatalf("BaseURL = %q, want %q", target.BaseURL, "https://example.com")
	}
	if target.Repository != "npm-hosted" {
		t.Fatalf("Repository = %q, want %q", target.Repository, "npm-hosted")
	}
}

func TestNpmPackageTarballPathScoped(t *testing.T) {
	got := npmPackageTarballPath("@scope/demo", "demo-1.0.0.tgz")
	want := "@scope/demo/-/demo-1.0.0.tgz"
	if got != want {
		t.Fatalf("npmPackageTarballPath = %q, want %q", got, want)
	}
}
