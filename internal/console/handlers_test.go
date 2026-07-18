package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// validRego is a minimal v0-syntax module satisfying the kubesentry contract.
const validRego = `package kubesentry

deny[msg] {
	input.request.object.spec.hostNetwork == true
	msg := "hostNetwork not allowed"
}`

type testEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *string         `json:"error"`
}

// newTestServer builds a console server around a bare *Handlers, for tests
// that don't need a database (e.g. handlers_schema.go, whose fetcher is
// swapped for a fake in tests rather than backed by Repos/Discovery).
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	srv := NewServer(&Handlers{}, func() bool { return true })
	return srv.Handler
}

// newTestServerWithHandlers is like newTestServer but returns the *Handlers
// itself too, for tests that need to poke at fields newTestServer doesn't
// expose (e.g. a fake SchemaFetcher).
func newTestServerWithHandlers(t *testing.T) (http.Handler, *Handlers) {
	t.Helper()
	h := &Handlers{}
	srv := NewServer(h, func() bool { return true })
	return srv.Handler, h
}

func doRequest(t *testing.T, h http.Handler, method, path string, body any) (*httptest.ResponseRecorder, testEnvelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
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
