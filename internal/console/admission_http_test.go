package console

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func TestAdmissionNetHTTPMatchesOgenRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	oracle := newOgenOracle(t, repos)
	settings := `{"defaultRetentionDays":14,"retryEnabled":true,"maxRetryAttempts":3,"queueCapacity":1024,"incidentRetentionDays":7,"observationEnabled":false,"storagePolicies":[]}`
	tests := []struct {
		name, method, path, body, contentType string
	}{
		{name: "events defaults", method: http.MethodGet, path: "/api/v1/admission-events"},
		{name: "events filters", method: http.MethodGet, path: "/api/v1/admission-events?page=1&pageSize=10&sortBy=verdict&sortOrder=asc&cluster=a,b&verdict=audit,deny&policy=p&namespace=n&gvk=pods&resourceName=r&username=u&from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z"},
		{name: "events bad page", method: http.MethodGet, path: "/api/v1/admission-events?page=0"},
		{name: "events bad page size", method: http.MethodGet, path: "/api/v1/admission-events?pageSize=11"},
		{name: "events bad sort by", method: http.MethodGet, path: "/api/v1/admission-events?sortBy=name"},
		{name: "events bad sort order", method: http.MethodGet, path: "/api/v1/admission-events?sortOrder=sideways"},
		{name: "events bad enum", method: http.MethodGet, path: "/api/v1/admission-events?verdict=allow"},
		{name: "events duplicate scalar", method: http.MethodGet, path: "/api/v1/admission-events?page=1&page=2"},
		{name: "events duplicate CSV", method: http.MethodGet, path: "/api/v1/admission-events?cluster=a&cluster=b"},
		{name: "events invalid optional date", method: http.MethodGet, path: "/api/v1/admission-events?from=nope"},
		{name: "event bad id", method: http.MethodGet, path: "/api/v1/admission-events/nope"},
		{name: "event escaped numeric id", method: http.MethodGet, path: "/api/v1/admission-events/%31"},
		{name: "event escaped slash", method: http.MethodGet, path: "/api/v1/admission-events/1%2F2"},
		{name: "event not found", method: http.MethodGet, path: "/api/v1/admission-events/999999"},
		{name: "trend valid", method: http.MethodGet, path: "/api/v1/admission-events/trend?from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z"},
		{name: "trend missing", method: http.MethodGet, path: "/api/v1/admission-events/trend?from=2026-01-01T00:00:00Z"},
		{name: "trend invalid", method: http.MethodGet, path: "/api/v1/admission-events/trend?from=nope&to=2026-01-02T00:00:00Z"},
		{name: "settings get", method: http.MethodGet, path: "/api/v1/admission-event-settings"},
		{name: "settings update", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: settings, contentType: "application/json"},
		{name: "settings missing body", method: http.MethodPut, path: "/api/v1/admission-event-settings", contentType: "application/json"},
		{name: "settings trailing JSON", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: settings + `{}`, contentType: "application/json"},
		{name: "settings unknown field accepted", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: settings[:len(settings)-1] + `,"unknown":true}`, contentType: "application/json"},
		{name: "settings DTO validation", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: `{}`, contentType: "application/json"},
		{name: "settings wrong content type", method: http.MethodPut, path: "/api/v1/admission-event-settings", body: settings, contentType: "text/plain"},
		{name: "incidents defaults", method: http.MethodGet, path: "/api/v1/admission-link-incidents"},
		{name: "incidents filters", method: http.MethodGet, path: "/api/v1/admission-link-incidents?page=1&pageSize=10&cluster=a&component=webhook,gateway&reason=timeout&status=active,closed&from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z"},
		{name: "incidents bad enum", method: http.MethodGet, path: "/api/v1/admission-link-incidents?component=console"},
		{name: "incidents duplicate CSV", method: http.MethodGet, path: "/api/v1/admission-link-incidents?status=active&status=closed"},
		{name: "incidents invalid optional date", method: http.MethodGet, path: "/api/v1/admission-link-incidents?to=nope"},
		{name: "incident bad id", method: http.MethodGet, path: "/api/v1/admission-link-incidents/nope"},
		{name: "incident not found", method: http.MethodGet, path: "/api/v1/admission-link-incidents/999999"},
		{name: "HEAD events", method: http.MethodHead, path: "/api/v1/admission-events"},
		{name: "HEAD event detail", method: http.MethodHead, path: "/api/v1/admission-events/1"},
		{name: "HEAD trend", method: http.MethodHead, path: "/api/v1/admission-events/trend"},
		{name: "HEAD settings", method: http.MethodHead, path: "/api/v1/admission-event-settings"},
		{name: "HEAD incidents", method: http.MethodHead, path: "/api/v1/admission-link-incidents"},
		{name: "HEAD incident detail", method: http.MethodHead, path: "/api/v1/admission-link-incidents/1"},
		{name: "method mismatch", method: http.MethodPost, path: "/api/v1/admission-events"},
		{name: "trend method mismatch", method: http.MethodPost, path: "/api/v1/admission-events/trend"},
		{name: "incident method mismatch", method: http.MethodDelete, path: "/api/v1/admission-link-incidents/1"},
		{name: "trailing path", method: http.MethodGet, path: "/api/v1/admission-events/1/"},
		{name: "noncanonical path", method: http.MethodGet, path: "/api/v1/admission-events/../admission-events/1"},
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
			if tt.method == http.MethodPut && tt.path == "/api/v1/admission-event-settings" && got.Code == http.StatusOK {
				var gotEnvelope, wantEnvelope map[string]any
				if err := json.Unmarshal(got.Body.Bytes(), &gotEnvelope); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(want.Body.Bytes(), &wantEnvelope); err != nil {
					t.Fatal(err)
				}
				gotData, gotOK := gotEnvelope["data"].(map[string]any)
				wantData, wantOK := wantEnvelope["data"].(map[string]any)
				if !gotOK || !wantOK {
					t.Fatalf("settings response data is not an object")
				}
				delete(gotData, "storagePolicyVersion")
				delete(wantData, "storagePolicyVersion")
				gotJSON, err := json.Marshal(gotEnvelope)
				if err != nil {
					t.Fatal(err)
				}
				wantJSON, err := json.Marshal(wantEnvelope)
				if err != nil {
					t.Fatal(err)
				}
				assertSameJSON(t, gotJSON, wantJSON)
				return
			}
			assertSameJSON(t, got.Body.Bytes(), want.Body.Bytes())
		})
	}
}

func TestAdmissionServiceUnavailableStatuses(t *testing.T) {
	for name, response := range map[string]any{
		"settings":  &dto.GetAdmissionEventSettingsServiceUnavailable{},
		"incidents": &dto.ListAdmissionLinkIncidentsServiceUnavailable{},
		"incident":  &dto.GetAdmissionLinkIncidentServiceUnavailable{},
	} {
		t.Run(name, func(t *testing.T) {
			if got := responseStatus(response); got != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", got, http.StatusServiceUnavailable)
			}
		})
	}
}
