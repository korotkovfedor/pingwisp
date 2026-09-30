package handlers

import (
	"cmp"
	"encoding/json"
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
	w.Header().Set("Content-Type", "application/json")

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

	jsonData, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(jsonData)
}

type getTargetsResponse struct {
	Targets []targetResponse `json:"targets"`
}
