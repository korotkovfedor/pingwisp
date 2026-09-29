package handlers

import (
	"net/http"
	"strconv"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type TargetDeleter interface {
	DeleteTarget(id models.TargetID) error
}

type DeleteTargetHandler struct {
	targetDeleter TargetDeleter
}

func NewDeleteTarget(targetDeleter TargetDeleter) *DeleteTargetHandler {
	return &DeleteTargetHandler{targetDeleter: targetDeleter}
}

func (h *DeleteTargetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")

	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = h.targetDeleter.DeleteTarget(models.TargetID(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
