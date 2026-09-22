package console

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDistributionRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	seedDistribution(t, repos, "my-dist", []string{"g1"})
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	bodyBytes, err := json.Marshal(validDistributionReq())
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	updateBody := strings.Replace(body, `"name":"my-dist",`, "", 1)
	tests := []struct {
		name, method, path, body, contentType string
		status                                int
		allow                                 string
		plainJSON                             bool
		message                               string
	}{
		{name: "list defaults", method: http.MethodGet, path: "/api/v1/distributions", status: http.StatusOK},
		{name: "list query", method: http.MethodGet, path: "/api/v1/distributions?page=1&pageSize=10&sortBy=name&sortOrder=desc", status: http.StatusOK},
		{name: "list bad page", method: http.MethodGet, path: "/api/v1/distributions?page=0", status: http.StatusBadRequest, plainJSON: true, message: `"page"`},
		{name: "list bad page size", method: http.MethodGet, path: "/api/v1/distributions?pageSize=11", status: http.StatusBadRequest, plainJSON: true, message: `"pageSize"`},
		{name: "list bad sort", method: http.MethodGet, path: "/api/v1/distributions?sortBy=createdAt", status: http.StatusBadRequest, plainJSON: true, message: `"sortBy"`},
		{name: "list duplicate scalar", method: http.MethodGet, path: "/api/v1/distributions?page=1&page=2", status: http.StatusBadRequest, plainJSON: true, message: "multiple values"},
		{name: "create conflict", method: http.MethodPost, path: "/api/v1/distributions", body: body, contentType: "application/json", status: http.StatusConflict, message: "policydistribution my-dist already exists"},
		{name: "create missing body", method: http.MethodPost, path: "/api/v1/distributions", contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "body required"},
		{name: "create trailing JSON", method: http.MethodPost, path: "/api/v1/distributions", body: body + `{}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "unexpected trailing data"},
		{name: "create missing spec", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist"}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: `"spec"`},
		{name: "create null spec", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist","spec":null}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: `"spec"`},
		{name: "create null name", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":null,"spec":{"policyGroups":[]}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "name"},
		{name: "create missing policy groups", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist","spec":{}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: `"policyGroups"`},
		{name: "create duplicate spec validates first", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist","spec":{},"spec":{"policyGroups":[]}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: `"policyGroups"`},
		{name: "create missing override cluster", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist","spec":{"policyGroups":[],"overrides":[{"enforcementMode":"audit"}]}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "cluster"},
		{name: "create null override cluster", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist","spec":{"policyGroups":[],"overrides":[{"cluster":null,"enforcementMode":"audit"}]}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "cluster"},
		{name: "create duplicate override cluster validates first", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist","spec":{"policyGroups":[],"overrides":[{"cluster":null,"cluster":"local","enforcementMode":"audit"}]}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "cluster"},
		{name: "create invalid enforcement mode", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist","spec":{"policyGroups":[],"overrides":[{"cluster":"local","enforcementMode":"invalid"}]}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "enforcementMode"},
		{name: "create invalid cluster selector", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist","spec":{"policyGroups":[],"clusters":{"clusterSelector":{"matchExpressions":[{"key":"env","operator":"invalid"}]}}}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "operator"},
		{name: "create null clusters", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist","spec":{"policyGroups":[],"clusters":null}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "clusters"},
		{name: "create null selector values", method: http.MethodPost, path: "/api/v1/distributions", body: `{"name":"new-dist","spec":{"policyGroups":[],"clusters":{"clusterSelector":{"matchExpressions":[{"key":"env","operator":"In","values":null}]}}}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "values"},
		{name: "create unknown field accepted", method: http.MethodPost, path: "/api/v1/distributions", body: strings.TrimSuffix(body, "}") + `,"unknown":true}`, contentType: "application/json", status: http.StatusConflict, message: "policydistribution my-dist already exists"},
		{name: "create wrong content type", method: http.MethodPost, path: "/api/v1/distributions", body: body, contentType: "text/plain", status: http.StatusBadRequest, plainJSON: true, message: "unexpected Content-Type: text/plain"},
		{name: "detail success", method: http.MethodGet, path: "/api/v1/distributions/my-dist", status: http.StatusOK},
		{name: "detail not found", method: http.MethodGet, path: "/api/v1/distributions/missing", status: http.StatusNotFound, message: "distribution not found"},
		{name: "escaped name", method: http.MethodGet, path: "/api/v1/distributions/miss%20ing", status: http.StatusNotFound, message: "distribution not found"},
		{name: "escaped slash", method: http.MethodGet, path: "/api/v1/distributions/miss%2Fing", status: http.StatusNotFound, message: "distribution not found"},
		{name: "update not found", method: http.MethodPut, path: "/api/v1/distributions/missing", body: updateBody, contentType: "application/json", status: http.StatusNotFound, message: "distribution not found"},
		{name: "update null resource version", method: http.MethodPut, path: "/api/v1/distributions/missing", body: `{"resourceVersion":null,"spec":{"policyGroups":[]}}`, contentType: "application/json", status: http.StatusBadRequest, plainJSON: true, message: "resourceVersion"},
		{name: "delete not found", method: http.MethodDelete, path: "/api/v1/distributions/missing", status: http.StatusNotFound, message: "distribution not found"},
		{name: "HEAD list", method: http.MethodHead, path: "/api/v1/distributions", status: http.StatusMethodNotAllowed, allow: "GET,POST"},
		{name: "HEAD detail", method: http.MethodHead, path: "/api/v1/distributions/my-dist", status: http.StatusMethodNotAllowed, allow: "DELETE,GET,PUT"},
		{name: "method mismatch", method: http.MethodPatch, path: "/api/v1/distributions/my-dist", status: http.StatusMethodNotAllowed, allow: "DELETE,GET,PUT"},
		{name: "trailing path", method: http.MethodGet, path: "/api/v1/distributions/my-dist/", status: http.StatusNotFound, plainJSON: true, message: "unknown API route"},
		{name: "noncanonical path", method: http.MethodGet, path: "/api/v1/distributions/../distributions/my-dist", status: http.StatusNotFound, plainJSON: true, message: "unknown API route"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := servePolicyRequest(actual.Handler, tt.method, tt.path, tt.body, tt.contentType)
			if got.Code != tt.status {
				t.Fatalf("status = %d, want %d; body=%s", got.Code, tt.status, got.Body.String())
			}
			if tt.status == http.StatusMethodNotAllowed {
				if got.Header().Get("Allow") != tt.allow || got.Header().Get("Content-Type") != "" || got.Body.Len() != 0 {
					t.Fatalf("405 response Allow/Content-Type/body = %q/%q/%q, want %q/empty/empty", got.Header().Get("Allow"), got.Header().Get("Content-Type"), got.Body.String(), tt.allow)
				}
				return
			}
			wantContentType := "application/json; charset=utf-8"
			if tt.plainJSON {
				wantContentType = "application/json"
			}
			if got.Header().Get("Content-Type") != wantContentType {
				t.Fatalf("Content-Type = %q, want %q", got.Header().Get("Content-Type"), wantContentType)
			}
			var envelope map[string]any
			if err := json.Unmarshal(got.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("response is not JSON: %v; body=%s", err, got.Body.String())
			}
			wantCode := float64(tt.status * 100)
			if tt.status == http.StatusOK {
				wantCode = 0
			}
			if envelope["code"] != wantCode {
				t.Fatalf("envelope code = %v, want %.0f; body=%s", envelope["code"], wantCode, got.Body.String())
			}
			if tt.status != http.StatusOK {
				message, ok := envelope["message"].(string)
				if !ok || !strings.Contains(message, tt.message) {
					t.Fatalf("error message = %q, want substring %q; body=%s", message, tt.message, got.Body.String())
				}
				if _, ok := envelope["data"]; ok {
					t.Fatalf("error response unexpectedly contains data: %s", got.Body.String())
				}
				return
			}
			if envelope["message"] != "success" {
				t.Fatalf("success message = %v", envelope["message"])
			}
			data, ok := envelope["data"].(map[string]any)
			if !ok {
				t.Fatalf("success data = %T, want object; body=%s", envelope["data"], got.Body.String())
			}
			switch tt.name {
			case "list defaults":
				if data["page"] != float64(1) || data["pageSize"] != float64(20) {
					t.Fatalf("default pagination = page %v/pageSize %v", data["page"], data["pageSize"])
				}
				assertDistributionItem(t, data, "my-dist")
			case "list query":
				if data["page"] != float64(1) || data["pageSize"] != float64(10) {
					t.Fatalf("query pagination = page %v/pageSize %v", data["page"], data["pageSize"])
				}
				assertDistributionItem(t, data, "my-dist")
			case "detail success":
				if data["name"] != "my-dist" {
					t.Fatalf("detail name = %v, want my-dist", data["name"])
				}
			}
		})
	}
}

func assertDistributionItem(t *testing.T, data map[string]any, name string) {
	t.Helper()
	items, ok := data["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %T/%v, want non-empty array", data["items"], data["items"])
	}
	for _, item := range items {
		if object, ok := item.(map[string]any); ok && object["name"] == name {
			return
		}
	}
	t.Fatalf("items do not contain %q: %v", name, items)
}
