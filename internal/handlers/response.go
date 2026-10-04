package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, response any) {
	data, err := json.Marshal(response)
	if err != nil {
		slog.Error("encode HTTP response failed", "error", err)
		w.Header().Del("Location")
		writeError(w, http.StatusInternalServerError, apiError{
			Code:    codeInternalError,
			Message: "internal server error",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		slog.Error("write HTTP response failed", "error", err)
	}
}
