package console

import (
	"net/http"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func TestAdmissionRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	settings := `{"defaultRetentionDays":14,"retryEnabled":true,"maxRetryAttempts":3,"queueCapacity":1024,"incidentRetentionDays":7,"observationEnabled":false,"storagePolicies":[]}`
	ok := routeExpectation{status: 200, code: 0, contentType: "application/json; charset=utf-8", message: "success"}
	bad := routeExpectation{status: 400, code: 40000, contentType: "application/json"}
	notFound := routeExpectation{status: 404, code: 40400, contentType: "application/json; charset=utf-8"}
	unknownRoute := routeExpectation{status: 404, code: 40400, contentType: "application/json", message: "unknown API route"}
	tests := []struct {
		name, method, path, body, contentType string
		want                                  routeExpectation
	}{
		{name: "events defaults", method: http.MethodGet, path: "/api/v1/admission-events", want: ok},
		{name: "events filters", method: http.MethodGet, path: "/api/v1/admission-events?page=1&pageSize=10&sortBy=verdict&sortOrder=asc&cluster=a,b&verdict=audit,deny&policy=p&namespace=n&gvk=pods&resourceName=r&username=u&from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z", want: ok},
		{name: "events bad page", method: http.MethodGet, path: "/api/v1/admission-events?page=0", want: withMessage(bad, `"page"`)},
		{name: "events bad page size", method: http.MethodGet, path: "/api/v1/admission-events?pageSize=11", want: withMessage(bad, `"pageSize"`)},
		{name: "events bad sort by", method: http.MethodGet, path: "/api/v1/admission-events?sortBy=name", want: withMessage(bad, `"sortBy"`)},
		{name: "events bad sort order", method: http.MethodGet, path: "/api/v1/admission-events?sortOrder=sideways", want: withMessage(bad, `"sortOrder"`)},
		{name: "events bad enum", method: http.MethodGet, path: "/api/v1/admission-events?verdict=allow", want: withMessage(bad, `"verdict"`)},
		{name: "events duplicate scalar", method: http.MethodGet, path: "/api/v1/admission-events?page=1&page=2", want: withMessage(bad, "multiple values")},
		{name: "events duplicate CSV", method: http.MethodGet, path: "/api/v1/admission-events?cluster=a&cluster=b", want: withMessage(bad, `"cluster"`)},
		{name: "events invalid optional date", method: http.MethodGet, path: "/api/v1/admission-events?from=nope", want: withMessage(bad, `"from"`)},
		{name: "event bad id", method: http.MethodGet, path: "/api/v1/admission-events/nope", want: withMessage(bad, `"id"`)},
		{name: "event escaped numeric id", method: http.MethodGet, path: "/api/v1/admission-events/%31", want: withMessage(notFound, "admission event not found")},
		{name: "event escaped slash", method: http.MethodGet, path: "/api/v1/admission-events/1%2F2", want: withMessage(bad, `"id"`)},
		{name: "event not found", method: http.MethodGet, path: "/api/v1/admission-events/999999", want: withMessage(notFound, "admission event not found")},
		{name: "trend valid", method: http.MethodGet, path: "/api/v1/admission-events/trend?from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z", want: ok},
		{name: "trend missing", method: http.MethodGet, path: "/api/v1/admission-events/trend?from=2026-01-01T00:00:00Z", want: withMessage(bad, `"to"`)},
		{name: "trend invalid", method: http.MethodGet, path: "/api/v1/admission-events/trend?from=nope&to=2026-01-02T00:00:00Z", want: withMessage(bad, `"from"`)},
		{name: "settings get", method: http.MethodGet, path: "/api/v1/admission-event-settings", want: ok},
		{name: "settings update", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: settings, contentType: "application/json", want: ok},
		{name: "settings missing body", method: http.MethodPut, path: "/api/v1/admission-event-settings", contentType: "application/json", want: withMessage(bad, "body required")},
		{name: "settings trailing JSON", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: settings + `{}`, contentType: "application/json", want: withMessage(bad, "unexpected trailing data")},
		{name: "settings unknown field accepted", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: settings[:len(settings)-1] + `,"unknown":true}`, contentType: "application/json", want: ok},
		{name: "settings DTO validation", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: `{}`, contentType: "application/json", want: withMessage(bad, "required")},
		{name: "settings null optional", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: `{"defaultRetentionDays":14,"retryEnabled":null}`, contentType: "application/json", want: withMessage(bad, "retryEnabled")},
		{name: "settings missing policy field", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: `{"defaultRetentionDays":14,"storagePolicies":[{"verdicts":[],"clusters":[],"gvks":[],"namespaces":[]}]}`, contentType: "application/json", want: withMessage(bad, "matchMode")},
		{name: "settings wrong content type", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: settings, contentType: "text/plain", want: withMessage(bad, "unexpected Content-Type")},
		{name: "incidents defaults", method: http.MethodGet, path: "/api/v1/admission-link-incidents", want: ok},
		{name: "incidents filters", method: http.MethodGet, path: "/api/v1/admission-link-incidents?page=1&pageSize=10&cluster=a&component=webhook,gateway&reason=timeout&status=active,closed&from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z", want: routeExpectation{status: 400, code: 40000, contentType: "application/json; charset=utf-8", message: "invalid reason"}},
		{name: "incidents bad enum", method: http.MethodGet, path: "/api/v1/admission-link-incidents?component=console", want: withMessage(bad, `"component"`)},
		{name: "incidents duplicate CSV", method: http.MethodGet, path: "/api/v1/admission-link-incidents?status=active&status=closed", want: withMessage(bad, `"status"`)},
		{name: "incidents invalid optional date", method: http.MethodGet, path: "/api/v1/admission-link-incidents?to=nope", want: withMessage(bad, `"to"`)},
		{name: "incident bad id", method: http.MethodGet, path: "/api/v1/admission-link-incidents/nope", want: withMessage(bad, `"id"`)},
		{name: "incident not found", method: http.MethodGet, path: "/api/v1/admission-link-incidents/999999", want: withMessage(notFound, "not found")},
		{name: "HEAD events", method: http.MethodHead, path: "/api/v1/admission-events", want: methodNotAllowed("GET")},
		{name: "HEAD event detail", method: http.MethodHead, path: "/api/v1/admission-events/1", want: methodNotAllowed("GET")},
		{name: "HEAD trend", method: http.MethodHead, path: "/api/v1/admission-events/trend", want: methodNotAllowed("GET")},
		{name: "HEAD settings", method: http.MethodHead, path: "/api/v1/admission-event-settings", want: methodNotAllowed("GET,PUT")},
		{name: "HEAD incidents", method: http.MethodHead, path: "/api/v1/admission-link-incidents", want: methodNotAllowed("GET")},
		{name: "HEAD incident detail", method: http.MethodHead, path: "/api/v1/admission-link-incidents/1", want: methodNotAllowed("GET")},
		{name: "method mismatch", method: http.MethodPost, path: "/api/v1/admission-events", want: methodNotAllowed("GET")},
		{name: "trend method mismatch", method: http.MethodPost, path: "/api/v1/admission-events/trend", want: methodNotAllowed("GET")},
		{name: "incident method mismatch", method: http.MethodDelete, path: "/api/v1/admission-link-incidents/1", want: methodNotAllowed("GET")},
		{name: "trailing path", method: http.MethodGet, path: "/api/v1/admission-events/1/", want: unknownRoute},
		{name: "noncanonical path", method: http.MethodGet, path: "/api/v1/admission-events/../admission-events/1", want: unknownRoute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := servePolicyRequest(actual.Handler, tt.method, tt.path, tt.body, tt.contentType)
			data := assertRouteExpectation(t, got, tt.want)
			if tt.want.status != http.StatusOK {
				return
			}
			switch {
			case tt.name == "trend valid":
				if _, ok := data.([]any); !ok {
					t.Fatalf("trend data = %T, want array", data)
				}
			case tt.name == "settings get" || tt.name == "settings update" || tt.name == "settings unknown field accepted":
				settingsData, ok := data.(map[string]any)
				_, retentionOK := settingsData["defaultRetentionDays"].(float64)
				_, retryOK := settingsData["retryEnabled"].(bool)
				_, attemptsOK := settingsData["maxRetryAttempts"].(float64)
				_, capacityOK := settingsData["queueCapacity"].(float64)
				_, incidentRetentionOK := settingsData["incidentRetentionDays"].(float64)
				_, observationOK := settingsData["observationEnabled"].(bool)
				policies, policiesOK := settingsData["storagePolicies"].([]any)
				version, versionOK := settingsData["storagePolicyVersion"].(float64)
				if !ok || !retentionOK || !retryOK || !attemptsOK || !capacityOK || !incidentRetentionOK || !observationOK || !policiesOK || !versionOK || version < 1 {
					t.Fatalf("settings data = %v, want configured object", data)
				}
				if tt.name != "settings get" && (settingsData["defaultRetentionDays"] != float64(14) || settingsData["retryEnabled"] != true || settingsData["maxRetryAttempts"] != float64(3) || settingsData["queueCapacity"] != float64(1024) || settingsData["incidentRetentionDays"] != float64(7) || settingsData["observationEnabled"] != false || len(policies) != 0) {
					t.Fatalf("updated settings = %v, want request values", settingsData)
				}
			default:
				page, ok := data.(map[string]any)
				items, itemsOK := page["items"].([]any)
				if !ok || !itemsOK || len(items) != 0 || page["total"] != float64(0) || page["page"] != float64(1) {
					t.Fatalf("page data = %v, want pagination object", data)
				}
				wantSize := float64(20)
				if tt.name == "events filters" || tt.name == "incidents filters" {
					wantSize = 10
				}
				if page["pageSize"] != wantSize {
					t.Fatalf("pageSize = %v, want %v", page["pageSize"], wantSize)
				}
			}
		})
	}
}

func TestAdmissionServiceUnavailableStatuses(t *testing.T) {
	for name, response := range map[string]any{
		"settings":  &dto.AdmissionEventServiceUnavailableHTTPError{},
		"incidents": &dto.AdmissionLinkIncidentServiceUnavailableHTTPError{},
		"incident":  &dto.AdmissionLinkIncidentServiceUnavailableHTTPError{},
	} {
		t.Run(name, func(t *testing.T) {
			if got := responseStatus(response); got != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", got, http.StatusServiceUnavailable)
			}
		})
	}
}
