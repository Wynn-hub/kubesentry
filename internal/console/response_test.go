package console

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteOK(t *testing.T) {
	rec := httptest.NewRecorder()
	writeOK(rec, map[string]string{"k": "v"})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	var env struct {
		Code    int               `json:"code"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != 0 {
		t.Fatalf("code = %d, want 0", env.Code)
	}
	if env.Message != "success" {
		t.Fatalf("message = %q, want success", env.Message)
	}
	if env.Data["k"] != "v" {
		t.Fatalf("data = %+v", env.Data)
	}
}

// writeOK must omit details entirely on success so clients can treat
// presence of details as an error signal.
func TestWriteOKOmitsDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	writeOK(rec, nil)
	body := rec.Body.String()
	if strings.Contains(body, "details") {
		t.Fatalf("success body should not contain details: %s", body)
	}
	if strings.Contains(body, `"data"`) {
		t.Fatalf("nil data should be omitted: %s", body)
	}
}

func TestWriteErrCodeMapping(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantCode int
	}{
		{"bad request", 400, 40000},
		{"not found", 404, 40400},
		{"conflict", 409, 40900},
		{"internal", 500, 50000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeErr(rec, tt.status, "boom")
			if rec.Code != tt.status {
				t.Fatalf("http status = %d, want %d", rec.Code, tt.status)
			}
			var env struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			if env.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d", env.Code, tt.wantCode)
			}
			if env.Message != "boom" {
				t.Fatalf("message = %q, want boom", env.Message)
			}
		})
	}
}

// writeErrData carries structured data alongside an error — the referencedBy
// list on a blocked delete. The spec deliberately keeps data on the error
// envelope rather than folding it into details.
func TestWriteErrDataKeepsData(t *testing.T) {
	rec := httptest.NewRecorder()
	writeErrData(rec, 409, "referenced", map[string][]string{"referencedBy": {"baseline-group"}})
	var env struct {
		Code    int                 `json:"code"`
		Message string              `json:"message"`
		Data    map[string][]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != 40900 {
		t.Fatalf("code = %d, want 40900", env.Code)
	}
	if len(env.Data["referencedBy"]) != 1 || env.Data["referencedBy"][0] != "baseline-group" {
		t.Fatalf("data = %+v", env.Data)
	}
}

// An unmapped status must still produce status*100 rather than zero, so a
// future handler using e.g. 422 doesn't silently report success.
func TestCodeForStatusUnmapped(t *testing.T) {
	if got := codeForStatus(422); got != 42200 {
		t.Fatalf("codeForStatus(422) = %d, want 42200", got)
	}
	if got := codeForStatus(200); got != 0 {
		t.Fatalf("codeForStatus(200) = %d, want 0", got)
	}
}
