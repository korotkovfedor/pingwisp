package handlers

import "net/http"

const (
	codeInvalidJSON      = "invalid_json"
	codeInvalidRequest   = "invalid_request"
	codeInvalidID        = "invalid_id"
	codeTargetNotFound   = "target_not_found"
	codeRouteNotFound    = "route_not_found"
	codeMethodNotAllowed = "method_not_allowed"
	codeRequestTooLarge  = "request_too_large"
	codeInternalError    = "internal_error"
)

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func writeError(w http.ResponseWriter, status int, response apiError) {
	writeJSON(w, status, errorResponse{Error: response})
}
