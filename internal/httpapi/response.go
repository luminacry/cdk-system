package httpapi

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the standard error JSON shape.
type ErrorResponse struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Errors  []string `json:"errors,omitempty"`
}

// RespondJSON writes a JSON response with the given status code.
func RespondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// RespondError writes a standard error JSON response.
func RespondError(w http.ResponseWriter, status int, message string, errors ...string) {
	RespondJSON(w, status, ErrorResponse{OK: false, Message: message, Errors: errors})
}

// OK writes a simple {ok: true} response.
func OK(w http.ResponseWriter) {
	RespondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
