package console

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func newBareServer(dist fstest.MapFS, ready func() bool) http.Handler {
	return NewServer(&Handlers{}, dist, ready).Handler
}

func TestHealthz(t *testing.T) {
	h := newBareServer(fstest.MapFS{}, func() bool { return true })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("healthz = %d", rec.Code)
	}
}

func TestReadyzGated(t *testing.T) {
	h := newBareServer(fstest.MapFS{}, func() bool { return false })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 503 {
		t.Fatalf("readyz = %d, want 503", rec.Code)
	}
}

func TestSPAFallbackToIndex(t *testing.T) {
	dist := fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
	}
	h := newBareServer(dist, func() bool { return true })
	// 未知的非 API 路径（history 路由）回退 index.html
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/policies/foo", nil))
	if rec.Code != 200 || rec.Body.String() != "<html>app</html>" {
		t.Fatalf("fallback: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSPANotBuilt(t *testing.T) {
	h := newBareServer(fstest.MapFS{}, func() bool { return true })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 503 {
		t.Fatalf("placeholder = %d, want 503", rec.Code)
	}
}

func TestUnknownAPIRouteIs404JSON(t *testing.T) {
	h := newBareServer(fstest.MapFS{}, func() bool { return true })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/nonexistent", nil))
	if rec.Code != 404 {
		t.Fatalf("unknown api = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
}
