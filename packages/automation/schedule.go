package automation

import (
	"fmt"
	"strings"
	"time"
)

// Schedule describes a persisted, interval-based automation trigger. The
// first self-contained scheduler intentionally uses intervals instead of cron
// expressions so the local tier has deterministic behaviour without another
// parser or service dependency.
type Schedule struct {
	ID              string
	OrganisationID  string
	CreatedByUserID string
	Name            string
	RunbookName     string
	Target          string
	Reason          string
	Params          map[string]string
	IntervalSeconds int
	NextRunAt       time.Time
	Enabled         bool
}

func (s Schedule) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("schedule name is required")
	}
	if strings.TrimSpace(s.RunbookName) == "" {
		return fmt.Errorf("runbook name is required")
	}
	if strings.TrimSpace(s.Target) == "" {
		return fmt.Errorf("target is required")
	}
	if s.IntervalSeconds < 60 {
		return fmt.Errorf("interval must be at least 60 seconds")
	}
	if s.NextRunAt.IsZero() {
		return fmt.Errorf("next run time is required")
	}
	return nil
}

func (s Schedule) Due(now time.Time) bool {
	return s.Enabled && !s.NextRunAt.IsZero() && !s.NextRunAt.After(now)
}

func (s Schedule) NextAfter(now time.Time) time.Time {
	if s.IntervalSeconds < 1 {
		return now
	}
	next := s.NextRunAt
	if next.IsZero() {
		return now.Add(time.Duration(s.IntervalSeconds) * time.Second)
	}
	for !next.After(now) {
		next = next.Add(time.Duration(s.IntervalSeconds) * time.Second)
	}
	return next
}
