package main

import (
	"sync"
	"time"
)

// Lease reconciliation walks every running target in an organisation to apply
// retry backoff and dead-lettering. Running it on every job claim made each
// runner poll a full scan, so N runners polling every two seconds produced N/2
// scans per second against the single SQLite writer.
//
// Reclaiming expired work does not depend on it: the claim query already
// selects targets whose lease has lapsed. Reconciliation only needs to run
// often enough to move exhausted work into dead_letter and to apply backoff,
// so it is throttled per organisation.
const leaseReconcileInterval = 15 * time.Second

// reconcileThrottle records the last reconciliation per organisation.
type reconcileThrottle struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func newReconcileThrottle() *reconcileThrottle {
	return &reconcileThrottle{last: map[string]time.Time{}}
}

// due reports whether orgID should reconcile now, and records the attempt when
// it returns true. Concurrent callers for the same organisation see exactly one
// true per interval.
func (t *reconcileThrottle) due(orgID string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if previous, seen := t.last[orgID]; seen && now.Sub(previous) < leaseReconcileInterval {
		return false
	}
	t.last[orgID] = now
	return true
}

// forget drops an organisation's entry. Used by tests to force reconciliation.
func (t *reconcileThrottle) forget(orgID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.last, orgID)
}

var claimReconcileThrottle = newReconcileThrottle()
