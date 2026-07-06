// Package console implements the web console HTTP API.
package console

import (
	"encoding/json"
	"net/http"
)

type envelope struct {
	Success bool    `json:"success"`
	Data    any     `json:"data"`
	Error   *string `json:"error"`
}

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, envelope{Success: true, Data: data})
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, envelope{Success: false, Error: &msg})
}

// writeErrData carries structured data alongside an error,
// e.g. the referencedBy list on a blocked delete.
func writeErrData(w http.ResponseWriter, status int, msg string, data any) {
	writeJSON(w, status, envelope{Success: false, Data: data, Error: &msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
