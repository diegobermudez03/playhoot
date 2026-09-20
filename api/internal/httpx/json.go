// Package httpx holds small HTTP response mechanics shared across every
// workflow group's handlers in api - JSON encoding, never business
// behavior or routing decisions.
package httpx

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the wire shape every HTTP handler in api uses to report
// a failure.
type ErrorResponse struct {
	Message string `json:"message"`
}

// WriteJSON writes body as a JSON response with the given HTTP status.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// WriteError writes message as an ErrorResponse with the given HTTP status.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{Message: message})
}
