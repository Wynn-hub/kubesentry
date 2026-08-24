// Package console implements the web console HTTP API.
package console

import (
	"encoding/json"
	"net/http"
)

// Envelope is the uniform response wrapper for every /api/v1 route. Its shape
// is defined in api/schemas/common.yaml — the two must stay in lockstep.
//
// code is HTTP status × 100 (0 for success) rather than a hand-maintained
// registry of business codes: the status already carries the semantics, so a
// derived code adds a machine-readable duplicate without a second vocabulary
// to keep in sync. Future finer-grained codes increment within a band
// (40001, 40002) without touching this mapping.
type Envelope struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    any      `json:"data,omitempty"`
	Details []string `json:"details,omitempty"`
}

// codeForStatus derives the envelope code from an HTTP status. 200 maps to 0
// (success); everything else is status×100 so an unmapped status can never be
// mistaken for success.
func codeForStatus(status int) int {
	if status == http.StatusOK {
		return 0
	}
	return status * 100
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, Envelope{Code: codeForStatus(status), Message: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
