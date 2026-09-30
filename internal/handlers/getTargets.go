package handlers

import (
	"cmp"
	"net/http"
	"slices"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type TargetLister interface {
	GetTargets() []models.TargetState
}

type GetTargetsHandler struct {
	targetLister TargetLister
}

func NewGetTargets(targetLister TargetLister) *GetTargetsHandler {
	return &GetTargetsHandler{targetLister: targetLister}
}

func (h *GetTargetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targets := h.targetLister.GetTargets()
	slices.SortFunc(targets, func(a, b models.TargetState) int {
		return cmp.Compare(a.Settings.ID, b.Settings.ID)
	})

	response := getTargetsResponse{
		Targets: make([]targetResponse, 0, len(targets)),
	}
	for _, state := range targets {
		response.Targets = append(response.Targets, newTargetResponse(state))
	}

	writeJSON(w, http.StatusOK, response)
}

type getTargetsResponse struct {
	Targets []targetResponse `json:"targets"`
}
