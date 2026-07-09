package console

import (
	"errors"
	"net/http"
	"testing"
)

type fakeSchemaFetcher struct {
	calls  int
	fields []FieldNode
	err    error
}

func (f *fakeSchemaFetcher) FieldsFor(group, version, resource string) ([]FieldNode, error) {
	f.calls++
	return f.fields, f.err
}

func TestGetFieldSchema_MissingParams(t *testing.T) {
	h, _ := newTestServer(t)
	rec, env := doRequest(t, h, "GET", "/api/v1/schema/fields?group=apps", nil)
	if rec.Code != 400 || env.Success {
		t.Fatalf("code=%d env=%+v, want 400", rec.Code, env)
	}
}

func TestGetFieldSchema_CachesUntilRefresh(t *testing.T) {
	handler, h := newTestServerWithHandlers(t)
	fetcher := &fakeSchemaFetcher{fields: []FieldNode{{Name: "spec", Type: "object"}}}
	h.SchemaFetcher = fetcher

	rec, env := doRequest(t, handler, "GET", "/api/v1/schema/fields?group=&version=v1&resource=pods", nil)
	if rec.Code != 200 || !env.Success {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
	doRequest(t, handler, "GET", "/api/v1/schema/fields?group=&version=v1&resource=pods", nil)
	if fetcher.calls != 1 {
		t.Fatalf("calls = %d, want 1 (second request should hit cache)", fetcher.calls)
	}

	doRequest(t, handler, "GET", "/api/v1/schema/fields?group=&version=v1&resource=pods&refresh=true", nil)
	if fetcher.calls != 2 {
		t.Fatalf("calls = %d, want 2 (refresh=true bypasses cache)", fetcher.calls)
	}
}

func TestGetFieldSchema_FetcherError(t *testing.T) {
	handler, h := newTestServerWithHandlers(t)
	h.SchemaFetcher = &fakeSchemaFetcher{err: errors.New("boom")}

	rec, env := doRequest(t, handler, "GET", "/api/v1/schema/fields?group=&version=v1&resource=pods", nil)
	if rec.Code != http.StatusBadGateway || env.Success {
		t.Fatalf("code=%d env=%+v, want 502", rec.Code, env)
	}
}
