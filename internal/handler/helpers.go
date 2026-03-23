package handler

import (
	"encoding/json"
	"net/http"
)

// APIResponse is the standard envelope for all API responses.
type APIResponse struct {
	Data  any        `json:"data,omitempty"`
	Error *APIError  `json:"error,omitempty"`
}

// APIError carries a machine-readable code and a human-readable message.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JSON writes a JSON-encoded response with the given status code.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIResponse{Data: data})
}

// Error writes a JSON-encoded error response with the given status code.
func Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIResponse{Error: &APIError{Code: code, Message: message}})
}

// DecodeJSON decodes the JSON body of r into v.
func DecodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
