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
	GetTargets() []models.Target
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
	slices.SortFunc(targets, func(a, b models.Target) int {
		return cmp.Compare(a.ID, b.ID)
	})

	for _, target := range targets {
		item := targetItem{
			ID:              target.ID,
			URL:             target.URL,
			IntervalSeconds: int(target.Interval / time.Second),
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
