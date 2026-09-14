package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
)

// validRego is a minimal v0-syntax module satisfying the kubesentry contract.
const validRego = `package kubesentry

deny[msg] {
	input.request.object.spec.hostNetwork == true
	msg := "hostNetwork not allowed"
}`

type testEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Details []string        `json:"details"`
}

type testPageResult[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// newTestServer builds a console server around a bare *Handlers, for tests
// that don't need a database (e.g. handlers_schema.go, whose fetcher is
// swapped for a fake in tests rather than backed by Repos/Discovery).
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	initTestIDs(t)
	srv, err := NewServer(&Handlers{}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	return srv.Handler
}

// newTestServerWithHandlers is like newTestServer but returns the *Handlers
// itself too, for tests that need to poke at fields newTestServer doesn't
// expose (e.g. a fake SchemaFetcher).
func newTestServerWithHandlers(t *testing.T) (http.Handler, *Handlers) {
	t.Helper()
	initTestIDs(t)
	h := &Handlers{}
	srv, err := NewServer(h, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	return srv.Handler, h
}

func initTestIDs(t *testing.T) {
	t.Helper()
	if err := dbmodel.Init(1); err != nil {
		t.Fatal(err)
	}
}

func doRequest(t *testing.T, h http.Handler, method, path string, body any) (*httptest.ResponseRecorder, testEnvelope) {
	return doRequestWithHeaders(t, h, method, path, body, nil)
}

func doRequestWithHeaders(t *testing.T, h http.Handler, method, path string, body any, headers map[string]string) (*httptest.ResponseRecorder, testEnvelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var env testEnvelope
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode envelope: %v (body=%s)", err, rec.Body.String())
		}
	}
	return rec, env
}

func mustUnmarshal[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal data: %v (raw=%s)", err, raw)
	}
	return v
}
