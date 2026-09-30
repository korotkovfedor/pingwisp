package handlers

import (
	"net/http"
	"strconv"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type TargetGetter interface {
	GetTarget(id models.TargetID) (models.TargetState, bool)
}

type GetTargetHandler struct {
	targetGetter TargetGetter
}

func NewGetTarget(targetGetter TargetGetter) *GetTargetHandler {
	return &GetTargetHandler{targetGetter: targetGetter}
}

func (h *GetTargetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    codeInvalidID,
			Message: "id must be a decimal unsigned 64-bit integer",
			Field:   "id",
		})
		return
	}

	state, ok := h.targetGetter.GetTarget(models.TargetID(id))
	if !ok {
		writeError(w, http.StatusNotFound, apiError{
			Code:    codeTargetNotFound,
			Message: "target not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, newTargetResponse(state))
}
