package handlers

import (
	"cmp"
	"encoding/json"
	"net/http"
	"slices"
	"time"

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
	w.Header().Add("Content-Type", "application/json")

	response := getTargetsResponse{
		Targets: []targetItem{},
	}

	targets := h.targetLister.GetTargets()
	slices.SortFunc(targets, func(a, b models.TargetState) int {
		return cmp.Compare(a.Settings.ID, b.Settings.ID)
	})

	for _, target := range targets {
		item := targetItem{
			ID:              target.Settings.ID,
			URL:             target.Settings.URL,
			IntervalSeconds: int(target.Settings.Interval / time.Second),
		}
		response.Targets = append(response.Targets, item)
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(jsonData)
}

type getTargetsResponse struct {
	Targets []targetItem `json:"targets"`
}

type targetItem struct {
	ID              models.TargetID `json:"id"`
	URL             string          `json:"url"`
	IntervalSeconds int             `json:"interval_seconds"`
}
