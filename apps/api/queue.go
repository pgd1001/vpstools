package main

import (
	"context"
	"fmt"
)

// reconcileExpiredLeases moves abandoned work back to the pending queue while
// there are attempts left. Work that has exhausted its budget is dead-lettered
// so it cannot leave an execution in running forever.
func reconcileExpiredLeases(ctx context.Context, exec auditExec, orgID string) error {
	rows, err := exec.QueryContext(ctx, `
		SELECT et.id, et.execution_id, et.attempt, et.max_attempts
		FROM execution_targets et
		JOIN executions e ON e.id = et.execution_id
		WHERE et.organisation_id = ? AND e.organisation_id = ?
		  AND e.status IN ('queued','running')
		  AND et.status = 'running'
		  AND et.lease_expires_at IS NOT NULL
		  AND et.lease_expires_at <= datetime('now')`, orgID, orgID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type expiredTarget struct {
		id, executionID string
		attempt, max    int
	}
	var expired []expiredTarget
	for rows.Next() {
		var target expiredTarget
		if err := rows.Scan(&target.id, &target.executionID, &target.attempt, &target.max); err != nil {
			return err
		}
		expired = append(expired, target)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, target := range expired {
		if target.attempt >= target.max {
			if _, err := exec.ExecContext(ctx, `UPDATE execution_targets
				SET status = 'dead_letter', runner_id = NULL, lease_id = NULL,
				    lease_expires_at = NULL, finished_at = datetime('now'),
				    error_summary = 'runner lease expired after maximum attempts'
				WHERE id = ? AND status = 'running'`, target.id); err != nil {
				return err
			}
			if err := recordExecutionEvent(ctx, exec, orgID, target.executionID, target.id, "running", "dead_letter", "execution.target.dead_lettered", map[string]any{
				"reason": "lease_expired", "attempt": target.attempt, "max_attempts": target.max,
			}); err != nil {
				return err
			}
			continue
		}

		backoff := retryBackoffSeconds(target.attempt)
		if _, err := exec.ExecContext(ctx, `UPDATE execution_targets
			SET status = 'pending', runner_id = NULL, lease_id = NULL,
			    lease_expires_at = NULL, next_attempt_at = datetime('now', ? || ' seconds')
			WHERE id = ? AND status = 'running'`, fmt.Sprintf("+%d", backoff), target.id); err != nil {
			return err
		}
		if err := recordExecutionEvent(ctx, exec, orgID, target.executionID, target.id, "running", "pending", "execution.target.lease_expired", map[string]any{
			"retry_at_seconds": backoff, "attempt": target.attempt, "max_attempts": target.max,
		}); err != nil {
			return err
		}
	}

	return finalizeReconciledExecutions(ctx, exec, orgID)
}

func retryBackoffSeconds(attempt int) int {
	// Keep the local queue predictable and bounded. The first retry waits one
	// second, then two, then four, capped at one minute.
	backoff := 1
	for i := 1; i < attempt && backoff < 60; i++ {
		backoff *= 2
	}
	if backoff > 60 {
		return 60
	}
	return backoff
}

func finalizeReconciledExecutions(ctx context.Context, exec auditExec, orgID string) error {
	_, err := exec.ExecContext(ctx, `UPDATE executions
		SET status = 'failed', finished_at = datetime('now'),
		    error_summary = 'one or more targets were dead-lettered'
		WHERE organisation_id = ? AND status IN ('queued','running')
		  AND NOT EXISTS (SELECT 1 FROM execution_targets et WHERE et.execution_id = executions.id AND et.status IN ('pending','running'))
		  AND EXISTS (SELECT 1 FROM execution_targets et WHERE et.execution_id = executions.id AND et.status IN ('failed','dead_letter'))`, orgID)
	return err
}
