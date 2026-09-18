package console

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDistributionNetHTTPMatchesOgenRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	seedDistribution(t, repos, "my-dist", []string{"g1"})
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	oracle := newOgenOracle(t, repos)
	bodyBytes, err := json.Marshal(validDistributionReq())
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	updateBody := strings.Replace(body, `"name":"my-dist",`, "", 1)
	tests := []struct{ name, method, path, body, contentType string }{
		{name: "list defaults", method: http.MethodGet, path: "/api/v1/distributions"},
		{name: "list query", method: http.MethodGet, path: "/api/v1/distributions?page=1&pageSize=10&sortBy=name&sortOrder=desc"},
		{name: "list bad page", method: http.MethodGet, path: "/api/v1/distributions?page=0"},
		{name: "list bad page size", method: http.MethodGet, path: "/api/v1/distributions?pageSize=11"},
		{name: "list bad sort", method: http.MethodGet, path: "/api/v1/distributions?sortBy=createdAt"},
		{name: "list duplicate scalar", method: http.MethodGet, path: "/api/v1/distributions?page=1&page=2"},
		{name: "create conflict", method: http.MethodPost, path: "/api/v1/distributions", body: body, contentType: "application/json"},
		{name: "create missing body", method: http.MethodPost, path: "/api/v1/distributions", contentType: "application/json"},
		{name: "create trailing JSON", method: http.MethodPost, path: "/api/v1/distributions", body: body + `{}`, contentType: "application/json"},
		{name: "create unknown field accepted", method: http.MethodPost, path: "/api/v1/distributions", body: strings.TrimSuffix(body, "}") + `,"unknown":true}`, contentType: "application/json"},
		{name: "create wrong content type", method: http.MethodPost, path: "/api/v1/distributions", body: body, contentType: "text/plain"},
		{name: "detail success", method: http.MethodGet, path: "/api/v1/distributions/my-dist"},
		{name: "detail not found", method: http.MethodGet, path: "/api/v1/distributions/missing"},
		{name: "escaped name", method: http.MethodGet, path: "/api/v1/distributions/miss%20ing"},
		{name: "escaped slash", method: http.MethodGet, path: "/api/v1/distributions/miss%2Fing"},
		{name: "update not found", method: http.MethodPut, path: "/api/v1/distributions/missing", body: updateBody, contentType: "application/json"},
		{name: "delete not found", method: http.MethodDelete, path: "/api/v1/distributions/missing"},
		{name: "HEAD list", method: http.MethodHead, path: "/api/v1/distributions"},
		{name: "HEAD detail", method: http.MethodHead, path: "/api/v1/distributions/my-dist"},
		{name: "method mismatch", method: http.MethodPatch, path: "/api/v1/distributions/my-dist"},
		{name: "trailing path", method: http.MethodGet, path: "/api/v1/distributions/my-dist/"},
		{name: "noncanonical path", method: http.MethodGet, path: "/api/v1/distributions/../distributions/my-dist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := servePolicyRequest(oracle, tt.method, tt.path, tt.body, tt.contentType)
			got := servePolicyRequest(actual.Handler, tt.method, tt.path, tt.body, tt.contentType)
			if got.Code != want.Code {
				t.Fatalf("status = %d, want %d; body=%s; want=%s", got.Code, want.Code, got.Body.String(), want.Body.String())
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
