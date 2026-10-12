package console

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

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

func TestWriteCodedErr(t *testing.T) {
	rec := httptest.NewRecorder()
	writeCodedErr(rec, 409, "PASSWORD_REUSED", "password was used recently")
	var env Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.ErrorCode != "PASSWORD_REUSED" {
		t.Fatalf("errorCode = %q, want PASSWORD_REUSED", env.ErrorCode)
	}
}
