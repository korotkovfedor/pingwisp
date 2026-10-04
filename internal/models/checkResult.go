package models

import "time"

const (
	StatusUp   = "up"
	StatusDown = "down"
)

type CheckResult struct {
	Status      string
	StatusCode  *int
	CompletedAt time.Time
	Latency     time.Duration
	Error       error
}
