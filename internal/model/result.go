// Package model contains the stable data contract returned by UnlockScope.
package model

import "time"

// State is the outcome of a provider probe.
type State string

const (
	Available   State = "available"
	Unavailable State = "unavailable"
	RegionOnly  State = "region_only"
	Failed      State = "failed"
	Unknown     State = "unknown"
)

// Result is intentionally small and versionable. Consumers should key on State
// and not on human-readable Note text.
type Result struct {
	ID         string    `json:"id"`
	Service    string    `json:"service"`
	Category   string    `json:"category"`
	Regions    []string  `json:"regions"`
	State      State     `json:"state"`
	Region     string    `json:"region,omitempty"`
	Note       string    `json:"note,omitempty"`
	DurationMS int64     `json:"duration_ms"`
	CheckedAt  time.Time `json:"checked_at"`
}
