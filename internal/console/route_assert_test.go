package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type routeExpectation struct {
	status, code                int
	contentType, allow, message string
}

func withMessage(expectation routeExpectation, message string) routeExpectation {
	expectation.message = message
	return expectation
}

func methodNotAllowed(allow string) routeExpectation {
	return routeExpectation{status: http.StatusMethodNotAllowed, allow: allow}
}

func assertRouteExpectation(t *testing.T, got *httptest.ResponseRecorder, want routeExpectation) any {
	t.Helper()
	if got.Code != want.status {
		t.Fatalf("status = %d, want %d; body=%s", got.Code, want.status, got.Body.String())
	}
	if got.Header().Get("Content-Type") != want.contentType || got.Header().Get("Allow") != want.allow {
		t.Fatalf("Content-Type/Allow = %q/%q, want %q/%q", got.Header().Get("Content-Type"), got.Header().Get("Allow"), want.contentType, want.allow)
	}
	if want.status == http.StatusMethodNotAllowed {
		if got.Body.Len() != 0 {
			t.Fatalf("405 body = %q, want empty", got.Body.String())
		}
		return nil
	}
	var envelope map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("response is not JSON: %v; body=%s", err, got.Body.String())
	}
	if envelope["code"] != float64(want.code) {
		t.Fatalf("code = %v, want %d; body=%s", envelope["code"], want.code, got.Body.String())
	}
	message, _ := envelope["message"].(string)
	if !strings.Contains(message, want.message) {
		t.Fatalf("message = %q, want substring %q", message, want.message)
	}
	if want.status != http.StatusOK {
		if _, ok := envelope["data"]; ok {
			t.Fatalf("error response unexpectedly contains data: %s", got.Body.String())
		}
		return nil
	}
	data, ok := envelope["data"]
	if !ok {
		t.Fatalf("success response has no data: %s", got.Body.String())
	}
	return data
}
