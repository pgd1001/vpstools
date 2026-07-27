package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pgd1001/svrtools/packages/sdk-go/client"
)

func TestApprovalDetailViewShowsDecisionEvidence(t *testing.T) {
	model := tuiModel{
		screen: screenApprovalDetail,
		selectedApproval: &client.ApprovalDetail{
			ID:             "apr_test",
			RequesterName:  "Junior Engineer",
			ActionType:     "runbook",
			Status:         "pending",
			RiskLevel:      "high",
			Reason:         "planned maintenance",
			TargetType:     "server",
			TargetID:       "srv_demo",
			TargetSnapshot: `[{"id":"srv_demo","environment":"production"}]`,
			RequestPayload: map[string]any{
				"rollback":     "systemctl start nginx",
				"verification": "systemctl is-active nginx",
				"params":       map[string]any{"service": "nginx"},
			},
			ExpiresAt: "2026-07-27T13:00:00Z",
		},
	}
	view := model.View()
	for _, expected := range []string{"apr_test", "production", "rollback", "verification", "nginx"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("approval detail view does not contain %q: %s", expected, view)
		}
	}
}

func TestRunbookRunViewShowsGuidedInputs(t *testing.T) {
	model := tuiModel{
		screen: screenRunbookRun,
		selectedRunbook: &client.RunbookItem{
			Name:        "check-uptime",
			Risk:        "low",
			Description: "Check service health",
		},
	}
	model.runbookTarget.Placeholder = "target"
	model.runbookReason.Placeholder = "reason"
	model.runbookParams.Placeholder = "params"
	view := model.View()
	for _, expected := range []string{"Run task: check-uptime", "Target", "Reason", "Parameters", "preflight", "submit"} {
		if !strings.Contains(strings.ToLower(view), strings.ToLower(expected)) {
			t.Fatalf("guided task view does not contain %q: %s", expected, view)
		}
	}
}

func TestExecutionDetailViewShowsCancelForQueuedExecution(t *testing.T) {
	model := tuiModel{
		screen:       screenExecutionDetail,
		selectedExec: &client.GetExecutionResponse{Execution: client.ExecutionDetail{ID: "exec_queued", Status: "queued"}},
	}
	if view := model.View(); !strings.Contains(view, "[c] cancel") {
		t.Fatalf("queued execution detail should show cancellation help: %s", view)
	}

	model.selectedExec.Execution.Status = "succeeded"
	if view := model.View(); strings.Contains(view, "[c] cancel") {
		t.Fatalf("terminal execution detail should not show cancellation help: %s", view)
	}
}

func TestApprovalDenyViewShowsReasonInput(t *testing.T) {
	model := tuiModel{screen: screenApprovalDeny, pendingApprovalID: "apr_test"}
	model.approvalNote.Placeholder = "reason"
	view := model.View()
	for _, expected := range []string{"Deny approval", "apr_test", "Reason", "enter"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("denial view does not contain %q: %s", expected, view)
		}
	}
}

func TestScheduleViewShowsDisableControl(t *testing.T) {
	model := tuiModel{screen: screenSchedules}
	view := model.View()
	for _, expected := range []string{"Schedules", "new schedule", "disable selected schedule"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("schedule view does not contain %q: %s", expected, view)
		}
	}
}

func TestScheduleCreateViewShowsGuidedInputs(t *testing.T) {
	model := tuiModel{screen: screenScheduleCreate}
	view := model.View()
	for _, expected := range []string{"Create schedule", "Name", "Published runbook", "Interval", "create"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("schedule creation view does not contain %q: %s", expected, view)
		}
	}
}

func TestScheduleCreatePostsAndRefreshes(t *testing.T) {
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/schedules":
			var req client.CreateScheduleRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode schedule request: %v", err)
			}
			if req.Name != "nightly" || req.RunbookName != "health" || req.Target != "server:srv_demo" || req.IntervalSeconds != 3600 || req.Params["service"] != "nginx" {
				t.Errorf("unexpected schedule request: %+v", req)
			}
			created = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"created"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/schedules":
			_ = json.NewEncoder(w).Encode(client.ListSchedulesResponse{Schedules: []client.Schedule{{ID: "sch_new", Name: "nightly", Enabled: true}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	model := newTUIModel(client.New(server.URL))
	model.screen = screenScheduleCreate
	model.scheduleName.SetValue("nightly")
	model.scheduleRunbook.SetValue("health")
	model.scheduleTarget.SetValue("server:srv_demo")
	model.scheduleReason.SetValue("nightly checks")
	model.scheduleParams.SetValue("service=nginx")
	model.scheduleInterval.SetValue("3600")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := updated.(tuiModel)
	if !created || result.screen != screenSchedules || result.err != "" || len(result.schedules) != 1 {
		t.Fatalf("schedule creation should post and refresh: created=%v screen=%v err=%q schedules=%+v", created, result.screen, result.err, result.schedules)
	}
}

func TestScheduleDisableRequiresConfirmationAndRefreshes(t *testing.T) {
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/schedules/sch_test":
			deleted = true
			_, _ = w.Write([]byte(`{"status":"disabled"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/schedules":
			_ = json.NewEncoder(w).Encode(client.ListSchedulesResponse{Schedules: []client.Schedule{{ID: "sch_test", Name: "nightly", Enabled: false}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	model := newTUIModel(client.New(server.URL))
	model.screen = screenSchedules
	model.schedules = []client.Schedule{{ID: "sch_test", Name: "nightly", Enabled: true}}
	model.scheduleTable.SetRows(scheduleRows(model.schedules))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	first := updated.(tuiModel)
	if deleted || first.confirm != "sch_test:disable" {
		t.Fatalf("first disable key should only request confirmation, deleted=%v confirm=%q", deleted, first.confirm)
	}
	updated, _ = first.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	second := updated.(tuiModel)
	if !deleted || second.err != "" || len(second.schedules) != 1 || second.schedules[0].Enabled {
		t.Fatalf("confirmed disable should delete and refresh schedule state: deleted=%v err=%q schedules=%+v", deleted, second.err, second.schedules)
	}
}
