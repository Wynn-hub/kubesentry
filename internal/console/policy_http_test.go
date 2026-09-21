package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/repository"
)

func TestPolicyRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	validValidate := `{"rego":` + strconvQuote(validRego) + `}`
	wildcard := validReq()
	wildcard.Name = "star-ops"
	wildcard.Match.Operations = []string{"*"}
	wildcardBody, err := json.Marshal(wildcard)
	if err != nil {
		t.Fatal(err)
	}
	ok := routeExpectation{status: 200, code: 0, contentType: "application/json; charset=utf-8", message: "success"}
	bad := routeExpectation{status: 400, code: 40000, contentType: "application/json"}
	notFound := routeExpectation{status: 404, code: 40400, contentType: "application/json; charset=utf-8", message: "not found"}
	unknownRoute := routeExpectation{status: 404, code: 40400, contentType: "application/json", message: "unknown API route"}
	tests := []struct {
		name, method, path, body, contentType string
		want                                  routeExpectation
	}{
		{name: "normal defaults", method: http.MethodGet, path: "/api/v1/policies", want: ok},
		{name: "missing required body fields", method: http.MethodPost, path: "/api/v1/policies", body: `{}`, contentType: "application/json", want: withMessage(bad, "required")},
		{name: "missing body", method: http.MethodPost, path: "/api/v1/policies", contentType: "application/json", want: withMessage(bad, "body required")},
		{name: "invalid JSON", method: http.MethodPost, path: "/api/v1/policies/validate", body: `{"rego":`, contentType: "application/json", want: withMessage(bad, "unexpected EOF")},
		{name: "trailing JSON", method: http.MethodPost, path: "/api/v1/policies/validate", body: validValidate + `{}`, contentType: "application/json", want: withMessage(bad, "unexpected trailing data")},
		{name: "missing validate rego", method: http.MethodPost, path: "/api/v1/policies/validate", body: `{}`, contentType: "application/json", want: withMessage(bad, "field is required")},
		{name: "null validate rego", method: http.MethodPost, path: "/api/v1/policies/validate", body: `{"rego":null}`, contentType: "application/json", want: withMessage(bad, "must be a string")},
		{name: "missing rollback direction", method: http.MethodPost, path: "/api/v1/policies/run-as-privileged/rollback", body: `{}`, contentType: "application/json", want: withMessage(bad, "field is required")},
		{name: "null rollback direction", method: http.MethodPost, path: "/api/v1/policies/run-as-privileged/rollback", body: `{"direction":null}`, contentType: "application/json", want: withMessage(bad, "must be a string")},
		{name: "CSV query", method: http.MethodGet, path: "/api/v1/policies?mode=enforce,audit&phase=Ready,Invalid&source=builtin,custom", want: ok},
		{name: "empty CSV query", method: http.MethodGet, path: "/api/v1/policies?mode=", want: ok},
		{name: "duplicate scalar query", method: http.MethodGet, path: "/api/v1/policies?page=1&page=2", want: withMessage(bad, "multiple values")},
		{name: "duplicate CSV query", method: http.MethodGet, path: "/api/v1/policies?mode=enforce&mode=audit", want: withMessage(bad, `"mode"`)},
		{name: "invalid enum query", method: http.MethodGet, path: "/api/v1/policies?mode=block", want: withMessage(bad, `"mode"`)},
		{name: "empty scalar query", method: http.MethodGet, path: "/api/v1/policies?page=", want: withMessage(bad, `"page"`)},
		{name: "invalid minimum query", method: http.MethodGet, path: "/api/v1/policies?page=0", want: withMessage(bad, `"page"`)},
		{name: "invalid page size", method: http.MethodGet, path: "/api/v1/policies?pageSize=7", want: withMessage(bad, `"pageSize"`)},
		{name: "invalid sort by", method: http.MethodGet, path: "/api/v1/policies?sortBy=droptable", want: withMessage(bad, `"sortBy"`)},
		{name: "wildcard operation rejected", method: http.MethodPost, path: "/api/v1/policies", body: string(wildcardBody), contentType: "application/json", want: withMessage(bad, "operations")},
		{name: "invalid force query", method: http.MethodDelete, path: "/api/v1/policies/does-not-exist?force=maybe", want: withMessage(bad, `"force"`)},
		{name: "business not found", method: http.MethodGet, path: "/api/v1/policies/does-not-exist", want: notFound},
		{name: "unknown route", method: http.MethodGet, path: "/api/v1/policies/example/unknown", want: unknownRoute},
		{name: "method mismatch", method: http.MethodPatch, path: "/api/v1/policies/example", want: methodNotAllowed("DELETE,GET,PUT")},
		{name: "validate static path method mismatch", method: http.MethodGet, path: "/api/v1/policies/validate", want: methodNotAllowed("POST")},
		{name: "suggestions static path update mismatch", method: http.MethodPut, path: "/api/v1/policies/resource-suggestions", body: `{}`, contentType: "application/json", want: methodNotAllowed("GET")},
		{name: "suggestions static path delete mismatch", method: http.MethodDelete, path: "/api/v1/policies/resource-suggestions", want: methodNotAllowed("GET")},
		{name: "HEAD does not imply GET", method: http.MethodHead, path: "/api/v1/policies", want: methodNotAllowed("GET,POST")},
		{name: "missing content type", method: http.MethodPost, path: "/api/v1/policies/validate", body: validValidate, want: withMessage(bad, "parse media type")},
		{name: "invalid content type", method: http.MethodPost, path: "/api/v1/policies/validate", body: validValidate, contentType: `application/json; charset="`, want: withMessage(bad, "parse media type")},
		{name: "unsupported content type", method: http.MethodPost, path: "/api/v1/policies/validate", body: validValidate, contentType: "text/plain", want: withMessage(bad, "unexpected Content-Type")},
		{name: "unknown field accepted", method: http.MethodPost, path: "/api/v1/policies/validate", body: strings.TrimSuffix(validValidate, "}") + `,"unknown":true}`, contentType: "application/json", want: ok},
		{name: "large unknown field accepted", method: http.MethodPost, path: "/api/v1/policies/validate", body: strings.TrimSuffix(validValidate, "}") + `,"padding":"` + strings.Repeat("x", 1<<20) + `"}`, contentType: "application/json", want: ok},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := servePolicyRequest(actual.Handler, tt.method, tt.path, tt.body, tt.contentType)
			if tt.name == "unknown field accepted" || tt.name == "large unknown field accepted" {
				if got.Code != http.StatusOK || got.Header().Get("Content-Type") != ok.contentType {
					t.Fatalf("status/Content-Type = %d/%q, want 200/%q", got.Code, got.Header().Get("Content-Type"), ok.contentType)
				}
				var envelope map[string]any
				if err := json.Unmarshal(got.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope["code"] != float64(0) || envelope["message"] != "success" {
					t.Fatalf("response = %v, want success", envelope)
				}
				if _, exists := envelope["data"]; exists {
					t.Fatalf("validate response unexpectedly contains data: %v", envelope)
				}
				return
			}
			data := assertRouteExpectation(t, got, tt.want)
			if tt.want.status != http.StatusOK {
				return
			}
			page, ok := data.(map[string]any)
			items, itemsOK := page["items"].([]any)
			if !ok || !itemsOK || len(items) != 20 || page["total"] != float64(37) || page["page"] != float64(1) || page["pageSize"] != float64(20) {
				t.Fatalf("page data = %v, want matching first page", data)
			}
			previous := ""
			for i, item := range items {
				policy, itemOK := item.(map[string]any)
				if !itemOK {
					t.Fatalf("item %d = %T, want object", i, item)
				}
				name, nameOK := policy["name"].(string)
				if !nameOK || name == "" || name < previous || policy["enforcementMode"] == nil || policy["phase"] == nil || policy["source"] == nil {
					t.Fatalf("item %d = %v, want complete name-sorted policy", i, policy)
				}
				previous = name
			}
		})
	}
}

func TestPolicyListRepositoryFailure(t *testing.T) {
	db := newTestDB(t)
	server, err := NewServer(&Handlers{Repos: repository.New(db)}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	got := servePolicyRequest(server.Handler, http.MethodGet, "/api/v1/policies", "", "")
	assertRouteExpectation(t, got, routeExpectation{status: 500, code: 50000, contentType: "application/json; charset=utf-8", message: "request failed"})
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

func strconvQuote(value string) string {
	quoted, _ := json.Marshal(value)
	return string(quoted)
}
