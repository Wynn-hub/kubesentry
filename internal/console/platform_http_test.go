package console

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/repository"
)

func TestPlatformNetHTTPMatchesOgenRouteMatrix(t *testing.T) {
	actual, err := NewServer(&Handlers{Repos: dbRepos(t)}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	oracle := newOgenOracle(t, dbRepos(t))
	settings := `{"enabled":true,"retentionDays":14}`
	tests := []struct{ name, method, path, body, contentType string }{
		{name: "overview defaults", method: http.MethodGet, path: "/api/v1/platform/overview"},
		{name: "overview query", method: http.MethodGet, path: "/api/v1/platform/overview?page=1&pageSize=10&health=degraded"},
		{name: "overview bad page", method: http.MethodGet, path: "/api/v1/platform/overview?page=0"},
		{name: "overview bad health", method: http.MethodGet, path: "/api/v1/platform/overview?health=unknown"},
		{name: "overview duplicate scalar", method: http.MethodGet, path: "/api/v1/platform/overview?page=1&page=2"},
		{name: "trend valid", method: http.MethodGet, path: "/api/v1/platform/instances/console-api/console-0/trend?from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z"},
		{name: "trend member cluster", method: http.MethodGet, path: "/api/v1/platform/instances/operator/op-0/trend?cluster=member"},
		{name: "trend bad component", method: http.MethodGet, path: "/api/v1/platform/instances/unknown/x/trend"},
		{name: "trend bad date", method: http.MethodGet, path: "/api/v1/platform/instances/console-api/x/trend?from=nope"},
		{name: "trend duplicate cluster", method: http.MethodGet, path: "/api/v1/platform/instances/operator/x/trend?cluster=a&cluster=b"},
		{name: "trend escaped instance", method: http.MethodGet, path: "/api/v1/platform/instances/console-api/console%200/trend"},
		{name: "trend escaped slash", method: http.MethodGet, path: "/api/v1/platform/instances/console-api/console%2F0/trend"},
		{name: "leadership defaults", method: http.MethodGet, path: "/api/v1/platform/leadership-events"},
		{name: "leadership query", method: http.MethodGet, path: "/api/v1/platform/leadership-events?page=1&pageSize=10&component=gateway&cluster=member&from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z"},
		{name: "leadership bad component", method: http.MethodGet, path: "/api/v1/platform/leadership-events?component=unknown"},
		{name: "leadership bad date", method: http.MethodGet, path: "/api/v1/platform/leadership-events?to=nope"},
		{name: "settings get", method: http.MethodGet, path: "/api/v1/platform/settings"},
		{name: "settings update", method: http.MethodPut, path: "/api/v1/platform/settings", body: settings, contentType: "application/json"},
		{name: "settings missing body", method: http.MethodPut, path: "/api/v1/platform/settings", contentType: "application/json"},
		{name: "settings trailing JSON", method: http.MethodPut, path: "/api/v1/platform/settings", body: settings + `{}`, contentType: "application/json"},
		{name: "settings unknown field accepted", method: http.MethodPut, path: "/api/v1/platform/settings", body: `{"enabled":true,"retentionDays":14,"unknown":true}`, contentType: "application/json"},
		{name: "settings invalid", method: http.MethodPut, path: "/api/v1/platform/settings", body: `{"enabled":true,"retentionDays":0}`, contentType: "application/json"},
		{name: "settings wrong content type", method: http.MethodPut, path: "/api/v1/platform/settings", body: settings, contentType: "text/plain"},
		{name: "HEAD overview", method: http.MethodHead, path: "/api/v1/platform/overview"},
		{name: "HEAD trend", method: http.MethodHead, path: "/api/v1/platform/instances/console-api/x/trend"},
		{name: "HEAD leadership", method: http.MethodHead, path: "/api/v1/platform/leadership-events"},
		{name: "HEAD settings", method: http.MethodHead, path: "/api/v1/platform/settings"},
		{name: "method mismatch", method: http.MethodPost, path: "/api/v1/platform/overview"},
		{name: "trailing path", method: http.MethodGet, path: "/api/v1/platform/settings/"},
		{name: "noncanonical path", method: http.MethodGet, path: "/api/v1/platform/../platform/settings"},
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

func TestPlatformSettingsNetHTTPMatchesOgenRepositoryFailure(t *testing.T) {
	actualDB := newTestDB(t)
	actual, err := NewServer(&Handlers{Repos: repository.New(actualDB)}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	oracleDB := newTestDB(t)
	oracle := newOgenOracle(t, repository.New(oracleDB))
	for _, db := range []interface{ DB() (*sql.DB, error) }{actualDB, oracleDB} {
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatal(err)
		}
	}

	want := servePolicyRequest(oracle, http.MethodGet, "/api/v1/platform/settings", "", "")
	got := servePolicyRequest(actual.Handler, http.MethodGet, "/api/v1/platform/settings", "", "")
	if got.Code != want.Code || got.Header().Get("Content-Type") != want.Header().Get("Content-Type") {
		t.Fatalf("status/content-type = %d/%q, want %d/%q", got.Code, got.Header().Get("Content-Type"), want.Code, want.Header().Get("Content-Type"))
	}
	assertSameJSON(t, got.Body.Bytes(), want.Body.Bytes())
}
