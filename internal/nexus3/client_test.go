package nexus3

import "testing"

func TestResolveTargetFromBaseURL(t *testing.T) {
	target, err := ResolveTarget("https://example.com/nexus", "maven-releases", "")
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if target.BaseURL != "https://example.com/nexus" {
		t.Fatalf("ResolveTarget BaseURL = %q, want %q", target.BaseURL, "https://example.com/nexus")
	}
	if target.Repository != "maven-releases" {
		t.Fatalf("ResolveTarget Repository = %q, want %q", target.Repository, "maven-releases")
	}
}

func TestResolveTargetFromRepositoryURL(t *testing.T) {
	target, err := ResolveTarget("", "", "https://example.com/nexus/repository/npm-hosted/")
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if target.BaseURL != "https://example.com/nexus" {
		t.Fatalf("ResolveTarget BaseURL = %q, want %q", target.BaseURL, "https://example.com/nexus")
	}
	if target.Repository != "npm-hosted" {
		t.Fatalf("ResolveTarget Repository = %q, want %q", target.Repository, "npm-hosted")
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		want  Format
	}{
		{input: "maven", want: FormatMaven},
		{input: "maven2", want: FormatMaven},
		{input: "npm", want: FormatNPM},
	}

	for _, tt := range tests {
		got, err := ParseFormat(tt.input)
		if err != nil {
			t.Fatalf("ParseFormat(%q) returned error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("ParseFormat(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeRepositoryPath(t *testing.T) {
	got, err := normalizeRepositoryPath("/com/example/demo/1.0.0/demo-1.0.0.jar")
	if err != nil {
		t.Fatalf("normalizeRepositoryPath returned error: %v", err)
	}
	want := "com/example/demo/1.0.0/demo-1.0.0.jar"
	if got != want {
		t.Fatalf("normalizeRepositoryPath = %q, want %q", got, want)
	}
}
