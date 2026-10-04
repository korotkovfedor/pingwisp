package handlers

import (
	"net/http"
	"strconv"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type targetDeleter interface {
	DeleteTarget(id models.TargetID) bool
}

type deleteTargetHandler struct {
	targetDeleter targetDeleter
}

func newDeleteTarget(targetDeleter targetDeleter) *deleteTargetHandler {
	return &deleteTargetHandler{targetDeleter: targetDeleter}
}

func (h *deleteTargetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    codeInvalidID,
			Message: "id must be a decimal unsigned 64-bit integer",
			Field:   "id",
		})
		return
	}

	if !h.targetDeleter.DeleteTarget(models.TargetID(id)) {
		writeError(w, http.StatusNotFound, apiError{
			Code:    codeTargetNotFound,
			Message: "target not found",
		})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
