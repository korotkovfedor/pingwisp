package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

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
	w.Header().Add("Content-Type", "application/json")

	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	target, ok := h.targetGetter.GetTarget(models.TargetID(id))
	if !ok {
		http.Error(w, "target not found", http.StatusNotFound)
		return
	}

	response := getTargetResponse{
		ID:              target.Settings.ID,
		URL:             target.Settings.URL,
		IntervalSeconds: int(target.Settings.Interval / time.Second),
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(jsonData)
}

type getTargetResponse struct {
	ID              models.TargetID `json:"id"`
	URL             string          `json:"url"`
	IntervalSeconds int             `json:"interval_seconds"`
}
