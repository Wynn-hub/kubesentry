package webhook_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

func TestHealthzAlwaysOK(t *testing.T) {
	store := &stubStore{ready: false}
	s := webhook.NewServer(store, webhook.ServerConfig{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("healthz: expected 200, got %d", w.Code)
	}
}

func TestReadyzReflectsCache(t *testing.T) {
	store := &stubStore{ready: false}
	s := webhook.NewServer(store, webhook.ServerConfig{})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz: expected 503 when not ready, got %d", w.Code)
	}

	store.ready = true
	w2 := httptest.NewRecorder()
	s.Handler.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w2.Code != http.StatusOK {
		t.Errorf("readyz: expected 200 when ready, got %d", w2.Code)
	}
}
