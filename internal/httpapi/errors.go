package httpapi

import (
	"encoding/json"
	"net/http"
)

// Every non-2xx response is this envelope, so clients parse one shape. The
// request ID is on the X-Request-ID response header, set by the observability
// middleware, not repeated in the body.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`    // machine-readable, stable: "unknown_tenant"
	Message string `json:"message"` // human-readable, may change
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: msg}})
}

// writeJSON sets the content type, the status, and encodes v. The status must
// be written before the body — net/http sends 200 implicitly on the first
// Write otherwise.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // nothing useful to do if the client has gone
}
