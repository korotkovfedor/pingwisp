package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type CreateTargetHandler struct {
	targetCreator TargetCreator
}

type TargetCreator interface {
	CreateTarget(url string, interval time.Duration) models.TargetState
}

func NewCreateTarget(targetCreator TargetCreator) *CreateTargetHandler {
	return &CreateTargetHandler{
		targetCreator: targetCreator,
	}
}

func (h *CreateTargetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var request CreateTargetRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "Malformed JSON body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := request.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state := h.targetCreator.CreateTarget(
		request.URL,
		time.Second*time.Duration(request.IntervalSeconds),
	)

	jsonData, err := json.Marshal(newTargetResponse(state))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", "/targets/"+strconv.FormatUint(uint64(state.Settings.ID), 10))
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
