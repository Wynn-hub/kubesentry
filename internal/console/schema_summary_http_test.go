package console

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/repository"
)

func TestSchemaAndSummaryNetHTTPMatchesOgenRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	handlers := &Handlers{Repos: repos}
	fetcher := &fakeSchemaFetcher{fields: []FieldNode{{Name: "spec", Type: "object"}}}
	handlers.SchemaFetcher = fetcher
	actual, err := NewServer(handlers, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	oracle := newOgenOracle(t, repos, handlers.fieldSchema)
	assertParity := func(t *testing.T, method, path string) {
		t.Helper()
		want := servePolicyRequest(oracle, method, path, "", "")
		got := servePolicyRequest(actual.Handler, method, path, "", "")
		if got.Code != want.Code {
			t.Fatalf("status = %d, want %d; body=%s; want body=%s", got.Code, want.Code, got.Body.String(), want.Body.String())
		}
		for _, header := range []string{"Content-Type", "Allow"} {
			if got.Header().Get(header) != want.Header().Get(header) {
				t.Fatalf("%s = %q, want %q", header, got.Header().Get(header), want.Header().Get(header))
			}
		}
		assertSameJSON(t, got.Body.Bytes(), want.Body.Bytes())
	}

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "schema valid", method: http.MethodGet, path: "/api/v1/schema/fields?group=&version=v1&resource=pods"},
		{name: "schema optional defaults", method: http.MethodGet, path: "/api/v1/schema/fields?version=v1&resource=pods"},
		{name: "schema refresh", method: http.MethodGet, path: "/api/v1/schema/fields?group=&version=v1&resource=pods&refresh=true"},
		{name: "schema missing required", method: http.MethodGet, path: "/api/v1/schema/fields?group=apps"},
		{name: "schema duplicate scalar", method: http.MethodGet, path: "/api/v1/schema/fields?version=v1&version=v2&resource=pods"},
		{name: "schema duplicate optional group", method: http.MethodGet, path: "/api/v1/schema/fields?group=&group=apps&version=v1&resource=pods"},
		{name: "schema duplicate optional refresh", method: http.MethodGet, path: "/api/v1/schema/fields?version=v1&resource=pods&refresh=true&refresh=false"},
		{name: "schema invalid bool", method: http.MethodGet, path: "/api/v1/schema/fields?version=v1&resource=pods&refresh=maybe"},
		{name: "schema HEAD does not imply GET", method: http.MethodHead, path: "/api/v1/schema/fields?version=v1&resource=pods"},
		{name: "summary success", method: http.MethodGet, path: "/api/v1/summary"},
		{name: "summary ignores query", method: http.MethodGet, path: "/api/v1/summary?unknown=true"},
		{name: "summary HEAD does not imply GET", method: http.MethodHead, path: "/api/v1/summary"},
		{name: "schema method mismatch", method: http.MethodPost, path: "/api/v1/schema/fields"},
		{name: "summary method mismatch", method: http.MethodPost, path: "/api/v1/summary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertParity(t, tt.method, tt.path)
		})
	}

	fetcher.err = errors.New("schema unavailable")
	assertParity(t, http.MethodGet, "/api/v1/schema/fields?version=v1&resource=deployments")
}

func TestSummaryNetHTTPMatchesOgenRepositoryFailure(t *testing.T) {
	db := newTestDB(t)
	repos := repository.New(db)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	oracle := newOgenOracle(t, repos)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	want := servePolicyRequest(oracle, http.MethodGet, "/api/v1/summary", "", "")
	got := servePolicyRequest(actual.Handler, http.MethodGet, "/api/v1/summary", "", "")
	if got.Code != want.Code || got.Header().Get("Content-Type") != want.Header().Get("Content-Type") {
		t.Fatalf("status/content-type = %d/%q, want %d/%q", got.Code, got.Header().Get("Content-Type"), want.Code, want.Header().Get("Content-Type"))
	}
	assertSameJSON(t, got.Body.Bytes(), want.Body.Bytes())
}
