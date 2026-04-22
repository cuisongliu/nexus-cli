package httpx

import "testing"

func TestNormalizeBaseURLAcceptsHTTPAndHTTPS(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "http",
			in:   "http://127.0.0.1:8081/repository/maven-hosted/",
			want: "http://127.0.0.1:8081/repository/maven-hosted",
		},
		{
			name: "https",
			in:   "https://nexus.example.com/repository/maven-hosted/",
			want: "https://nexus.example.com/repository/maven-hosted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tt.in)
			if err != nil {
				t.Fatalf("NormalizeBaseURL returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeBaseURL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeRequestURLRejectsUnsupportedScheme(t *testing.T) {
	if _, err := NormalizeRequestURL("ftp://example.com/file.tgz"); err == nil {
		t.Fatal("NormalizeRequestURL expected error for unsupported scheme")
	}
}
