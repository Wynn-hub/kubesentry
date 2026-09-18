package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newBareServer(ready func() bool) http.Handler {
	srv, err := NewServer(&Handlers{}, ready)
	if err != nil {
		panic(err)
	}
	return srv.Handler
}

func TestHealthz(t *testing.T) {
	h := newBareServer(func() bool { return true })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("healthz = %d", rec.Code)
	}
}

func TestReadyzGated(t *testing.T) {
	h := newBareServer(func() bool { return false })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 503 {
		t.Fatalf("readyz = %d, want 503", rec.Code)
	}
}

func TestReadyzNilAlwaysReady(t *testing.T) {
	h := newBareServer(nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 200 {
		t.Fatalf("readyz with nil ready func = %d, want 200", rec.Code)
	}
}

func TestUnknownAPIRouteIs404JSON(t *testing.T) {
	h := newBareServer(func() bool { return true })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/nonexistent", nil))
	if rec.Code != 404 {
		t.Fatalf("unknown api = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
}

func TestUnknownNonAPIRouteIs404(t *testing.T) {
	// The SPA and its history-routing fallback now live in the nginx `web`
	// container (charts/kubesentry-gateway/console); console-api's own mux
	// has no catch-all route left, so an unmatched path is a plain 404.
	h := newBareServer(func() bool { return true })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/policies/foo", nil))
	if rec.Code != 404 {
		t.Fatalf("unknown non-api route = %d, want 404", rec.Code)
	}
}

func TestMigratedRoutesUseNetHTTPPatterns(t *testing.T) {
	srv, err := NewServer(&Handlers{}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	mux := srv.mux

	tests := []struct {
		method  string
		path    string
		pattern string
	}{
		{http.MethodGet, "/api/v1/policies", "GET /api/v1/policies"},
		{http.MethodPost, "/api/v1/policies", "POST /api/v1/policies"},
		{http.MethodGet, "/api/v1/policies/resource-suggestions", "GET /api/v1/policies/resource-suggestions"},
		{http.MethodPost, "/api/v1/policies/validate", "POST /api/v1/policies/validate"},
		{http.MethodGet, "/api/v1/policies/example", "GET /api/v1/policies/{name}"},
		{http.MethodPut, "/api/v1/policies/example", "PUT /api/v1/policies/{name}"},
		{http.MethodDelete, "/api/v1/policies/example", "DELETE /api/v1/policies/{name}"},
		{http.MethodGet, "/api/v1/policies/example/versions", "GET /api/v1/policies/{name}/versions"},
		{http.MethodPost, "/api/v1/policies/example/rollback", "POST /api/v1/policies/{name}/rollback"},
		{http.MethodGet, "/api/v1/policygroups", "GET /api/v1/policygroups"},
		{http.MethodPost, "/api/v1/policygroups", "POST /api/v1/policygroups"},
		{http.MethodGet, "/api/v1/policygroups/example", "GET /api/v1/policygroups/{name}"},
		{http.MethodPut, "/api/v1/policygroups/example", "PUT /api/v1/policygroups/{name}"},
		{http.MethodDelete, "/api/v1/policygroups/example", "DELETE /api/v1/policygroups/{name}"},
		{http.MethodPut, "/api/v1/policygroups/example/enabled", "PUT /api/v1/policygroups/{name}/enabled"},
		{http.MethodGet, "/api/v1/exceptions", "GET /api/v1/exceptions"},
		{http.MethodPost, "/api/v1/exceptions", "POST /api/v1/exceptions"},
		{http.MethodGet, "/api/v1/exceptions/example", "GET /api/v1/exceptions/{name}"},
		{http.MethodPut, "/api/v1/exceptions/example", "PUT /api/v1/exceptions/{name}"},
		{http.MethodDelete, "/api/v1/exceptions/example", "DELETE /api/v1/exceptions/{name}"},
		{http.MethodGet, "/api/v1/schema/fields", "GET /api/v1/schema/fields"},
		{http.MethodGet, "/api/v1/summary", "GET /api/v1/summary"},
		{http.MethodGet, "/api/v1/admission-events", "GET /api/v1/admission-events"},
		{http.MethodGet, "/api/v1/admission-events/example", "GET /api/v1/admission-events/{id}"},
		{http.MethodGet, "/api/v1/admission-events/trend", "GET /api/v1/admission-events/trend"},
		{http.MethodGet, "/api/v1/admission-event-settings", "GET /api/v1/admission-event-settings"},
		{http.MethodPut, "/api/v1/admission-event-settings", "PUT /api/v1/admission-event-settings"},
		{http.MethodGet, "/api/v1/admission-link-incidents", "GET /api/v1/admission-link-incidents"},
		{http.MethodGet, "/api/v1/admission-link-incidents/example", "GET /api/v1/admission-link-incidents/{id}"},
		{http.MethodGet, "/api/v1/clusters", "GET /api/v1/clusters"},
		{http.MethodPost, "/api/v1/clusters", "POST /api/v1/clusters"},
		{http.MethodGet, "/api/v1/clusters/example", "GET /api/v1/clusters/{name}"},
		{http.MethodPut, "/api/v1/clusters/example", "PUT /api/v1/clusters/{name}"},
		{http.MethodDelete, "/api/v1/clusters/example", "DELETE /api/v1/clusters/{name}"},
		{http.MethodPost, "/api/v1/clusters/example/reissue-token", "POST /api/v1/clusters/{name}/reissue-token"},
		{http.MethodGet, "/api/v1/clusters/example/latest-install-params", "GET /api/v1/clusters/{name}/latest-install-params"},
		{http.MethodPost, "/api/v1/clusters/example/uninstall", "POST /api/v1/clusters/{name}/uninstall"},
		{http.MethodGet, "/api/v1/clusters/example/uninstall-tasks", "GET /api/v1/clusters/{name}/uninstall-tasks"},
		{http.MethodPost, "/api/v1/clusters/example/uninstall-tasks/1/retry", "POST /api/v1/clusters/{name}/uninstall-tasks/{taskID}/retry"},
		{http.MethodGet, "/api/v1/install-records", "GET /api/v1/install-records"},
		{http.MethodGet, "/api/v1/distributions", "GET /api/v1/distributions"},
		{http.MethodPost, "/api/v1/distributions", "POST /api/v1/distributions"},
		{http.MethodGet, "/api/v1/distributions/example", "GET /api/v1/distributions/{name}"},
		{http.MethodPut, "/api/v1/distributions/example", "PUT /api/v1/distributions/{name}"},
		{http.MethodDelete, "/api/v1/distributions/example", "DELETE /api/v1/distributions/{name}"},
		{http.MethodGet, "/api/v1/platform/overview", "GET /api/v1/platform/overview"},
		{http.MethodGet, "/api/v1/platform/instances/console/example/trend", "GET /api/v1/platform/instances/{component}/{instance}/trend"},
		{http.MethodGet, "/api/v1/platform/leadership-events", "GET /api/v1/platform/leadership-events"},
		{http.MethodGet, "/api/v1/platform/settings", "GET /api/v1/platform/settings"},
		{http.MethodPut, "/api/v1/platform/settings", "PUT /api/v1/platform/settings"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			_, got := mux.Handler(httptest.NewRequest(tt.method, tt.path, nil))
			if got != tt.pattern {
				t.Fatalf("matched pattern = %q, want %q", got, tt.pattern)
			}
		})
	}
}

func TestNonMigratedRoutesRemainOnOgenFallback(t *testing.T) {
	srv, err := NewServer(&Handlers{}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	mux := srv.mux

	routes := []struct {
		method string
		path   string
	}{}
	if len(routes) != 0 {
		t.Fatalf("fallback route count = %d, want 0", len(routes))
	}
	for _, route := range routes {
		_, pattern := mux.Handler(httptest.NewRequest(route.method, route.path, nil))
		if pattern != "/api/v1/" {
			t.Errorf("%s %s matched %q, want Ogen fallback", route.method, route.path, pattern)
		}
	}
}

func TestOpenAPIDocumentationEndpoints(t *testing.T) {
	h := newBareServer(func() bool { return true })

	spec := httptest.NewRecorder()
	h.ServeHTTP(spec, httptest.NewRequest("GET", "/api/docs/openapi.json", nil))
	if spec.Code != http.StatusOK || spec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("openapi json = %d %q: %s", spec.Code, spec.Header().Get("Content-Type"), spec.Body.String())
	}
	var document map[string]any
	if err := json.Unmarshal(spec.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode openapi json: %v", err)
	}
	if document["openapi"] != "3.1.0" {
		t.Fatalf("openapi version = %v", document["openapi"])
	}
	assertAllRefsResolve(t, document, document)
}

func assertAllRefsResolve(t *testing.T, root, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				ref, ok := child.(string)
				if !ok {
					t.Fatalf("$ref has non-string value %T", child)
				}
				resolveInternalRef(t, root, ref)
			}
			assertAllRefsResolve(t, root, child)
		}
	case []any:
		for _, child := range typed {
			assertAllRefsResolve(t, root, child)
		}
	}
}

func resolveInternalRef(t *testing.T, root any, ref string) {
	t.Helper()
	if !strings.HasPrefix(ref, "#/") {
		t.Fatalf("OpenAPI document contains non-internal ref %q", ref)
	}
	target := root
	for _, rawSegment := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		segment := strings.NewReplacer("~1", "/", "~0", "~").Replace(rawSegment)
		object, ok := target.(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI reference %q traverses non-object segment %q", ref, segment)
		}
		var found bool
		target, found = object[segment]
		if !found {
			t.Fatalf("OpenAPI document contains dangling ref %q", ref)
		}
	}
}
