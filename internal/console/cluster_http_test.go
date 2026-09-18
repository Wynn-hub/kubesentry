package console

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestClusterNetHTTPMatchesOgenRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	oracle := newOgenOracle(t, repos)
	createBytes, err := json.Marshal(map[string]any{"name": "Not_Valid", "installParams": validInstallParams()})
	if err != nil {
		t.Fatal(err)
	}
	tokenBytes, err := json.Marshal(map[string]any{"installParams": validInstallParams()})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, method, path, body, contentType string }{
		{name: "clusters defaults", method: http.MethodGet, path: "/api/v1/clusters"},
		{name: "clusters query", method: http.MethodGet, path: "/api/v1/clusters?page=1&pageSize=10&sortBy=createdAt&sortOrder=desc"},
		{name: "clusters bad page", method: http.MethodGet, path: "/api/v1/clusters?page=0"},
		{name: "clusters bad page size", method: http.MethodGet, path: "/api/v1/clusters?pageSize=11"},
		{name: "clusters bad sort", method: http.MethodGet, path: "/api/v1/clusters?sortBy=unknown"},
		{name: "clusters duplicate scalar", method: http.MethodGet, path: "/api/v1/clusters?page=1&page=2"},
		{name: "create invalid service input", method: http.MethodPost, path: "/api/v1/clusters", body: string(createBytes), contentType: "application/json"},
		{name: "create missing body", method: http.MethodPost, path: "/api/v1/clusters", contentType: "application/json"},
		{name: "create trailing JSON", method: http.MethodPost, path: "/api/v1/clusters", body: string(createBytes) + `{}`, contentType: "application/json"},
		{name: "create wrong content type", method: http.MethodPost, path: "/api/v1/clusters", body: string(createBytes), contentType: "text/plain"},
		{name: "detail not found", method: http.MethodGet, path: "/api/v1/clusters/missing"},
		{name: "detail escaped name", method: http.MethodGet, path: "/api/v1/clusters/miss%20ing"},
		{name: "detail escaped slash", method: http.MethodGet, path: "/api/v1/clusters/miss%2Fing"},
		{name: "update not found", method: http.MethodPut, path: "/api/v1/clusters/missing", body: `{"labels":{}}`, contentType: "application/json"},
		{name: "delete not found", method: http.MethodDelete, path: "/api/v1/clusters/missing"},
		{name: "reissue not found", method: http.MethodPost, path: "/api/v1/clusters/missing/reissue-token", body: string(tokenBytes), contentType: "application/json"},
		{name: "latest params missing", method: http.MethodGet, path: "/api/v1/clusters/missing/latest-install-params"},
		{name: "uninstall missing", method: http.MethodPost, path: "/api/v1/clusters/missing/uninstall"},
		{name: "uninstall list missing", method: http.MethodGet, path: "/api/v1/clusters/missing/uninstall-tasks"},
		{name: "retry invalid id", method: http.MethodPost, path: "/api/v1/clusters/missing/uninstall-tasks/nope/retry"},
		{name: "retry not found", method: http.MethodPost, path: "/api/v1/clusters/missing/uninstall-tasks/999/retry"},
		{name: "install records defaults", method: http.MethodGet, path: "/api/v1/install-records"},
		{name: "install records query", method: http.MethodGet, path: "/api/v1/install-records?page=1&pageSize=10&sortBy=namespace&sortOrder=asc"},
		{name: "install records bad sort", method: http.MethodGet, path: "/api/v1/install-records?sortOrder=sideways"},
		{name: "HEAD clusters", method: http.MethodHead, path: "/api/v1/clusters"},
		{name: "HEAD detail", method: http.MethodHead, path: "/api/v1/clusters/missing"},
		{name: "HEAD latest params", method: http.MethodHead, path: "/api/v1/clusters/missing/latest-install-params"},
		{name: "HEAD uninstall list", method: http.MethodHead, path: "/api/v1/clusters/missing/uninstall-tasks"},
		{name: "HEAD install records", method: http.MethodHead, path: "/api/v1/install-records"},
		{name: "method mismatch", method: http.MethodPatch, path: "/api/v1/clusters/missing"},
		{name: "static method mismatch", method: http.MethodGet, path: "/api/v1/clusters/missing/reissue-token"},
		{name: "trailing path", method: http.MethodGet, path: "/api/v1/clusters/missing/"},
		{name: "noncanonical path", method: http.MethodGet, path: "/api/v1/clusters/../clusters/missing"},
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
