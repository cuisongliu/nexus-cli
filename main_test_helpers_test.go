package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTarballServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/demo/-/demo-1.0.0.tgz" {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("tarball-body"))
	}))
}
