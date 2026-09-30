package models

import "time"

type TargetState struct {
	Settings    Target
	LastCheck   *CheckResult
	NextCheckAt *time.Time
}
