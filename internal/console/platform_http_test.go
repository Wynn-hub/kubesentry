package console

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/repository"
)

func TestPlatformRouteMatrix(t *testing.T) {
	actual, err := NewServer(&Handlers{Repos: dbRepos(t)}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	settings := `{"enabled":true,"retentionDays":7}`
	to := time.Now().UTC().Add(-time.Hour)
	from := to.Add(-time.Hour)
	window := fmt.Sprintf("from=%s&to=%s", from.Format(time.RFC3339), to.Format(time.RFC3339))
	ok := routeExpectation{status: 200, code: 0, contentType: "application/json; charset=utf-8", message: "success"}
	bad := routeExpectation{status: 400, code: 40000, contentType: "application/json"}
	notFound := routeExpectation{status: 404, code: 40400, contentType: "application/json; charset=utf-8", message: "cluster not found"}
	tests := []struct {
		name, method, path, body, contentType string
		want                                  routeExpectation
	}{
		{name: "overview defaults", method: http.MethodGet, path: "/api/v1/platform/overview", want: ok},
		{name: "overview query", method: http.MethodGet, path: "/api/v1/platform/overview?page=1&pageSize=10&health=degraded", want: ok},
		{name: "overview bad page", method: http.MethodGet, path: "/api/v1/platform/overview?page=0", want: withMessage(bad, `"page"`)},
		{name: "overview bad health", method: http.MethodGet, path: "/api/v1/platform/overview?health=unknown", want: withMessage(bad, `"health"`)},
		{name: "overview duplicate scalar", method: http.MethodGet, path: "/api/v1/platform/overview?page=1&page=2", want: withMessage(bad, "multiple values")},
		{name: "trend valid", method: http.MethodGet, path: "/api/v1/platform/instances/console-api/console-0/trend?" + window, want: ok},
		{name: "trend member cluster", method: http.MethodGet, path: "/api/v1/platform/instances/operator/op-0/trend?cluster=member", want: notFound},
		{name: "trend bad component", method: http.MethodGet, path: "/api/v1/platform/instances/unknown/x/trend", want: withMessage(bad, "component")},
		{name: "trend bad date", method: http.MethodGet, path: "/api/v1/platform/instances/console-api/x/trend?from=nope", want: withMessage(bad, `"from"`)},
		{name: "trend duplicate cluster", method: http.MethodGet, path: "/api/v1/platform/instances/operator/x/trend?cluster=a&cluster=b", want: withMessage(bad, "multiple values")},
		{name: "trend escaped instance", method: http.MethodGet, path: "/api/v1/platform/instances/console-api/console%200/trend", want: ok},
		{name: "trend escaped slash", method: http.MethodGet, path: "/api/v1/platform/instances/console-api/console%2F0/trend", want: ok},
		{name: "leadership defaults", method: http.MethodGet, path: "/api/v1/platform/leadership-events", want: ok},
		{name: "leadership query", method: http.MethodGet, path: "/api/v1/platform/leadership-events?page=1&pageSize=10&component=gateway&cluster=member&" + window, want: notFound},
		{name: "leadership bad component", method: http.MethodGet, path: "/api/v1/platform/leadership-events?component=unknown", want: withMessage(bad, `"component"`)},
		{name: "leadership bad date", method: http.MethodGet, path: "/api/v1/platform/leadership-events?to=nope", want: withMessage(bad, `"to"`)},
		{name: "settings get", method: http.MethodGet, path: "/api/v1/platform/settings", want: ok},
		{name: "settings update", method: http.MethodPut, path: "/api/v1/platform/settings", body: settings, contentType: "application/json", want: ok},
		{name: "settings missing body", method: http.MethodPut, path: "/api/v1/platform/settings", contentType: "application/json", want: withMessage(bad, "body required")},
		{name: "settings missing enabled", method: http.MethodPut, path: "/api/v1/platform/settings", body: `{"retentionDays":7}`, contentType: "application/json", want: withMessage(bad, "field is required")},
		{name: "settings null enabled", method: http.MethodPut, path: "/api/v1/platform/settings", body: `{"enabled":null,"retentionDays":7}`, contentType: "application/json", want: withMessage(bad, "must be a boolean")},
		{name: "settings missing retention", method: http.MethodPut, path: "/api/v1/platform/settings", body: `{"enabled":true}`, contentType: "application/json", want: withMessage(bad, "field is required")},
		{name: "settings null retention", method: http.MethodPut, path: "/api/v1/platform/settings", body: `{"enabled":true,"retentionDays":null}`, contentType: "application/json", want: withMessage(bad, "must be an integer")},
		{name: "settings trailing JSON", method: http.MethodPut, path: "/api/v1/platform/settings", body: settings + `{}`, contentType: "application/json", want: withMessage(bad, "unexpected trailing data")},
		{name: "settings unknown field accepted", method: http.MethodPut, path: "/api/v1/platform/settings", body: `{"enabled":true,"retentionDays":7,"unknown":true}`, contentType: "application/json", want: ok},
		{name: "settings invalid", method: http.MethodPut, path: "/api/v1/platform/settings", body: `{"enabled":true,"retentionDays":0}`, contentType: "application/json", want: withMessage(bad, "retentionDays")},
		{name: "settings wrong content type", method: http.MethodPut, path: "/api/v1/platform/settings", body: settings, contentType: "text/plain", want: withMessage(bad, "unexpected Content-Type")},
		{name: "HEAD overview", method: http.MethodHead, path: "/api/v1/platform/overview", want: methodNotAllowed("GET")},
		{name: "HEAD trend", method: http.MethodHead, path: "/api/v1/platform/instances/console-api/x/trend", want: methodNotAllowed("GET")},
		{name: "HEAD leadership", method: http.MethodHead, path: "/api/v1/platform/leadership-events", want: methodNotAllowed("GET")},
		{name: "HEAD settings", method: http.MethodHead, path: "/api/v1/platform/settings", want: methodNotAllowed("GET,PUT")},
		{name: "method mismatch", method: http.MethodPost, path: "/api/v1/platform/overview", want: methodNotAllowed("GET")},
		{name: "trailing path", method: http.MethodGet, path: "/api/v1/platform/settings/", want: routeExpectation{status: 404, code: 40400, contentType: "application/json", message: "unknown API route"}},
		{name: "noncanonical path", method: http.MethodGet, path: "/api/v1/platform/../platform/settings", want: routeExpectation{status: 404, code: 40400, contentType: "application/json", message: "unknown API route"}},
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
			switch {
			case tt.name == "overview defaults" || tt.name == "overview query":
				if object["gatewayPlane"] == nil || object["memberPlane"] == nil || object["platformHealth"] == nil {
					t.Fatalf("overview data missing required fields: %v", object)
				}
			case tt.name == "trend valid" || tt.name == "trend escaped instance" || tt.name == "trend escaped slash":
				if object["items"] == nil || object["stepSeconds"] == nil || object["from"] == nil || object["to"] == nil {
					t.Fatalf("trend data missing required fields: %v", object)
				}
			case tt.name == "leadership defaults":
				if object["items"] == nil || object["total"] == nil || object["page"] != float64(1) || object["pageSize"] != float64(20) {
					t.Fatalf("leadership data = %v", object)
				}
			case tt.name == "settings update" || tt.name == "settings unknown field accepted":
				if object["enabled"] != true || object["retentionDays"] != float64(7) {
					t.Fatalf("updated settings = %v", object)
				}
			case tt.name == "settings get":
				if object["enabled"] == nil || object["retentionDays"] == nil || object["maxRetentionDays"] == nil {
					t.Fatalf("settings data missing required fields: %v", object)
				}
			}
		})
	}
}

func TestPlatformSettingsRepositoryFailure(t *testing.T) {
	actualDB := newTestDB(t)
	actual, err := NewServer(&Handlers{Repos: repository.New(actualDB)}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := actualDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	got := servePolicyRequest(actual.Handler, http.MethodGet, "/api/v1/platform/settings", "", "")
	assertRouteExpectation(t, got, routeExpectation{status: 500, code: 50000, contentType: "application/json; charset=utf-8", message: "request failed"})
}
