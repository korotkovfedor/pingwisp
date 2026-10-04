package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

const (
	maxRequestBodyBytes = 16 * 1024
	minIntervalSeconds  = 1
	maxIntervalSeconds  = 86400
)

type createTargetHandler struct {
	targetCreator targetCreator
}

type targetCreator interface {
	CreateTarget(url string, interval time.Duration) models.TargetState
}

func newCreateTarget(targetCreator targetCreator) *createTargetHandler {
	return &createTargetHandler{
		targetCreator: targetCreator,
	}
}

func (h *createTargetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			writeError(w, http.StatusRequestEntityTooLarge, apiError{
				Code:    codeRequestTooLarge,
				Message: "request body must not exceed 16 KiB",
			})
		} else {
			writeError(w, http.StatusBadRequest, apiError{
				Code:    codeInvalidJSON,
				Message: "request body could not be read",
			})
		}
		return
	}

	var request createTargetRequest
	err = json.NewDecoder(bytes.NewReader(body)).Decode(&request)
	if err != nil {
		if typeError, ok := errors.AsType[*json.UnmarshalTypeError](err); ok && typeError.Field != "" {
			writeError(w, http.StatusBadRequest, apiError{
				Code:    codeInvalidRequest,
				Message: "invalid value for " + typeError.Field,
				Field:   typeError.Field,
			})
		} else {
			writeError(w, http.StatusBadRequest, apiError{
				Code:    codeInvalidJSON,
				Message: "malformed JSON body",
			})
		}
		return
	}

	if err := request.validate(); err != nil {
		writeError(w, http.StatusBadRequest, *err)
		return
	}

	state := h.targetCreator.CreateTarget(
		request.URL,
		time.Second*time.Duration(request.IntervalSeconds),
	)

	w.Header().Set("Location", "/targets/"+strconv.FormatUint(uint64(state.Settings.ID), 10))
	writeJSON(w, http.StatusCreated, newTargetResponse(state))
}

type createTargetRequest struct {
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
}

func (r *createTargetRequest) validate() *apiError {
	if r.URL == "" {
		return &apiError{
			Code:    codeInvalidRequest,
			Message: "url is required",
			Field:   "url",
		}
	}

	if r.IntervalSeconds < minIntervalSeconds || r.IntervalSeconds > maxIntervalSeconds {
		return &apiError{
			Code:    codeInvalidRequest,
			Message: fmt.Sprintf("interval_seconds must be between %d and %d", minIntervalSeconds, maxIntervalSeconds),
			Field:   "interval_seconds",
		}
	}

	return nil
}
