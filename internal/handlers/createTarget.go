package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type CreateTargetHandler struct {
	targetCreator TargetCreator
}

type TargetCreator interface {
	CreateTarget(url string, interval time.Duration) models.Target
}

func NewCreateTarget(targetCreator TargetCreator) *CreateTargetHandler {
	return &CreateTargetHandler{
		targetCreator: targetCreator,
	}
}

func (h *CreateTargetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")

	var CreateTarget CreateTargetRequest
	err := json.NewDecoder(r.Body).Decode(&CreateTarget)
	if err != nil {
		http.Error(w, "Malformed JSON body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := CreateTarget.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	target := h.targetCreator.CreateTarget(
		CreateTarget.URL,
		time.Second*time.Duration(CreateTarget.IntervalSeconds),
	)

	jsonData, err := json.Marshal(CreatedTargetResponse{
		ID:              target.ID,
		URL:             target.URL,
		IntervalSeconds: int(target.Interval / time.Second),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write(jsonData)
}

type CreateTargetRequest struct {
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
}

func (r *CreateTargetRequest) validate() error {
	if r.URL == "" {
		return errors.New("url is required")
	}

	if r.IntervalSeconds <= 0 {
		return errors.New("interval_seconds must be greater than 0")
	}

	return nil
}

type CreatedTargetResponse struct {
	ID              models.TargetID `json:"id"`
	URL             string          `json:"url"`
	IntervalSeconds int             `json:"interval_seconds"`
}
