package console

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPolicyGroupRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	validBodyBytes, err := json.Marshal(validGroupReq())
	if err != nil {
		t.Fatal(err)
	}
	validBody := string(validBodyBytes)
	validEnabled := `{"enabled":true}`
	ok := routeExpectation{status: 200, code: 0, contentType: "application/json; charset=utf-8", message: "success"}
	bad := routeExpectation{status: 400, code: 40000, contentType: "application/json"}
	notFound := routeExpectation{status: 404, code: 40400, contentType: "application/json; charset=utf-8", message: "not found"}
	unknownRoute := routeExpectation{status: 404, code: 40400, contentType: "application/json", message: "unknown API route"}
	tests := []struct {
		name, method, path, body, contentType string
		want                                  routeExpectation
	}{
		{name: "normal defaults", method: http.MethodGet, path: "/api/v1/policygroups", want: ok},
		{name: "explicit query", method: http.MethodGet, path: "/api/v1/policygroups?page=1&pageSize=10&sortBy=resolvedCount&sortOrder=desc", want: ok},
		{name: "duplicate scalar query", method: http.MethodGet, path: "/api/v1/policygroups?page=1&page=2", want: withMessage(bad, "multiple values")},
		{name: "empty scalar query", method: http.MethodGet, path: "/api/v1/policygroups?page=", want: withMessage(bad, `"page"`)},
		{name: "invalid minimum query", method: http.MethodGet, path: "/api/v1/policygroups?page=0", want: withMessage(bad, "page")},
		{name: "invalid page size", method: http.MethodGet, path: "/api/v1/policygroups?pageSize=11", want: withMessage(bad, `"pageSize"`)},
		{name: "invalid sort enum", method: http.MethodGet, path: "/api/v1/policygroups?sortBy=createdAt", want: withMessage(bad, `"sortBy"`)},
		{name: "business not found", method: http.MethodGet, path: "/api/v1/policygroups/does-not-exist", want: notFound},
		{name: "escaped path", method: http.MethodGet, path: "/api/v1/policygroups/does%20not%20exist", want: notFound},
		{name: "double slash path", method: http.MethodGet, path: "/api/v1/policygroups//example", want: unknownRoute},
		{name: "dot path", method: http.MethodGet, path: "/api/v1/policygroups/./example", want: unknownRoute},
		{name: "dot dot path", method: http.MethodPut, path: "/api/v1/policygroups/example/../default", body: validBody, contentType: "application/json", want: unknownRoute},
		{name: "unclean path enters API prefix", method: http.MethodPut, path: "/api//v1/policygroups/example/../default", body: validBody, contentType: "application/json", want: unknownRoute},
		{name: "API prefix without trailing slash", method: http.MethodGet, path: "/api/v1", want: unknownRoute},
		{name: "escaped API prefix without trailing slash", method: http.MethodGet, path: "/%61pi/v1", want: unknownRoute},
		{name: "missing required body fields", method: http.MethodPost, path: "/api/v1/policygroups", body: `{}`, contentType: "application/json", want: withMessage(bad, "required")},
		{name: "missing body", method: http.MethodPost, path: "/api/v1/policygroups", contentType: "application/json", want: withMessage(bad, "body required")},
		{name: "invalid JSON", method: http.MethodPost, path: "/api/v1/policygroups", body: `{"enabled":`, contentType: "application/json", want: withMessage(bad, "unexpected EOF")},
		{name: "trailing JSON", method: http.MethodPost, path: "/api/v1/policygroups", body: validBody + `{}`, contentType: "application/json", want: withMessage(bad, "unexpected trailing data")},
		{name: "missing content type", method: http.MethodPost, path: "/api/v1/policygroups", body: validBody, want: withMessage(bad, "parse media type")},
		{name: "invalid content type", method: http.MethodPost, path: "/api/v1/policygroups", body: validBody, contentType: `application/json; charset="`, want: withMessage(bad, "parse media type")},
		{name: "unsupported content type", method: http.MethodPost, path: "/api/v1/policygroups", body: validBody, contentType: "text/plain", want: withMessage(bad, "unexpected Content-Type")},
		{name: "enabled missing required field", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: `{}`, contentType: "application/json", want: withMessage(bad, "enabled")},
		{name: "enabled null field", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: `{"enabled":null}`, contentType: "application/json", want: withMessage(bad, "must be a boolean")},
		{name: "unknown field accepted", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: `{"enabled":true,"unknown":true}`, contentType: "application/json", want: notFound},
		{name: "large unknown field accepted", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: `{"enabled":true,"padding":"` + strings.Repeat("x", 1<<20) + `"}`, contentType: "application/json", want: notFound},
		{name: "enabled trailing JSON", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: validEnabled + `{}`, contentType: "application/json", want: withMessage(bad, "unexpected trailing data")},
		{name: "update reserved group forbidden", method: http.MethodPut, path: "/api/v1/policygroups/default", body: validBody, contentType: "application/json", want: routeExpectation{status: 403, code: 40300, contentType: "application/json; charset=utf-8", message: "default policy group scope is managed"}},
		{name: "delete reserved group forbidden", method: http.MethodDelete, path: "/api/v1/policygroups/default", want: routeExpectation{status: 403, code: 40300, contentType: "application/json; charset=utf-8", message: "default policy group cannot be deleted"}},
		{name: "unknown route", method: http.MethodGet, path: "/api/v1/policygroups/example/unknown", want: unknownRoute},
		{name: "method mismatch", method: http.MethodPatch, path: "/api/v1/policygroups/example", want: methodNotAllowed("DELETE,GET,PUT")},
		{name: "static enabled path method mismatch", method: http.MethodGet, path: "/api/v1/policygroups/example/enabled", want: methodNotAllowed("PUT")},
		{name: "static enabled path selected", method: http.MethodPut, path: "/api/v1/policygroups/does-not-exist/enabled", body: validEnabled, contentType: "application/json", want: notFound},
		{name: "HEAD list does not imply GET", method: http.MethodHead, path: "/api/v1/policygroups", want: methodNotAllowed("GET,POST")},
		{name: "HEAD detail does not imply GET", method: http.MethodHead, path: "/api/v1/policygroups/default", want: methodNotAllowed("DELETE,GET,PUT")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := servePolicyRequest(actual.Handler, tt.method, tt.path, tt.body, tt.contentType)
			data := assertRouteExpectation(t, got, tt.want)
			if tt.want.status != http.StatusOK {
				return
			}
			object, ok := data.(map[string]any)
			if !ok {
				t.Fatalf("list data = %v, want pagination object", data)
			}
			items, ok := object["items"].([]any)
			if !ok || len(items) == 0 || object["total"] != float64(len(items)) {
				t.Fatalf("items/total = %T(%v)/%v, want non-empty matching list", object["items"], object["items"], object["total"])
			}
			pageSize := float64(20)
			if tt.name == "explicit query" {
				pageSize = 10
			}
			if object["page"] != float64(1) || object["pageSize"] != pageSize {
				t.Fatalf("pagination = page %v/pageSize %v, want 1/%v", object["page"], object["pageSize"], pageSize)
			}
			for i, item := range items {
				group, ok := item.(map[string]any)
				name, nameOK := group["name"].(string)
				if !ok || !nameOK || name == "" {
					t.Fatalf("item %d = %v, want named group", i, item)
				}
				if tt.name == "explicit query" && i > 0 {
					previous := items[i-1].(map[string]any)
					previousCount, previousOK := previous["resolvedCount"].(float64)
					count, countOK := group["resolvedCount"].(float64)
					if !previousOK || !countOK || previousCount < count {
						t.Fatalf("resolvedCount order = %v before %v, want descending", previous, group)
					}
				}
			}
		})
	}
}
