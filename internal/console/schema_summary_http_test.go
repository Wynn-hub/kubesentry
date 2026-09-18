package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/repository"
)

func TestSchemaAndSummaryRouteMatrix(t *testing.T) {
	repos := dbRepos(t)
	handlers := &Handlers{Repos: repos}
	fetcher := &fakeSchemaFetcher{fields: []FieldNode{{Name: "spec", Type: "object"}}}
	handlers.SchemaFetcher = fetcher
	actual, err := NewServer(handlers, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	assertResponse := func(t *testing.T, method, path string, status int, message string, plainJSON bool) map[string]any {
		t.Helper()
		got := servePolicyRequest(actual.Handler, method, path, "", "")
		if got.Code != status {
			t.Fatalf("status = %d, want %d; body=%s", got.Code, status, got.Body.String())
		}
		if status == http.StatusMethodNotAllowed {
			if got.Header().Get("Allow") != "GET" || got.Header().Get("Content-Type") != "" || got.Body.Len() != 0 {
				t.Fatalf("405 Allow/Content-Type/body = %q/%q/%q", got.Header().Get("Allow"), got.Header().Get("Content-Type"), got.Body.String())
			}
			return nil
		}
		contentType := "application/json; charset=utf-8"
		if plainJSON {
			contentType = "application/json"
		}
		if got.Header().Get("Content-Type") != contentType {
			t.Fatalf("Content-Type = %q, want %q", got.Header().Get("Content-Type"), contentType)
		}
		var envelope map[string]any
		if err := json.Unmarshal(got.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("response is not JSON: %v; body=%s", err, got.Body.String())
		}
		code := float64(status * 100)
		if status == http.StatusOK {
			code = 0
		}
		if envelope["code"] != code || !strings.Contains(envelope["message"].(string), message) {
			t.Fatalf("envelope = %v, want code %.0f and message containing %q", envelope, code, message)
		}
		return envelope
	}

	tests := []struct {
		name   string
		method string
		path   string
		status int
		msg    string
		plain  bool
	}{
		{name: "schema valid", method: http.MethodGet, path: "/api/v1/schema/fields?group=&version=v1&resource=pods", status: 200, msg: "success"},
		{name: "schema optional defaults", method: http.MethodGet, path: "/api/v1/schema/fields?version=v1&resource=pods", status: 200, msg: "success"},
		{name: "schema refresh", method: http.MethodGet, path: "/api/v1/schema/fields?group=&version=v1&resource=pods&refresh=true", status: 200, msg: "success"},
		{name: "schema missing required", method: http.MethodGet, path: "/api/v1/schema/fields?group=apps", status: 400, msg: "version", plain: true},
		{name: "schema duplicate scalar", method: http.MethodGet, path: "/api/v1/schema/fields?version=v1&version=v2&resource=pods", status: 400, msg: "multiple values", plain: true},
		{name: "schema duplicate optional group", method: http.MethodGet, path: "/api/v1/schema/fields?group=&group=apps&version=v1&resource=pods", status: 400, msg: "multiple values", plain: true},
		{name: "schema duplicate optional refresh", method: http.MethodGet, path: "/api/v1/schema/fields?version=v1&resource=pods&refresh=true&refresh=false", status: 400, msg: "multiple values", plain: true},
		{name: "schema invalid bool", method: http.MethodGet, path: "/api/v1/schema/fields?version=v1&resource=pods&refresh=maybe", status: 400, msg: "invalid syntax", plain: true},
		{name: "schema HEAD does not imply GET", method: http.MethodHead, path: "/api/v1/schema/fields?version=v1&resource=pods", status: 405},
		{name: "summary success", method: http.MethodGet, path: "/api/v1/summary", status: 200, msg: "success"},
		{name: "summary ignores query", method: http.MethodGet, path: "/api/v1/summary?unknown=true", status: 200, msg: "success"},
		{name: "summary HEAD does not imply GET", method: http.MethodHead, path: "/api/v1/summary", status: 405},
		{name: "schema method mismatch", method: http.MethodPost, path: "/api/v1/schema/fields", status: 405},
		{name: "summary method mismatch", method: http.MethodPost, path: "/api/v1/summary", status: 405},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := assertResponse(t, tt.method, tt.path, tt.status, tt.msg, tt.plain)
			if tt.status != http.StatusOK {
				return
			}
			switch {
			case strings.HasPrefix(tt.name, "schema"):
				fields, ok := envelope["data"].([]any)
				if !ok || len(fields) != 1 || fields[0].(map[string]any)["name"] != "spec" {
					t.Fatalf("schema data = %v, want one spec field", envelope["data"])
				}
			case strings.HasPrefix(tt.name, "summary"):
				data, ok := envelope["data"].(map[string]any)
				if !ok || data["totals"] == nil || data["clusterPhases"] == nil || data["groups"] == nil {
					t.Fatalf("summary data = %v, want totals/clusterPhases/groups", envelope["data"])
				}
			}
		})
	}

	fetcher.err = errors.New("schema unavailable")
	assertResponse(t, http.MethodGet, "/api/v1/schema/fields?version=v1&resource=deployments", http.StatusBadGateway, "schema unavailable", false)
}

func TestSummaryRepositoryFailure(t *testing.T) {
	db := newTestDB(t)
	repos := repository.New(db)
	actual, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
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

	got := servePolicyRequest(actual.Handler, http.MethodGet, "/api/v1/summary", "", "")
	if got.Code != http.StatusInternalServerError || got.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("status/content-type = %d/%q", got.Code, got.Header().Get("Content-Type"))
	}
	var envelope map[string]any
	err = json.Unmarshal(got.Body.Bytes(), &envelope)
	message, _ := envelope["message"].(string)
	if err != nil || envelope["code"] != float64(50000) || !strings.Contains(message, "database is closed") {
		t.Fatalf("error envelope = %v, parse error = %v", envelope, err)
	}
	if _, ok := envelope["data"]; ok {
		t.Fatalf("error response unexpectedly contains data: %s", got.Body.String())
	}
}
