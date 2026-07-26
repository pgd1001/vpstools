package automation

import (
	"testing"
	"time"
)

func TestScheduleValidationAndDue(t *testing.T) {
	now := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	schedule := Schedule{Name: "health-check", RunbookName: "check-uptime", Target: "server:srv_demo", IntervalSeconds: 300, NextRunAt: now.Add(-time.Minute), Enabled: true}
	if err := schedule.Validate(); err != nil {
		t.Fatalf("validate schedule: %v", err)
	}
	if !schedule.Due(now) {
		t.Fatal("expected schedule to be due")
	}
	if got := schedule.NextAfter(now); !got.Equal(now.Add(4 * time.Minute)) {
		t.Fatalf("unexpected next run: %s", got)
	}
}

func TestScheduleRejectsUnsafeInterval(t *testing.T) {
	schedule := Schedule{Name: "too-fast", RunbookName: "check-uptime", Target: "server:srv_demo", IntervalSeconds: 5, NextRunAt: time.Now().Add(time.Minute)}
	if err := schedule.Validate(); err == nil {
		t.Fatal("expected short interval to be rejected")
	}
}
