package console

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExceptionNetHTTPMatchesOgenRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	oracle := newOgenOracle(t, repos)

	validBodyBytes, err := json.Marshal(validExceptionReq())
	if err != nil {
		t.Fatal(err)
	}
	validBody := string(validBodyBytes)
	updateBody := strings.Replace(validBody, `"name":"my-exception",`, "", 1)
	conflictBody := strings.Replace(updateBody, `"match":`, `"resourceVersion":"stale","match":`, 1)
	duplicateBody := strings.Replace(validBody, "my-exception", "default-system-namespaces", 1)
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
	}{
		{name: "normal defaults", method: http.MethodGet, path: "/api/v1/exceptions"},
		{name: "explicit query", method: http.MethodGet, path: "/api/v1/exceptions?page=1&pageSize=10&sortBy=expiresAt&sortOrder=desc"},
		{name: "duplicate scalar query", method: http.MethodGet, path: "/api/v1/exceptions?page=1&page=2"},
		{name: "empty scalar query", method: http.MethodGet, path: "/api/v1/exceptions?page="},
		{name: "invalid minimum query", method: http.MethodGet, path: "/api/v1/exceptions?page=0"},
		{name: "invalid page size", method: http.MethodGet, path: "/api/v1/exceptions?pageSize=11"},
		{name: "invalid sort enum", method: http.MethodGet, path: "/api/v1/exceptions?sortBy=createdAt"},
		{name: "invalid sort order", method: http.MethodGet, path: "/api/v1/exceptions?sortOrder=sideways"},
		{name: "business detail success", method: http.MethodGet, path: "/api/v1/exceptions/default-system-namespaces"},
		{name: "business not found", method: http.MethodGet, path: "/api/v1/exceptions/does-not-exist"},
		{name: "delete not found", method: http.MethodDelete, path: "/api/v1/exceptions/does-not-exist"},
		{name: "create conflict", method: http.MethodPost, path: "/api/v1/exceptions", body: duplicateBody, contentType: "application/json"},
		{name: "update conflict", method: http.MethodPut, path: "/api/v1/exceptions/default-system-namespaces", body: conflictBody, contentType: "application/json"},
		{name: "escaped path", method: http.MethodGet, path: "/api/v1/exceptions/does%20not%20exist"},
		{name: "escaped slash path", method: http.MethodGet, path: "/api/v1/exceptions/does%2Fnot%2Fexist"},
		{name: "double slash path", method: http.MethodGet, path: "/api/v1/exceptions//example"},
		{name: "dot path", method: http.MethodGet, path: "/api/v1/exceptions/./example"},
		{name: "dot dot path", method: http.MethodPut, path: "/api/v1/exceptions/example/../default-system-namespaces", body: updateBody, contentType: "application/json"},
		{name: "unclean path enters API prefix", method: http.MethodPut, path: "/api//v1/exceptions/example/../default-system-namespaces", body: updateBody, contentType: "application/json"},
		{name: "API prefix without trailing slash", method: http.MethodGet, path: "/api/v1"},
		{name: "escaped API prefix without trailing slash", method: http.MethodGet, path: "/%61pi/v1"},
		{name: "missing required body fields", method: http.MethodPost, path: "/api/v1/exceptions", body: `{}`, contentType: "application/json"},
		{name: "missing body", method: http.MethodPost, path: "/api/v1/exceptions", contentType: "application/json"},
		{name: "invalid JSON", method: http.MethodPost, path: "/api/v1/exceptions", body: `{"match":`, contentType: "application/json"},
		{name: "trailing JSON", method: http.MethodPost, path: "/api/v1/exceptions", body: validBody + `{}`, contentType: "application/json"},
		{name: "missing content type", method: http.MethodPost, path: "/api/v1/exceptions", body: validBody},
		{name: "invalid content type", method: http.MethodPost, path: "/api/v1/exceptions", body: validBody, contentType: `application/json; charset="`},
		{name: "unsupported content type", method: http.MethodPost, path: "/api/v1/exceptions", body: validBody, contentType: "text/plain"},
		{name: "unknown field accepted", method: http.MethodPut, path: "/api/v1/exceptions/does-not-exist", body: strings.TrimSuffix(updateBody, "}") + `,"unknown":true}`, contentType: "application/json"},
		{name: "large unknown field accepted", method: http.MethodPut, path: "/api/v1/exceptions/does-not-exist", body: strings.TrimSuffix(updateBody, "}") + `,"padding":"` + strings.Repeat("x", 1<<20) + `"}`, contentType: "application/json"},
		{name: "unknown route", method: http.MethodGet, path: "/api/v1/exceptions/example/unknown"},
		{name: "method mismatch", method: http.MethodPatch, path: "/api/v1/exceptions/example"},
		{name: "HEAD list does not imply GET", method: http.MethodHead, path: "/api/v1/exceptions"},
		{name: "HEAD detail does not imply GET", method: http.MethodHead, path: "/api/v1/exceptions/default-system-namespaces"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := servePolicyRequest(oracle, tt.method, tt.path, tt.body, tt.contentType)
			got := servePolicyRequest(actual.Handler, tt.method, tt.path, tt.body, tt.contentType)
			if got.Code != want.Code {
				t.Fatalf("status = %d, want %d; body=%s; want body=%s", got.Code, want.Code, got.Body.String(), want.Body.String())
			}
			for _, header := range []string{"Content-Type", "Allow"} {
				if got.Header().Get(header) != want.Header().Get(header) {
					t.Fatalf("%s = %q, want %q", header, got.Header().Get(header), want.Header().Get(header))
				}
			}
			assertSameJSON(t, got.Body.Bytes(), want.Body.Bytes())
		})
	}
}
