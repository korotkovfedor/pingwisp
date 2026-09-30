package handlers

import (
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type targetResponse struct {
	ID              models.TargetID `json:"id"`
	URL             string          `json:"url"`
	IntervalSeconds int             `json:"interval_seconds"`
	Status          string          `json:"status"`
	LastCheckedAt   *time.Time      `json:"last_checked_at"`
	StatusCode      *int            `json:"status_code"`
	LatencyMS       *int64          `json:"latency_ms"`
	Error           *string         `json:"error"`
	NextCheckAt     *time.Time      `json:"next_check_at"`
}

func newTargetResponse(state models.TargetState) targetResponse {
	response := targetResponse{
		ID:              state.Settings.ID,
		URL:             state.Settings.URL,
		IntervalSeconds: int(state.Settings.Interval / time.Second),
		Status:          "pending",
	}

	if state.NextCheckAt != nil {
		nextCheckAt := state.NextCheckAt.UTC()
		response.NextCheckAt = &nextCheckAt
	}

	if state.LastCheck == nil {
		return response
	}

	check := state.LastCheck
	lastCheckedAt := check.CompletedAt.UTC()
	latencyMS := check.Latency.Milliseconds()
	response.Status = check.Status
	response.LastCheckedAt = &lastCheckedAt
	response.LatencyMS = &latencyMS
	if check.StatusCode != nil {
		statusCode := *check.StatusCode
		response.StatusCode = &statusCode
	}
	if check.Error != nil {
		message := check.Error.Error()
		response.Error = &message
	}

	return response
}
