package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pgd1001/svrtools/packages/authz"
	"github.com/pgd1001/svrtools/packages/automation"
	"github.com/pgd1001/svrtools/packages/redact"
	"github.com/pgd1001/svrtools/packages/runbooks"
)

const automationActorID = "user_automation"

type automationControl struct {
	Paused   bool   `json:"paused"`
	PausedAt string `json:"paused_at,omitempty"`
	PausedBy string `json:"paused_by,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func readAutomationControl(ctx context.Context, db *sql.DB, organisationID string) (automationControl, error) {
	var control automationControl
	var paused int
	err := apiQueryRow(ctx, db, `SELECT paused, COALESCE(paused_at,''), COALESCE(paused_by_user_id,''), COALESCE(reason,'') FROM automation_controls WHERE organisation_id = ?`, organisationID).Scan(&paused, &control.PausedAt, &control.PausedBy, &control.Reason)
	if err == sql.ErrNoRows {
		return control, nil
	}
	if err != nil {
		return control, err
	}
	control.Paused = paused == 1
	return control, nil
}

func automationPaused(ctx context.Context, db *sql.DB, organisationID string) (bool, error) {
	control, err := readAutomationControl(ctx, db, organisationID)
	return control.Paused, err
}

func handleAutomationStatus(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	control, err := readAutomationControl(r.Context(), dbFrom(r), actor.OrganisationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read automation status"})
		return
	}
	writeJSON(w, http.StatusOK, control)
}

func handlePauseAutomation(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "automation.paused", "organisation", actor.OrganisationID, authz.Deny("automation_requires_senior", "Pausing automation requires senior engineer or above."))
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}
	db := dbFrom(r)
	_, err := apiExec(r.Context(), db, `INSERT INTO automation_controls (organisation_id, paused, paused_at, paused_by_user_id, reason, updated_at) VALUES (?,1,`+apiCurrentTime()+`,?,?,`+apiCurrentTime()+`) ON CONFLICT(organisation_id) DO UPDATE SET paused=1, paused_at=`+apiCurrentTime()+`, paused_by_user_id=excluded.paused_by_user_id, reason=excluded.reason, updated_at=`+apiCurrentTime(), actor.OrganisationID, actor.UserID, strings.TrimSpace(req.Reason))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to pause automation"})
		return
	}
	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "automation.paused", "organisation", actor.OrganisationID, "success", map[string]any{"reason": strings.TrimSpace(req.Reason)})
	writeJSON(w, http.StatusOK, map[string]any{"paused": true, "reason": strings.TrimSpace(req.Reason)})
}

func handleResumeAutomation(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "automation.resumed", "organisation", actor.OrganisationID, authz.Deny("automation_requires_senior", "Resuming automation requires senior engineer or above."))
		return
	}
	db := dbFrom(r)
	_, err := apiExec(r.Context(), db, `INSERT INTO automation_controls (organisation_id, paused, reason, updated_at) VALUES (?,0,'',`+apiCurrentTime()+`) ON CONFLICT(organisation_id) DO UPDATE SET paused=0, reason='', updated_at=`+apiCurrentTime(), actor.OrganisationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resume automation"})
		return
	}
	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "automation.resumed", "organisation", actor.OrganisationID, "success", nil)
	writeJSON(w, http.StatusOK, map[string]any{"paused": false})
}

func handleListSchedules(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	rows, err := apiQuery(r.Context(), dbFrom(r), `SELECT id, name, runbook_name, target, reason, params, interval_seconds, next_run_at, enabled, COALESCE(last_run_at,''), COALESCE(last_error,'') FROM automation_schedules WHERE organisation_id = ? ORDER BY name`, actor.OrganisationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list schedules"})
		return
	}
	defer rows.Close()
	type item struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		RunbookName     string `json:"runbook_name"`
		Target          string `json:"target"`
		Reason          string `json:"reason"`
		Params          string `json:"params"`
		IntervalSeconds int    `json:"interval_seconds"`
		NextRunAt       string `json:"next_run_at"`
		Enabled         bool   `json:"enabled"`
		LastRunAt       string `json:"last_run_at"`
		LastError       string `json:"last_error"`
	}
	items := []item{}
	for rows.Next() {
		var value item
		var enabled int
		if err := rows.Scan(&value.ID, &value.Name, &value.RunbookName, &value.Target, &value.Reason, &value.Params, &value.IntervalSeconds, &value.NextRunAt, &enabled, &value.LastRunAt, &value.LastError); err == nil {
			value.Enabled = enabled == 1
			items = append(items, value)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": items})
}

func handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "automation.schedule.created", "automation_schedule", "", authz.Deny("schedule_requires_senior", "Creating schedules requires senior engineer or above."))
		return
	}
	var req struct {
		Name            string            `json:"name"`
		RunbookName     string            `json:"runbook_name"`
		Target          string            `json:"target"`
		Reason          string            `json:"reason"`
		Params          map[string]string `json:"params"`
		IntervalSeconds int               `json:"interval_seconds"`
		NextRunAt       string            `json:"next_run_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	nextRunAt := time.Now().UTC().Add(time.Duration(req.IntervalSeconds) * time.Second)
	if strings.TrimSpace(req.NextRunAt) != "" {
		parsed, err := time.Parse(time.RFC3339, req.NextRunAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "next_run_at must be RFC3339"})
			return
		}
		nextRunAt = parsed.UTC()
	}
	schedule := automation.Schedule{Name: req.Name, OrganisationID: actor.OrganisationID, CreatedByUserID: actor.UserID, RunbookName: req.RunbookName, Target: req.Target, Reason: req.Reason, Params: req.Params, IntervalSeconds: req.IntervalSeconds, NextRunAt: nextRunAt, Enabled: true}
	if err := schedule.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	db := dbFrom(r)
	_, err := apiExec(r.Context(), db, `INSERT INTO automation_schedules (id, organisation_id, created_by_user_id, name, runbook_name, target, reason, params, interval_seconds, next_run_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, "sch_"+shortID(), actor.OrganisationID, actor.UserID, schedule.Name, schedule.RunbookName, schedule.Target, schedule.Reason, jsonString(schedule.Params), schedule.IntervalSeconds, formatScheduleTime(schedule.NextRunAt))
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "schedule name already exists or could not be created"})
		return
	}
	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "automation.schedule.created", "automation_schedule", schedule.Name, "success", map[string]any{"runbook": schedule.RunbookName, "target": schedule.Target, "interval_seconds": schedule.IntervalSeconds})
	writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "name": schedule.Name, "next_run_at": formatScheduleTime(schedule.NextRunAt)})
}

func handleDisableSchedule(w http.ResponseWriter, r *http.Request, scheduleID string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "automation.schedule.disabled", "automation_schedule", scheduleID, authz.Deny("schedule_requires_senior", "Changing schedules requires senior engineer or above."))
		return
	}
	result, err := apiExec(r.Context(), dbFrom(r), "UPDATE automation_schedules SET enabled = 0, updated_at = "+apiCurrentTime()+" WHERE id = ? AND organisation_id = ?", scheduleID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to disable schedule"})
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "automation.schedule.disabled", "automation_schedule", scheduleID, "success", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

func runEmbeddedScheduler(ctx context.Context, db *sql.DB, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := runDueSchedulesOnce(ctx, db); err != nil {
				logger.Error("embedded scheduler cycle failed", "error", err)
			}
		}
	}
}

func runDueSchedulesOnce(ctx context.Context, db *sql.DB) error {
	orgRows, err := apiQuery(ctx, db, `SELECT DISTINCT organisation_id FROM automation_schedules WHERE enabled = 1 AND next_run_at <= `+apiCurrentTime())
	if err != nil {
		return err
	}
	organisationIDs := []string{}
	for orgRows.Next() {
		var organisationID string
		if err := orgRows.Scan(&organisationID); err != nil {
			_ = orgRows.Close()
			return err
		}
		organisationIDs = append(organisationIDs, organisationID)
	}
	if err := orgRows.Close(); err != nil {
		return err
	}
	pausedOrgs := map[string]bool{}
	for _, organisationID := range organisationIDs {
		paused, err := automationPaused(ctx, db, organisationID)
		if err != nil {
			return err
		}
		pausedOrgs[organisationID] = paused
	}
	if len(pausedOrgs) == 0 {
		return nil
	}
	rows, err := apiQuery(ctx, db, `SELECT id, organisation_id, created_by_user_id, name, runbook_name, target, reason, params, interval_seconds, next_run_at FROM automation_schedules WHERE enabled = 1 AND next_run_at <= `+apiCurrentTime()+` ORDER BY next_run_at LIMIT 20`)
	if err != nil {
		return err
	}
	due := make([]automation.Schedule, 0, 20)
	for rows.Next() {
		var schedule automation.Schedule
		var paramsJSON, nextRunAt string
		if err := rows.Scan(&schedule.ID, &schedule.OrganisationID, &schedule.CreatedByUserID, &schedule.Name, &schedule.RunbookName, &schedule.Target, &schedule.Reason, &paramsJSON, &schedule.IntervalSeconds, &nextRunAt); err != nil {
			_ = rows.Close()
			return err
		}
		_ = json.Unmarshal([]byte(paramsJSON), &schedule.Params)
		schedule.Enabled = true
		schedule.NextRunAt, _ = parseScheduleTime(nextRunAt)
		due = append(due, schedule)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, schedule := range due {
		if pausedOrgs[schedule.OrganisationID] {
			continue
		}
		paused, err := automationPaused(ctx, db, schedule.OrganisationID)
		if err != nil {
			return err
		}
		if paused {
			continue
		}
		next := schedule.NextAfter(time.Now().UTC())
		claimed, err := apiExec(ctx, db, `UPDATE automation_schedules SET next_run_at = ?, last_run_at = `+apiCurrentTime()+`, last_error = NULL, updated_at = `+apiCurrentTime()+` WHERE id = ? AND enabled = 1 AND next_run_at <= `+apiCurrentTime(), formatScheduleTime(next), schedule.ID)
		if err != nil {
			return err
		}
		count, _ := claimed.RowsAffected()
		if count == 0 {
			continue
		}
		if err := executeScheduledRun(ctx, db, schedule); err != nil {
			_, updateErr := apiExec(ctx, db, "UPDATE automation_schedules SET last_error = ?, updated_at = "+apiCurrentTime()+" WHERE id = ?", err.Error(), schedule.ID)
			if updateErr != nil {
				return updateErr
			}
		}
	}
	return nil
}

func executeScheduledRun(ctx context.Context, db *sql.DB, schedule automation.Schedule) error {
	var runbookID, risk, definition string
	if err := apiQueryRow(ctx, db, `SELECT r.id, rv.risk_level, rv.definition_json FROM runbooks r JOIN runbook_versions rv ON rv.id = r.current_version_id WHERE r.organisation_id = ? AND r.name = ? AND r.status = 'published'`, schedule.OrganisationID, schedule.RunbookName).Scan(&runbookID, &risk, &definition); err != nil {
		return fmt.Errorf("runbook is not published")
	}
	if risk == string(runbooks.RiskHigh) || risk == string(runbooks.RiskCritical) {
		return fmt.Errorf("high-risk schedules require an approval workflow and are not auto-executed")
	}
	var rb runbooks.Runbook
	if err := json.Unmarshal([]byte(definition), &rb); err != nil {
		return fmt.Errorf("invalid stored runbook definition: %w", err)
	}
	targetIDs := resolveTargets(ctx, db, schedule.OrganisationID, schedule.Target)
	if len(targetIDs) == 0 {
		return fmt.Errorf("no servers found for target %q", schedule.Target)
	}
	env, mixed := targetEnvironment(ctx, db, schedule.OrganisationID, targetIDs)
	if mixed || !rb.EnvironmentAllowed(env) || !runbookTargetsAllowed(ctx, db, schedule.OrganisationID, rb, targetIDs) {
		return fmt.Errorf("schedule target is not permitted")
	}
	if env == "production" && strings.TrimSpace(schedule.Reason) == "" {
		return fmt.Errorf("production schedules require a reason")
	}
	command, err := rb.RenderCommand(schedule.Params)
	if err != nil {
		return err
	}
	snapshots := snapshotTargets(ctx, db, schedule.OrganisationID, targetIDs)
	if len(snapshots) != len(targetIDs) {
		return fmt.Errorf("failed to snapshot schedule targets")
	}
	timeout := rb.Spec.Execution.TimeoutSeconds
	if timeout <= 0 {
		timeout = 300
	}
	execID := "exe_" + shortID()
	tx, err := beginAPITx(ctx, db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = apiExec(ctx, tx, `INSERT INTO executions (id, organisation_id, actor_user_id, actor_role_at_time, delegated_by_user_id, execution_type, status, environment, risk_level, reason, command, command_preview, command_hash, timeout_seconds) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, execID, schedule.OrganisationID, automationActorID, "automation", schedule.CreatedByUserID, "runbook", "queued", env, risk, schedule.Reason, command, redact.Stdout(command), hashCmd(command), timeout); err != nil {
		return err
	}
	for i, serverID := range targetIDs {
		if _, err = apiExec(ctx, tx, `INSERT INTO execution_targets (id, organisation_id, execution_id, server_id, status, server_snapshot) VALUES (?,?,?,?,'pending',`+metadataRuntime().JSONParameter()+`)`, "ext_"+shortID(), schedule.OrganisationID, execID, serverID, jsonString(snapshots[i])); err != nil {
			return err
		}
	}
	if err = recordExecutionEvent(ctx, tx, schedule.OrganisationID, execID, "", "", "queued", "automation.queued", map[string]any{"schedule_id": schedule.ID, "schedule_name": schedule.Name}); err != nil {
		return err
	}
	if err = writeAuditEventTypeTx(ctx, tx, schedule.OrganisationID, automationActorID, "automation", "automation.execution.queued", "execution", execID, "queued", map[string]any{"schedule_id": schedule.ID, "runbook_id": runbookID, "target_count": len(targetIDs)}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := publishPendingJobNotifications(ctx, db, execID); err != nil {
		return fmt.Errorf("execution committed but JetStream notification publication was incomplete: %w", err)
	}
	return nil
}

func formatScheduleTime(value time.Time) string { return value.UTC().Format("2006-01-02 15:04:05") }

func parseScheduleTime(value string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05", value, time.UTC)
}
