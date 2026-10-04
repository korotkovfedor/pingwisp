package models

import "time"

type TargetID uint64

type Target struct {
	ID       TargetID
	URL      string
	Interval time.Duration
}
