package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newBareServer(ready func() bool) http.Handler {
	srv, err := NewServer(&Handlers{}, ready)
	if err != nil {
		panic(err)
	}
	return srv.Handler
}

func TestHealthz(t *testing.T) {
	h := newBareServer(func() bool { return true })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("healthz = %d", rec.Code)
	}
}

func TestReadyzGated(t *testing.T) {
	h := newBareServer(func() bool { return false })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 503 {
		t.Fatalf("readyz = %d, want 503", rec.Code)
	}
}

func TestReadyzNilAlwaysReady(t *testing.T) {
	h := newBareServer(nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 200 {
		t.Fatalf("readyz with nil ready func = %d, want 200", rec.Code)
	}
}

func TestUnknownAPIRouteIs404JSON(t *testing.T) {
	h := newBareServer(func() bool { return true })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/nonexistent", nil))
	if rec.Code != 404 {
		t.Fatalf("unknown api = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
}

func TestUnknownNonAPIRouteIs404(t *testing.T) {
	// The SPA and its history-routing fallback now live in the nginx `web`
	// container (charts/kubesentry-gateway/console); console-api's own mux
	// has no catch-all route left, so an unmatched path is a plain 404.
	h := newBareServer(func() bool { return true })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/policies/foo", nil))
	if rec.Code != 404 {
		t.Fatalf("unknown non-api route = %d, want 404", rec.Code)
	}
}

func TestOpenAPIDocumentationEndpoints(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAPI_SPEC_FILE", filepath.Join(root, "..", "..", "api", "openapi.yaml"))
	h := newBareServer(func() bool { return true })

	docs := httptest.NewRecorder()
	h.ServeHTTP(docs, httptest.NewRequest("GET", "/api/docs", nil))
	if docs.Code != http.StatusOK || docs.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("api docs = %d %q", docs.Code, docs.Header().Get("Content-Type"))
	}

	spec := httptest.NewRecorder()
	h.ServeHTTP(spec, httptest.NewRequest("GET", "/api/docs/openapi.json", nil))
	if spec.Code != http.StatusOK || spec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("openapi json = %d %q: %s", spec.Code, spec.Header().Get("Content-Type"), spec.Body.String())
	}
	var document map[string]any
	if err := json.Unmarshal(spec.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode openapi json: %v", err)
	}
	if document["openapi"] != "3.1.0" {
		t.Fatalf("openapi version = %v", document["openapi"])
	}
}
