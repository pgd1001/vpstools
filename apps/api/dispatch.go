package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pgd1001/svrtools/packages/dispatch"
)

// publishPendingJobNotifications publishes only after the execution
// transaction has committed. The database remains the source of truth, and a
// notification contains no executable command or lease material.
func publishPendingJobNotifications(ctx context.Context, db *sql.DB, executionID string) error {
	if apiDispatcher == nil {
		return nil
	}
	rows, err := apiQuery(ctx, db, "SELECT id, execution_id, attempt FROM execution_targets WHERE execution_id = ? AND status = 'pending' ORDER BY id", executionID)
	if err != nil {
		return fmt.Errorf("list pending execution targets: %w", err)
	}
	defer rows.Close()
	var firstErr error
	for rows.Next() {
		var notification dispatch.Notification
		notification.Version = dispatch.NotificationVersion
		notification.Kind = dispatch.NotificationKind
		if err := rows.Scan(&notification.TargetID, &notification.ExecutionID, &notification.Attempt); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("scan pending execution target: %w", err)
			}
			continue
		}
		publishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := apiDispatcher.Publish(publishCtx, notification)
		cancel()
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("publish target %s notification: %w", notification.TargetID, err)
		}
	}
	if err := rows.Err(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("read pending execution targets: %w", err)
	}
	return firstErr
}
