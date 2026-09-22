package console

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestClusterRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	createBytes, err := json.Marshal(map[string]any{"name": "Not_Valid", "installParams": validInstallParams()})
	if err != nil {
		t.Fatal(err)
	}
	tokenBytes, err := json.Marshal(map[string]any{"installParams": validInstallParams()})
	if err != nil {
		t.Fatal(err)
	}
	ok := routeExpectation{status: 200, code: 0, contentType: "application/json; charset=utf-8", message: "success"}
	bad := routeExpectation{status: 400, code: 40000, contentType: "application/json"}
	notFound := routeExpectation{status: 404, code: 40400, contentType: "application/json; charset=utf-8"}
	unknownRoute := routeExpectation{status: 404, code: 40400, contentType: "application/json", message: "unknown API route"}
	tests := []struct {
		name, method, path, body, contentType string
		want                                  routeExpectation
	}{
		{name: "clusters defaults", method: http.MethodGet, path: "/api/v1/clusters", want: ok},
		{name: "clusters query", method: http.MethodGet, path: "/api/v1/clusters?page=1&pageSize=10&sortBy=createdAt&sortOrder=desc", want: ok},
		{name: "clusters bad page", method: http.MethodGet, path: "/api/v1/clusters?page=0", want: withMessage(bad, `"page"`)},
		{name: "clusters bad page size", method: http.MethodGet, path: "/api/v1/clusters?pageSize=11", want: withMessage(bad, `"pageSize"`)},
		{name: "clusters bad sort", method: http.MethodGet, path: "/api/v1/clusters?sortBy=unknown", want: withMessage(bad, `"sortBy"`)},
		{name: "clusters duplicate scalar", method: http.MethodGet, path: "/api/v1/clusters?page=1&page=2", want: withMessage(bad, "multiple values")},
		{name: "create invalid service input", method: http.MethodPost, path: "/api/v1/clusters", body: string(createBytes), contentType: "application/json", want: routeExpectation{status: 400, code: 40000, contentType: "application/json; charset=utf-8", message: "invalid name"}},
		{name: "create null install params", method: http.MethodPost, path: "/api/v1/clusters", body: `{"name":"test","installParams":null}`, contentType: "application/json", want: withMessage(bad, "null is not allowed")},
		{name: "create invalid pull policy", method: http.MethodPost, path: "/api/v1/clusters", body: `{"name":"test","installParams":{"namespace":"kubesentry-system","webhookImage":{"repository":"webhook","tag":"latest","pullPolicy":"bad"},"operatorImage":{"repository":"operator","tag":"latest","pullPolicy":"IfNotPresent"},"highAvailability":false,"gatewayAddress":"gateway"}}`, contentType: "application/json", want: withMessage(bad, "invalid value")},
		{name: "create missing body", method: http.MethodPost, path: "/api/v1/clusters", contentType: "application/json", want: withMessage(bad, "body required")},
		{name: "create trailing JSON", method: http.MethodPost, path: "/api/v1/clusters", body: string(createBytes) + `{}`, contentType: "application/json", want: withMessage(bad, "unexpected trailing data")},
		{name: "create wrong content type", method: http.MethodPost, path: "/api/v1/clusters", body: string(createBytes), contentType: "text/plain", want: withMessage(bad, "unexpected Content-Type")},
		{name: "detail not found", method: http.MethodGet, path: "/api/v1/clusters/missing", want: withMessage(notFound, "cluster not found")},
		{name: "detail escaped name", method: http.MethodGet, path: "/api/v1/clusters/miss%20ing", want: withMessage(notFound, "cluster not found")},
		{name: "detail escaped slash", method: http.MethodGet, path: "/api/v1/clusters/miss%2Fing", want: withMessage(notFound, "cluster not found")},
		{name: "update not found", method: http.MethodPut, path: "/api/v1/clusters/missing", body: `{"labels":{}}`, contentType: "application/json", want: withMessage(notFound, "cluster not found")},
		{name: "update missing labels", method: http.MethodPut, path: "/api/v1/clusters/missing", body: `{}`, contentType: "application/json", want: withMessage(bad, "field is required")},
		{name: "update null labels", method: http.MethodPut, path: "/api/v1/clusters/missing", body: `{"labels":null}`, contentType: "application/json", want: withMessage(bad, "must be an object")},
		{name: "update duplicate labels", method: http.MethodPut, path: "/api/v1/clusters/missing", body: `{"labels":{"a":"1"},"labels":{"b":"2"}}`, contentType: "application/json", want: withMessage(notFound, "cluster not found")},
		{name: "delete not found", method: http.MethodDelete, path: "/api/v1/clusters/missing", want: withMessage(notFound, "cluster not found")},
		{name: "reissue not found", method: http.MethodPost, path: "/api/v1/clusters/missing/reissue-token", body: string(tokenBytes), contentType: "application/json", want: withMessage(notFound, "cluster not found")},
		{name: "latest params missing", method: http.MethodGet, path: "/api/v1/clusters/missing/latest-install-params", want: ok},
		{name: "uninstall missing", method: http.MethodPost, path: "/api/v1/clusters/missing/uninstall", want: withMessage(notFound, "cluster not found")},
		{name: "uninstall list missing", method: http.MethodGet, path: "/api/v1/clusters/missing/uninstall-tasks", want: withMessage(notFound, "cluster not found")},
		{name: "retry invalid id", method: http.MethodPost, path: "/api/v1/clusters/missing/uninstall-tasks/nope/retry", want: withMessage(bad, `"taskID"`)},
		{name: "retry not found", method: http.MethodPost, path: "/api/v1/clusters/missing/uninstall-tasks/999/retry", want: withMessage(notFound, "task not found")},
		{name: "install records defaults", method: http.MethodGet, path: "/api/v1/install-records", want: ok},
		{name: "install records query", method: http.MethodGet, path: "/api/v1/install-records?page=1&pageSize=10&sortBy=namespace&sortOrder=asc", want: ok},
		{name: "install records bad sort", method: http.MethodGet, path: "/api/v1/install-records?sortOrder=sideways", want: withMessage(bad, `"sortOrder"`)},
		{name: "HEAD clusters", method: http.MethodHead, path: "/api/v1/clusters", want: methodNotAllowed("GET,POST")},
		{name: "HEAD detail", method: http.MethodHead, path: "/api/v1/clusters/missing", want: methodNotAllowed("DELETE,GET,PUT")},
		{name: "HEAD latest params", method: http.MethodHead, path: "/api/v1/clusters/missing/latest-install-params", want: methodNotAllowed("GET")},
		{name: "HEAD uninstall list", method: http.MethodHead, path: "/api/v1/clusters/missing/uninstall-tasks", want: methodNotAllowed("GET")},
		{name: "HEAD install records", method: http.MethodHead, path: "/api/v1/install-records", want: methodNotAllowed("GET")},
		{name: "method mismatch", method: http.MethodPatch, path: "/api/v1/clusters/missing", want: methodNotAllowed("DELETE,GET,PUT")},
		{name: "static method mismatch", method: http.MethodGet, path: "/api/v1/clusters/missing/reissue-token", want: methodNotAllowed("POST")},
		{name: "trailing path", method: http.MethodGet, path: "/api/v1/clusters/missing/", want: unknownRoute},
		{name: "noncanonical path", method: http.MethodGet, path: "/api/v1/clusters/../clusters/missing", want: unknownRoute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := servePolicyRequest(actual.Handler, tt.method, tt.path, tt.body, tt.contentType)
			if tt.name == "latest params missing" {
				if got.Code != http.StatusOK || got.Header().Get("Content-Type") != ok.contentType || got.Header().Get("Allow") != "" {
					t.Fatalf("status/Content-Type/Allow = %d/%q/%q", got.Code, got.Header().Get("Content-Type"), got.Header().Get("Allow"))
				}
				var envelope map[string]any
				if err := json.Unmarshal(got.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope["code"] != float64(0) || envelope["message"] != "success" {
					t.Fatalf("response = %v, want success", envelope)
				}
				if _, exists := envelope["data"]; exists {
					t.Fatalf("missing params response unexpectedly contains data: %v", envelope)
				}
				return
			}
			data := assertRouteExpectation(t, got, tt.want)
			if tt.want.status != http.StatusOK {
				return
			}
			page, ok := data.(map[string]any)
			items, itemsOK := page["items"].([]any)
			if !ok || !itemsOK || len(items) != 0 || page["total"] != float64(0) || page["page"] != float64(1) {
				t.Fatalf("page data = %v, want empty first page", data)
			}
			wantSize := float64(20)
			if tt.name == "clusters query" || tt.name == "install records query" {
				wantSize = 10
			}
			if page["pageSize"] != wantSize {
				t.Fatalf("pageSize = %v, want %v", page["pageSize"], wantSize)
			}
		})
	}
}
