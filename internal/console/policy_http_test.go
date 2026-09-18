package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/handler"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func TestPolicyNetHTTPMatchesOgenRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	oracle := newOgenOracle(t, repos)

	validValidate := `{"rego":` + strconvQuote(validRego) + `}`
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
	}{
		{name: "normal defaults", method: http.MethodGet, path: "/api/v1/policies"},
		{name: "missing required body fields", method: http.MethodPost, path: "/api/v1/policies", body: `{}`, contentType: "application/json"},
		{name: "missing body", method: http.MethodPost, path: "/api/v1/policies", contentType: "application/json"},
		{name: "invalid JSON", method: http.MethodPost, path: "/api/v1/policies/validate", body: `{"rego":`, contentType: "application/json"},
		{name: "trailing JSON", method: http.MethodPost, path: "/api/v1/policies/validate", body: validValidate + `{}`, contentType: "application/json"},
		{name: "CSV query", method: http.MethodGet, path: "/api/v1/policies?mode=enforce,audit&phase=Ready,Invalid&source=builtin,custom"},
		{name: "empty CSV query", method: http.MethodGet, path: "/api/v1/policies?mode="},
		{name: "duplicate scalar query", method: http.MethodGet, path: "/api/v1/policies?page=1&page=2"},
		{name: "duplicate CSV query", method: http.MethodGet, path: "/api/v1/policies?mode=enforce&mode=audit"},
		{name: "invalid enum query", method: http.MethodGet, path: "/api/v1/policies?mode=block"},
		{name: "empty scalar query", method: http.MethodGet, path: "/api/v1/policies?page="},
		{name: "invalid force query", method: http.MethodDelete, path: "/api/v1/policies/does-not-exist?force=maybe"},
		{name: "business not found", method: http.MethodGet, path: "/api/v1/policies/does-not-exist"},
		{name: "unknown route", method: http.MethodGet, path: "/api/v1/policies/example/unknown"},
		{name: "method mismatch", method: http.MethodPatch, path: "/api/v1/policies/example"},
		{name: "validate static path method mismatch", method: http.MethodGet, path: "/api/v1/policies/validate"},
		{name: "suggestions static path update mismatch", method: http.MethodPut, path: "/api/v1/policies/resource-suggestions", body: `{}`, contentType: "application/json"},
		{name: "suggestions static path delete mismatch", method: http.MethodDelete, path: "/api/v1/policies/resource-suggestions"},
		{name: "HEAD does not imply GET", method: http.MethodHead, path: "/api/v1/policies"},
		{name: "missing content type", method: http.MethodPost, path: "/api/v1/policies/validate", body: validValidate},
		{name: "invalid content type", method: http.MethodPost, path: "/api/v1/policies/validate", body: validValidate, contentType: `application/json; charset="`},
		{name: "unsupported content type", method: http.MethodPost, path: "/api/v1/policies/validate", body: validValidate, contentType: "text/plain"},
		{name: "unknown field accepted", method: http.MethodPost, path: "/api/v1/policies/validate", body: strings.TrimSuffix(validValidate, "}") + `,"unknown":true}`, contentType: "application/json"},
		{name: "large unknown field accepted", method: http.MethodPost, path: "/api/v1/policies/validate", body: strings.TrimSuffix(validValidate, "}") + `,"padding":"` + strings.Repeat("x", 1<<20) + `"}`, contentType: "application/json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := servePolicyRequest(oracle, tt.method, tt.path, tt.body, tt.contentType)
			got := servePolicyRequest(actual.Handler, tt.method, tt.path, tt.body, tt.contentType)
			if got.Code != want.Code {
				t.Fatalf("status = %d, want %d; body=%s; want body=%s", got.Code, want.Code, got.Body.String(), want.Body.String())
			}
			if got.Header().Get("Content-Type") != want.Header().Get("Content-Type") {
				t.Fatalf("content-type = %q, want %q", got.Header().Get("Content-Type"), want.Header().Get("Content-Type"))
			}
			assertSameJSON(t, got.Body.Bytes(), want.Body.Bytes())
		})
	}
}

func newOgenOracle(t *testing.T, repos repository.Repositories, schema ...func(context.Context, dto.FieldSchemaOptions) ([]dto.FieldNodeView, error)) http.Handler {
	t.Helper()
	svcs := service.New(repos, nil)
	if len(schema) > 0 {
		svcs.Schema = schema[0]
	}
	ogen, err := dto.NewServer(
		handler.New(svcs),
		dto.WithPathPrefix("/api/v1"),
		dto.WithMiddleware(TraceIDMiddleware()),
		dto.WithErrorHandler(ogenErrorHandler),
	)
	if err != nil {
		t.Fatal(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		ogen.ServeHTTP(rec, r)
		if rec.Code == http.StatusNotFound && rec.Body.String() == "404 page not found\n" {
			writeErr(w, http.StatusNotFound, "unknown API route")
			return
		}
		for key, values := range rec.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	})
}

func ogenErrorHandler(_ context.Context, w http.ResponseWriter, _ *http.Request, err error) {
	writeErr(w, http.StatusBadRequest, err.Error())
}

func servePolicyRequest(h http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-Request-Id", "policy-route-matrix")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func assertSameJSON(t *testing.T, got, want []byte) {
	t.Helper()
	if len(got) == 0 || len(want) == 0 {
		if !bytes.Equal(got, want) {
			t.Fatalf("body = %q, want %q", got, want)
		}
		return
	}
	var gotJSON, wantJSON any
	if err := json.Unmarshal(got, &gotJSON); err != nil {
		t.Fatalf("decode actual JSON %q: %v", got, err)
	}
	if err := json.Unmarshal(want, &wantJSON); err != nil {
		t.Fatalf("decode oracle JSON %q: %v", want, err)
	}
	gotNormalized, _ := json.Marshal(gotJSON)
	wantNormalized, _ := json.Marshal(wantJSON)
	if !bytes.Equal(gotNormalized, wantNormalized) {
		t.Fatalf("body = %s, want %s", gotNormalized, wantNormalized)
	}
}

func strconvQuote(value string) string {
	quoted, _ := json.Marshal(value)
	return string(quoted)
}
