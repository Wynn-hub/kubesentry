package console

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExceptionRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	validBodyBytes, err := json.Marshal(validExceptionReq())
	if err != nil {
		t.Fatal(err)
	}
	validBody := strings.Replace(string(validBodyBytes), "my-exception", "aaa-exception", 1)
	updateBody := strings.Replace(validBody, `"name":"aaa-exception",`, "", 1)
	conflictBody := strings.Replace(updateBody, `"match":`, `"resourceVersion":"stale","match":`, 1)
	duplicateBody := strings.Replace(validBody, "aaa-exception", "default-system-namespaces", 1)
	ok := routeExpectation{status: 200, code: 0, contentType: "application/json; charset=utf-8", message: "success"}
	bad := routeExpectation{status: 400, code: 40000, contentType: "application/json"}
	notFound := routeExpectation{status: 404, code: 40400, contentType: "application/json; charset=utf-8", message: "not found"}
	unknownRoute := routeExpectation{status: 404, code: 40400, contentType: "application/json", message: "unknown API route"}
	assertRouteExpectation(t, servePolicyRequest(actual.Handler, http.MethodPost, "/api/v1/exceptions", validBody, "application/json"), ok)
	tests := []struct {
		name, method, path, body, contentType string
		want                                  routeExpectation
	}{
		{name: "normal defaults", method: http.MethodGet, path: "/api/v1/exceptions", want: ok},
		{name: "explicit query", method: http.MethodGet, path: "/api/v1/exceptions?page=1&pageSize=10&sortBy=expiresAt&sortOrder=desc", want: ok},
		{name: "duplicate scalar query", method: http.MethodGet, path: "/api/v1/exceptions?page=1&page=2", want: withMessage(bad, "multiple values")},
		{name: "empty scalar query", method: http.MethodGet, path: "/api/v1/exceptions?page=", want: withMessage(bad, `"page"`)},
		{name: "invalid minimum query", method: http.MethodGet, path: "/api/v1/exceptions?page=0", want: withMessage(bad, `"page"`)},
		{name: "invalid page size", method: http.MethodGet, path: "/api/v1/exceptions?pageSize=11", want: withMessage(bad, `"pageSize"`)},
		{name: "invalid sort enum", method: http.MethodGet, path: "/api/v1/exceptions?sortBy=createdAt", want: withMessage(bad, `"sortBy"`)},
		{name: "invalid sort order", method: http.MethodGet, path: "/api/v1/exceptions?sortOrder=sideways", want: withMessage(bad, `"sortOrder"`)},
		{name: "business detail success", method: http.MethodGet, path: "/api/v1/exceptions/default-system-namespaces", want: ok},
		{name: "business not found", method: http.MethodGet, path: "/api/v1/exceptions/does-not-exist", want: notFound},
		{name: "delete not found", method: http.MethodDelete, path: "/api/v1/exceptions/does-not-exist", want: notFound},
		{name: "create conflict", method: http.MethodPost, path: "/api/v1/exceptions", body: duplicateBody, contentType: "application/json", want: routeExpectation{status: 409, code: 40900, contentType: "application/json; charset=utf-8", message: "already exists"}},
		{name: "update conflict", method: http.MethodPut, path: "/api/v1/exceptions/default-system-namespaces", body: conflictBody, contentType: "application/json", want: routeExpectation{status: 409, code: 40900, contentType: "application/json; charset=utf-8", message: "modified"}},
		{name: "escaped path", method: http.MethodGet, path: "/api/v1/exceptions/does%20not%20exist", want: notFound},
		{name: "escaped slash path", method: http.MethodGet, path: "/api/v1/exceptions/does%2Fnot%2Fexist", want: notFound},
		{name: "double slash path", method: http.MethodGet, path: "/api/v1/exceptions//example", want: unknownRoute},
		{name: "dot path", method: http.MethodGet, path: "/api/v1/exceptions/./example", want: unknownRoute},
		{name: "dot dot path", method: http.MethodPut, path: "/api/v1/exceptions/example/../default-system-namespaces", body: updateBody, contentType: "application/json", want: unknownRoute},
		{name: "unclean path enters API prefix", method: http.MethodPut, path: "/api//v1/exceptions/example/../default-system-namespaces", body: updateBody, contentType: "application/json", want: unknownRoute},
		{name: "API prefix without trailing slash", method: http.MethodGet, path: "/api/v1", want: unknownRoute},
		{name: "escaped API prefix without trailing slash", method: http.MethodGet, path: "/%61pi/v1", want: unknownRoute},
		{name: "missing required body fields", method: http.MethodPost, path: "/api/v1/exceptions", body: `{}`, contentType: "application/json", want: withMessage(bad, "required")},
		{name: "null match", method: http.MethodPost, path: "/api/v1/exceptions", body: `{"match":null,"duration":"24h","reason":"temporary"}`, contentType: "application/json", want: withMessage(bad, "match")},
		{name: "null optional list", method: http.MethodPost, path: "/api/v1/exceptions", body: `{"policyRefs":null,"match":{},"duration":"24h","reason":"temporary"}`, contentType: "application/json", want: withMessage(bad, "policyRefs")},
		{name: "duplicate duration validates first", method: http.MethodPost, path: "/api/v1/exceptions", body: `{"match":{},"duration":null,"duration":"24h","reason":"temporary"}`, contentType: "application/json", want: withMessage(bad, "duration")},
		{name: "missing body", method: http.MethodPost, path: "/api/v1/exceptions", contentType: "application/json", want: withMessage(bad, "body required")},
		{name: "invalid JSON", method: http.MethodPost, path: "/api/v1/exceptions", body: `{"match":`, contentType: "application/json", want: withMessage(bad, "unexpected EOF")},
		{name: "trailing JSON", method: http.MethodPost, path: "/api/v1/exceptions", body: validBody + `{}`, contentType: "application/json", want: withMessage(bad, "unexpected trailing data")},
		{name: "missing content type", method: http.MethodPost, path: "/api/v1/exceptions", body: validBody, want: withMessage(bad, "parse media type")},
		{name: "invalid content type", method: http.MethodPost, path: "/api/v1/exceptions", body: validBody, contentType: `application/json; charset="`, want: withMessage(bad, "parse media type")},
		{name: "unsupported content type", method: http.MethodPost, path: "/api/v1/exceptions", body: validBody, contentType: "text/plain", want: withMessage(bad, "unexpected Content-Type")},
		{name: "unknown field accepted", method: http.MethodPut, path: "/api/v1/exceptions/does-not-exist", body: strings.TrimSuffix(updateBody, "}") + `,"unknown":true}`, contentType: "application/json", want: notFound},
		{name: "large unknown field accepted", method: http.MethodPut, path: "/api/v1/exceptions/does-not-exist", body: strings.TrimSuffix(updateBody, "}") + `,"padding":"` + strings.Repeat("x", 1<<20) + `"}`, contentType: "application/json", want: notFound},
		{name: "unknown route", method: http.MethodGet, path: "/api/v1/exceptions/example/unknown", want: unknownRoute},
		{name: "method mismatch", method: http.MethodPatch, path: "/api/v1/exceptions/example", want: methodNotAllowed("DELETE,GET,PUT")},
		{name: "HEAD list does not imply GET", method: http.MethodHead, path: "/api/v1/exceptions", want: methodNotAllowed("GET,POST")},
		{name: "HEAD detail does not imply GET", method: http.MethodHead, path: "/api/v1/exceptions/default-system-namespaces", want: methodNotAllowed("DELETE,GET,PUT")},
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
				t.Fatalf("success data = %T, want object", data)
			}
			if tt.name == "business detail success" {
				spec, specOK := object["spec"].(map[string]any)
				if object["name"] != "default-system-namespaces" || !specOK || spec["match"] == nil {
					t.Fatalf("detail data = %v", object)
				}
				return
			}
			items, ok := object["items"].([]any)
			if !ok || len(items) != 2 || object["total"] != float64(len(items)) {
				t.Fatalf("items/total = %T(%v)/%v, want two matching items", object["items"], object["items"], object["total"])
			}
			pageSize := float64(20)
			if tt.name == "explicit query" {
				pageSize = 10
			}
			if object["page"] != float64(1) || object["pageSize"] != pageSize {
				t.Fatalf("pagination = page %v/pageSize %v, want 1/%v", object["page"], object["pageSize"], pageSize)
			}
			wantNames := []string{"aaa-exception", "default-system-namespaces"}
			if tt.name == "explicit query" {
				wantNames = []string{"default-system-namespaces", "aaa-exception"}
			}
			for i, item := range items {
				exception, itemOK := item.(map[string]any)
				if !itemOK {
					t.Fatalf("item %d = %T, want object", i, item)
				}
				name, nameOK := exception["name"].(string)
				if !nameOK || name != wantNames[i] || exception["expiresAt"] == nil {
					t.Fatalf("item %d = %v, want %q with expiresAt", i, item, wantNames[i])
				}
			}
		})
	}
}
