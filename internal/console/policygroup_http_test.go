package console

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPolicyGroupNetHTTPMatchesOgenRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	oracle := newOgenOracle(t, repos)

	validBodyBytes, err := json.Marshal(validGroupReq())
	if err != nil {
		t.Fatal(err)
	}
	validBody := string(validBodyBytes)
	validEnabled := `{"enabled":true}`
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
	}{
		{name: "normal defaults", method: http.MethodGet, path: "/api/v1/policygroups"},
		{name: "explicit query", method: http.MethodGet, path: "/api/v1/policygroups?page=1&pageSize=10&sortBy=resolvedCount&sortOrder=desc"},
		{name: "duplicate scalar query", method: http.MethodGet, path: "/api/v1/policygroups?page=1&page=2"},
		{name: "empty scalar query", method: http.MethodGet, path: "/api/v1/policygroups?page="},
		{name: "invalid minimum query", method: http.MethodGet, path: "/api/v1/policygroups?page=0"},
		{name: "invalid page size", method: http.MethodGet, path: "/api/v1/policygroups?pageSize=11"},
		{name: "invalid sort enum", method: http.MethodGet, path: "/api/v1/policygroups?sortBy=createdAt"},
		{name: "business not found", method: http.MethodGet, path: "/api/v1/policygroups/does-not-exist"},
		{name: "escaped path", method: http.MethodGet, path: "/api/v1/policygroups/does%20not%20exist"},
		{name: "double slash path", method: http.MethodGet, path: "/api/v1/policygroups//example"},
		{name: "dot path", method: http.MethodGet, path: "/api/v1/policygroups/./example"},
		{name: "dot dot path", method: http.MethodPut, path: "/api/v1/policygroups/example/../default", body: validBody, contentType: "application/json"},
		{name: "unclean path enters API prefix", method: http.MethodPut, path: "/api//v1/policygroups/example/../default", body: validBody, contentType: "application/json"},
		{name: "API prefix without trailing slash", method: http.MethodGet, path: "/api/v1"},
		{name: "escaped API prefix without trailing slash", method: http.MethodGet, path: "/%61pi/v1"},
		{name: "missing required body fields", method: http.MethodPost, path: "/api/v1/policygroups", body: `{}`, contentType: "application/json"},
		{name: "missing body", method: http.MethodPost, path: "/api/v1/policygroups", contentType: "application/json"},
		{name: "invalid JSON", method: http.MethodPost, path: "/api/v1/policygroups", body: `{"enabled":`, contentType: "application/json"},
		{name: "trailing JSON", method: http.MethodPost, path: "/api/v1/policygroups", body: validBody + `{}`, contentType: "application/json"},
		{name: "missing content type", method: http.MethodPost, path: "/api/v1/policygroups", body: validBody},
		{name: "invalid content type", method: http.MethodPost, path: "/api/v1/policygroups", body: validBody, contentType: `application/json; charset="`},
		{name: "unsupported content type", method: http.MethodPost, path: "/api/v1/policygroups", body: validBody, contentType: "text/plain"},
		{name: "enabled missing required field", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: `{}`, contentType: "application/json"},
		{name: "unknown field accepted", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: `{"enabled":true,"unknown":true}`, contentType: "application/json"},
		{name: "large unknown field accepted", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: `{"enabled":true,"padding":"` + strings.Repeat("x", 1<<20) + `"}`, contentType: "application/json"},
		{name: "enabled trailing JSON", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: validEnabled + `{}`, contentType: "application/json"},
		{name: "update reserved group forbidden", method: http.MethodPut, path: "/api/v1/policygroups/default", body: validBody, contentType: "application/json"},
		{name: "delete reserved group forbidden", method: http.MethodDelete, path: "/api/v1/policygroups/default"},
		{name: "unknown route", method: http.MethodGet, path: "/api/v1/policygroups/example/unknown"},
		{name: "method mismatch", method: http.MethodPatch, path: "/api/v1/policygroups/example"},
		{name: "static enabled path method mismatch", method: http.MethodGet, path: "/api/v1/policygroups/example/enabled"},
		{name: "static enabled path selected", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: validEnabled, contentType: "application/json"},
		{name: "HEAD list does not imply GET", method: http.MethodHead, path: "/api/v1/policygroups"},
		{name: "HEAD detail does not imply GET", method: http.MethodHead, path: "/api/v1/policygroups/default"},
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
