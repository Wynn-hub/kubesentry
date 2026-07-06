package console

import (
	"encoding/json"
	"net/http/httptest"
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
		Success bool              `json:"success"`
		Data    map[string]string `json:"data"`
		Error   *string           `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Success || env.Data["k"] != "v" || env.Error != nil {
		t.Fatalf("envelope = %+v", env)
	}
}

func TestWriteErr(t *testing.T) {
	rec := httptest.NewRecorder()
	writeErr(rec, 404, "not found")
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var env struct {
		Success bool    `json:"success"`
		Error   *string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Success || env.Error == nil || *env.Error != "not found" {
		t.Fatalf("envelope = %+v", env)
	}
}
