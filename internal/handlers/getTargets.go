package handlers

import (
	"cmp"
	"net/http"
	"slices"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type targetLister interface {
	GetTargets() []models.TargetState
}

type getTargetsHandler struct {
	targetLister targetLister
}

func newGetTargets(targetLister targetLister) *getTargetsHandler {
	return &getTargetsHandler{targetLister: targetLister}
}

func (h *getTargetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
