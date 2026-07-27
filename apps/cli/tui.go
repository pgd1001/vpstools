package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pgd1001/svrtools/packages/runbooks"
	"github.com/pgd1001/svrtools/packages/sdk-go/client"
)

var (
	tuiAppStyle   = lipgloss.NewStyle().Padding(0, 1)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).MarginBottom(1)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	tableKeyMap   = table.DefaultKeyMap()
	listKeyMap    = list.DefaultKeyMap()
)

type screen int

const (
	screenHome screen = iota
	screenServers
	screenRunbooks
	screenExecutions
	screenApprovals
	screenSchedules
	screenAudit
	screenHelp
	screenExecutionDetail
	screenRunbookRun
	screenScheduleCreate
	screenApprovalDetail
	screenApprovalDeny
)

type tuiModel struct {
	screen screen
	width  int
	height int
	err    string
	msg    string
	client *client.Client

	// Server browser
	serverTable table.Model
	servers     []client.Server

	// Runbook browser
	runbookList list.Model
	runbooks    []client.RunbookItem

	// Execution monitor
	execTable  table.Model
	executions []client.ExecutionListItem

	// Approval queue
	approvalTable table.Model
	approvals     []client.ApprovalItem

	// Automation schedules
	scheduleTable table.Model
	schedules     []client.Schedule

	// Audit browser
	auditInput  textinput.Model
	auditEvents []client.AuditEvent

	// Execution detail
	selectedExec      *client.GetExecutionResponse
	selectedApproval  *client.ApprovalDetail
	selectedRunbook   *client.RunbookItem
	runbookTarget     textinput.Model
	runbookReason     textinput.Model
	runbookParams     textinput.Model
	approvalNote      textinput.Model
	scheduleName      textinput.Model
	scheduleRunbook   textinput.Model
	scheduleTarget    textinput.Model
	scheduleReason    textinput.Model
	scheduleParams    textinput.Model
	scheduleInterval  textinput.Model
	scheduleNextRun   textinput.Model
	pendingApprovalID string
	confirm           string

	quitting bool
}

func newTUIModel(c *client.Client) tuiModel {
	st := table.New(
		table.WithColumns([]table.Column{
			{Title: "ID", Width: 14},
			{Title: "Name", Width: 18},
			{Title: "Hostname", Width: 16},
			{Title: "Env", Width: 14},
			{Title: "Status", Width: 10},
		}),
		table.WithHeight(12),
	)
	stt := table.DefaultStyles()
	stt.Header = stt.Header.Bold(true).Foreground(lipgloss.Color("10"))
	st.SetStyles(stt)

	et := table.New(
		table.WithColumns([]table.Column{
			{Title: "ID", Width: 14},
			{Title: "Status", Width: 10},
			{Title: "Targets", Width: 10},
			{Title: "Command", Width: 40},
		}),
		table.WithHeight(12),
	)
	ets := table.DefaultStyles()
	ets.Header = ets.Header.Bold(true).Foreground(lipgloss.Color("10"))
	et.SetStyles(ets)

	apt := table.New(
		table.WithColumns([]table.Column{
			{Title: "ID", Width: 14},
			{Title: "Requester", Width: 14},
			{Title: "Type", Width: 10},
			{Title: "Status", Width: 10},
			{Title: "Reason", Width: 30},
		}),
		table.WithHeight(12),
	)
	apts := table.DefaultStyles()
	apts.Header = apts.Header.Bold(true).Foreground(lipgloss.Color("10"))
	apt.SetStyles(apts)

	sct := table.New(table.WithColumns([]table.Column{
		{Title: "Name", Width: 20},
		{Title: "Runbook", Width: 20},
		{Title: "Target", Width: 22},
		{Title: "Next run", Width: 20},
		{Title: "Status", Width: 10},
	}), table.WithHeight(12))
	scts := table.DefaultStyles()
	scts.Header = scts.Header.Bold(true).Foreground(lipgloss.Color("10"))
	sct.SetStyles(scts)

	ai := textinput.New()
	ai.Placeholder = "search audits..."
	ai.Width = 40
	targetInput := textinput.New()
	targetInput.Placeholder = "server:demo or tag:role=web"
	targetInput.Width = 52
	reasonInput := textinput.New()
	reasonInput.Placeholder = "Why is this task needed?"
	reasonInput.Width = 52
	paramsInput := textinput.New()
	paramsInput.Placeholder = "name=value,name2=value2"
	paramsInput.Width = 52
	noteInput := textinput.New()
	noteInput.Placeholder = "Why is this approval being denied?"
	noteInput.Width = 52
	scheduleName := textinput.New()
	scheduleName.Placeholder = "nightly-health-check"
	scheduleName.Width = 52
	scheduleRunbook := textinput.New()
	scheduleRunbook.Placeholder = "Published runbook name"
	scheduleRunbook.Width = 52
	scheduleTarget := textinput.New()
	scheduleTarget.Placeholder = "server:demo or tag:role=web"
	scheduleTarget.Width = 52
	scheduleReason := textinput.New()
	scheduleReason.Placeholder = "Why should this run automatically?"
	scheduleReason.Width = 52
	scheduleParams := textinput.New()
	scheduleParams.Placeholder = "name=value,name2=value2"
	scheduleParams.Width = 52
	scheduleInterval := textinput.New()
	scheduleInterval.Placeholder = "3600"
	scheduleInterval.Width = 20
	scheduleNextRun := textinput.New()
	scheduleNextRun.Placeholder = "Optional RFC3339 timestamp"
	scheduleNextRun.Width = 35

	rbItems := []list.Item{}
	rbList := list.New(rbItems, list.NewDefaultDelegate(), 0, 0)
	rbList.Title = "Runbooks"
	rbList.SetShowStatusBar(false)
	rbList.SetFilteringEnabled(true)
	rbList.SetShowHelp(false)

	return tuiModel{
		screen:           screenHome,
		client:           c,
		serverTable:      st,
		execTable:        et,
		approvalTable:    apt,
		scheduleTable:    sct,
		auditInput:       ai,
		runbookList:      rbList,
		runbookTarget:    targetInput,
		runbookReason:    reasonInput,
		runbookParams:    paramsInput,
		approvalNote:     noteInput,
		scheduleName:     scheduleName,
		scheduleRunbook:  scheduleRunbook,
		scheduleTarget:   scheduleTarget,
		scheduleReason:   scheduleReason,
		scheduleParams:   scheduleParams,
		scheduleInterval: scheduleInterval,
		scheduleNextRun:  scheduleNextRun,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentWidth := msg.Width - 4
		if contentWidth < 20 {
			contentWidth = 20
		}
		contentHeight := msg.Height - 8
		if contentHeight < 4 {
			contentHeight = 4
		}
		m.serverTable.SetWidth(contentWidth)
		m.execTable.SetWidth(contentWidth)
		m.approvalTable.SetWidth(contentWidth)
		m.scheduleTable.SetWidth(contentWidth)
		m.runbookList.SetWidth(contentWidth)
		m.runbookList.SetHeight(contentHeight)
		return m, nil

	case tea.KeyMsg:
		if m.screen == screenRunbookRun {
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				m.screen = screenRunbooks
				m.selectedRunbook = nil
				m.runbookTarget.Blur()
				m.runbookReason.Blur()
				m.runbookParams.Blur()
				return m, nil
			case "tab", "shift+tab":
				m.focusRunbookInput(msg.String() == "tab")
				return m, nil
			case "p":
				params, err := runbooks.ParseParameterValues(m.runbookParams.Value())
				if err != nil {
					m.err = "preflight failed: " + err.Error()
					return m, nil
				}
				response, err := m.client.PreflightRunbook(m.selectedRunbook.Name, strings.TrimSpace(m.runbookTarget.Value()), strings.TrimSpace(m.runbookReason.Value()), params)
				if err != nil {
					m.err = "preflight failed: " + err.Error()
					return m, nil
				}
				m.err = ""
				if response["approval_required"] == true {
					m.msg = "Preflight passed. Approval is required before execution."
				} else {
					m.msg = "Preflight passed. Press Enter to submit the task."
				}
				return m, nil
			case "enter":
				if strings.TrimSpace(m.runbookTarget.Value()) == "" {
					m.err = "target is required"
					return m, nil
				}
				params, err := runbooks.ParseParameterValues(m.runbookParams.Value())
				if err != nil {
					m.err = "task not submitted: " + err.Error()
					return m, nil
				}
				response, err := m.client.RunRunbook(m.selectedRunbook.Name, strings.TrimSpace(m.runbookTarget.Value()), strings.TrimSpace(m.runbookReason.Value()), params)
				if err != nil {
					m.err = "task submission failed: " + err.Error()
					return m, nil
				}
				m.err = ""
				if response["status"] == "awaiting_approval" {
					m.msg = "Task submitted. Approval is required before execution."
				} else if executionID, ok := response["execution_id"].(string); ok && executionID != "" {
					m.msg = "Task queued. Execution ID: " + executionID
				} else {
					m.msg = "Task submitted."
				}
				m.screen = screenRunbooks
				m.selectedRunbook = nil
				m.runbookTarget.Blur()
				m.runbookReason.Blur()
				m.runbookParams.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			switch {
			case m.runbookTarget.Focused():
				m.runbookTarget, cmd = m.runbookTarget.Update(msg)
			case m.runbookReason.Focused():
				m.runbookReason, cmd = m.runbookReason.Update(msg)
			default:
				m.runbookParams, cmd = m.runbookParams.Update(msg)
			}
			return m, cmd
		}
		if m.screen == screenScheduleCreate {
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				m.screen = screenSchedules
				m.blurScheduleInputs()
				return m, nil
			case "tab", "shift+tab":
				m.focusScheduleInput(msg.String() == "tab")
				return m, nil
			case "enter":
				name := strings.TrimSpace(m.scheduleName.Value())
				runbook := strings.TrimSpace(m.scheduleRunbook.Value())
				target := strings.TrimSpace(m.scheduleTarget.Value())
				reason := strings.TrimSpace(m.scheduleReason.Value())
				if name == "" || runbook == "" || target == "" || reason == "" {
					m.err = "name, runbook, target, and reason are required"
					return m, nil
				}
				interval, err := strconv.Atoi(strings.TrimSpace(m.scheduleInterval.Value()))
				if err != nil || interval < 60 {
					m.err = "interval must be at least 60 seconds"
					return m, nil
				}
				params, err := runbooks.ParseParameterValues(m.scheduleParams.Value())
				if err != nil {
					m.err = "schedule not created: " + err.Error()
					return m, nil
				}
				_, err = m.client.CreateSchedule(client.CreateScheduleRequest{
					Name: name, RunbookName: runbook, Target: target, Reason: reason,
					Params: params, IntervalSeconds: interval, NextRunAt: strings.TrimSpace(m.scheduleNextRun.Value()),
				})
				if err != nil {
					m.err = "schedule creation failed: " + err.Error()
					return m, nil
				}
				m.err = ""
				m.msg = "Created schedule " + name
				m.blurScheduleInputs()
				m.screen = screenSchedules
				if schedules, refreshErr := m.client.ListSchedules(); refreshErr == nil && schedules != nil {
					m.schedules = schedules.Schedules
					m.scheduleTable.SetRows(scheduleRows(schedules.Schedules))
				}
				return m, nil
			}
			var cmd tea.Cmd
			switch {
			case m.scheduleName.Focused():
				m.scheduleName, cmd = m.scheduleName.Update(msg)
			case m.scheduleRunbook.Focused():
				m.scheduleRunbook, cmd = m.scheduleRunbook.Update(msg)
			case m.scheduleTarget.Focused():
				m.scheduleTarget, cmd = m.scheduleTarget.Update(msg)
			case m.scheduleReason.Focused():
				m.scheduleReason, cmd = m.scheduleReason.Update(msg)
			case m.scheduleParams.Focused():
				m.scheduleParams, cmd = m.scheduleParams.Update(msg)
			case m.scheduleInterval.Focused():
				m.scheduleInterval, cmd = m.scheduleInterval.Update(msg)
			default:
				m.scheduleNextRun, cmd = m.scheduleNextRun.Update(msg)
			}
			return m, cmd
		}
		if m.screen == screenExecutionDetail {
			switch msg.String() {
			case "q", "esc":
				m.screen = screenExecutions
				m.selectedExec = nil
				m.confirm = ""
				return m, nil
			case "c":
				if m.selectedExec == nil || (m.selectedExec.Execution.Status != "created" && m.selectedExec.Execution.Status != "queued") {
					m.err = "only created or queued executions can be cancelled"
					return m, nil
				}
				executionID := m.selectedExec.Execution.ID
				if m.confirm != executionID+":cancel" {
					m.confirm = executionID + ":cancel"
					m.msg = "Press c again to cancel " + executionID
					return m, nil
				}
				m.confirm = ""
				if _, err := m.client.CancelExecution(executionID); err != nil {
					m.err = "cancel failed: " + err.Error()
					return m, nil
				}
				if refreshed, err := m.client.GetExecution(executionID); err == nil && refreshed != nil {
					m.selectedExec = refreshed
				}
				if refreshed, err := m.client.ListExecutions("", "50"); err == nil && refreshed != nil {
					m.executions = refreshed.Executions
					m.execTable.SetRows(execRows(refreshed.Executions))
				}
				m.err = ""
				m.msg = "Execution cancelled: " + executionID
				return m, nil
			}
		}
		if m.screen == screenApprovalDeny {
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				m.screen = screenApprovals
				m.approvalNote.Blur()
				m.pendingApprovalID = ""
				return m, nil
			case "enter":
				note := strings.TrimSpace(m.approvalNote.Value())
				if note == "" {
					m.err = "a denial note is required"
					return m, nil
				}
				if _, err := m.client.DenyApproval(m.pendingApprovalID, note); err != nil {
					m.err = "deny failed: " + err.Error()
					return m, nil
				}
				m.msg = "Denied " + m.pendingApprovalID
				m.err = ""
				m.approvalNote.Blur()
				m.pendingApprovalID = ""
				m.screen = screenApprovals
				if a, err := m.client.ListApprovals(""); err == nil && a != nil {
					m.approvals = a.Approvals
					m.approvalTable.SetRows(approvalRows(a.Approvals))
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.approvalNote, cmd = m.approvalNote.Update(msg)
			return m, cmd
		}
		if m.screen == screenAudit {
			switch msg.String() {
			case "ctrl+c", "q":
				m.screen = screenHome
				m.auditInput.Blur()
				return m, nil
			case "h":
				m.screen = screenHelp
				m.auditInput.Blur()
				return m, nil
			case "r", "enter":
				actor := m.auditInput.Value()
				events, err := m.client.ListAudit("50")
				if err != nil {
					m.err = "audit refresh failed: " + err.Error()
					return m, nil
				}
				if actor != "" {
					filtered := []client.AuditEvent{}
					for _, e := range events.Events {
						if e.ActorID == actor || e.Action == actor {
							filtered = append(filtered, e)
						}
					}
					m.auditEvents = filtered
				} else {
					m.auditEvents = events.Events
				}
				m.msg = fmt.Sprintf("Showing %d audit events", len(m.auditEvents))
				return m, nil
			}
			var cmd tea.Cmd
			m.auditInput, cmd = m.auditInput.Update(msg)
			return m, cmd
		}

		// Delegate navigation to active component
		if m.screen == screenServers {
			m.serverTable, _ = m.serverTable.Update(msg)
		}
		if m.screen == screenExecutions {
			m.execTable, _ = m.execTable.Update(msg)
		}
		if m.screen == screenApprovals {
			m.approvalTable, _ = m.approvalTable.Update(msg)
		}
		if m.screen == screenSchedules {
			m.scheduleTable, _ = m.scheduleTable.Update(msg)
		}
		if m.screen == screenRunbooks {
			m.runbookList, _ = m.runbookList.Update(msg)
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.screen == screenHome {
				m.quitting = true
				return m, tea.Quit
			}
			m.screen = screenHome
			m.err = ""
			return m, nil
		case "1":
			m.screen = screenServers
			m.err = ""
			servers, _ := m.client.ListServers("", "", "")
			if servers != nil {
				m.servers = servers.Servers
				m.serverTable.SetRows(serverRows(servers.Servers))
			}
			return m, nil
		case "2":
			m.screen = screenRunbooks
			m.err = ""
			rbs, _ := m.client.ListRunbooks()
			if rbs != nil {
				m.runbooks = rbs.Runbooks
				items := []list.Item{}
				for _, rb := range rbs.Runbooks {
					items = append(items, runbookListItem{rb})
				}
				m.runbookList.SetItems(items)
			}
			return m, nil

		case "3":
			m.screen = screenExecutions
			m.err = ""
			execs, _ := m.client.ListExecutions("", "50")
			if execs != nil {
				m.executions = execs.Executions
				m.execTable.SetRows(execRows(execs.Executions))
			}
			return m, nil
		case "4":
			m.screen = screenApprovals
			m.err = ""
			approvals, _ := m.client.ListApprovals("")
			if approvals != nil {
				m.approvals = approvals.Approvals
				m.approvalTable.SetRows(approvalRows(approvals.Approvals))
			}
			return m, nil
		case "5":
			m.screen = screenSchedules
			m.err = ""
			schedules, err := m.client.ListSchedules()
			if err != nil {
				m.err = "schedule list failed: " + err.Error()
			} else if schedules != nil {
				m.schedules = schedules.Schedules
				m.scheduleTable.SetRows(scheduleRows(schedules.Schedules))
			}
			return m, nil
		case "6":
			m.screen = screenAudit
			m.err = ""
			m.auditInput.Focus()
			return m, tea.Batch(textinput.Blink)
		case "h":
			m.screen = screenHelp
			return m, nil
		case "r":
			switch m.screen {
			case screenServers:
				s, _ := m.client.ListServers("", "", "")
				if s != nil {
					m.servers = s.Servers
					m.serverTable.SetRows(serverRows(s.Servers))
				}
			case screenExecutions:
				e, _ := m.client.ListExecutions("", "50")
				if e != nil {
					m.executions = e.Executions
					m.execTable.SetRows(execRows(e.Executions))
				}
			case screenApprovals:
				a, _ := m.client.ListApprovals("")
				if a != nil {
					m.approvals = a.Approvals
					m.approvalTable.SetRows(approvalRows(a.Approvals))
				}
			case screenSchedules:
				s, err := m.client.ListSchedules()
				if err != nil {
					m.err = "schedule refresh failed: " + err.Error()
				} else if s != nil {
					m.schedules = s.Schedules
					m.scheduleTable.SetRows(scheduleRows(s.Schedules))
				}
			case screenRunbooks:
				r, _ := m.client.ListRunbooks()
				if r != nil {
					m.runbooks = r.Runbooks
					items := []list.Item{}
					for _, rb := range r.Runbooks {
						items = append(items, runbookListItem{rb})
					}
					m.runbookList.SetItems(items)
				}
			case screenAudit:
				a, _ := m.client.ListAudit("50")
				if a != nil {
					m.auditEvents = a.Events
				}
			}
			return m, nil
		case "enter":
			if m.screen == screenRunbooks {
				idx := m.runbookList.Index()
				if idx >= 0 && idx < len(m.runbooks) {
					rb := m.runbooks[idx]
					m.msg = fmt.Sprintf("Runbook: %s  Risk: %s  Permitted: %v  Role: %s",
						rb.Name, rb.Risk, rb.Permitted, rb.AllowedRoles)
				}
				return m, nil
			}
			if m.screen == screenAudit {
				actor := m.auditInput.Value()
				events, _ := m.client.ListAudit("50")
				if events != nil && actor != "" {
					filtered := []client.AuditEvent{}
					for _, e := range events.Events {
						if e.ActorID == actor || e.Action == actor {
							filtered = append(filtered, e)
						}
					}
					m.auditEvents = filtered
				} else if events != nil {
					m.auditEvents = events.Events
				}
			}
			if m.screen == screenExecutions {
				cursor := m.execTable.Cursor()
				if cursor >= 0 && cursor < len(m.executions) {
					execID := m.executions[cursor].ID
					exec, err := m.client.GetExecution(execID)
					if err == nil && exec != nil {
						m.selectedExec = exec
						m.screen = screenExecutionDetail
					}
				}
			}
			if m.screen == screenApprovals {
				cursor := m.approvalTable.Cursor()
				if cursor >= 0 && cursor < len(m.approvals) {
					approval, err := m.client.GetApproval(m.approvals[cursor].ID)
					if err != nil {
						m.err = "approval detail failed: " + err.Error()
					} else {
						m.selectedApproval = approval
						m.screen = screenApprovalDetail
					}
				}
				return m, nil
			}
			if m.screen == screenExecutionDetail {
				m.screen = screenExecutions
				m.selectedExec = nil
			}
			if m.screen == screenApprovalDetail {
				m.screen = screenApprovals
				m.selectedApproval = nil
			}
			return m, nil
		case "x":
			if m.screen == screenRunbooks {
				idx := m.runbookList.Index()
				if idx >= 0 && idx < len(m.runbooks) {
					rb := m.runbooks[idx]
					if !rb.Permitted {
						m.err = "you are not permitted to run this runbook"
						return m, nil
					}
					m.selectedRunbook = &rb
					m.runbookTarget.SetValue("server:srv_demo")
					m.runbookReason.SetValue("")
					m.runbookParams.SetValue("")
					m.runbookReason.Blur()
					m.runbookParams.Blur()
					m.runbookTarget.Focus()
					m.screen = screenRunbookRun
					m.err = ""
					m.msg = "Press p for preflight, then Enter to submit."
				}
			}
			return m, nil
		case "a":
			if m.screen == screenApprovals {
				cursor := m.approvalTable.Cursor()
				if cursor >= 0 && cursor < len(m.approvals) {
					approvalID := m.approvals[cursor].ID
					if m.confirm != approvalID+":approve" {
						m.confirm = approvalID + ":approve"
						m.msg = "Press a again to approve " + approvalID
						return m, nil
					}
					m.confirm = ""
					_, err := m.client.ApproveApproval(approvalID)
					if err != nil {
						m.err = "approve failed: " + err.Error()
					} else {
						m.msg = "Approved " + approvalID
						// refresh approvals
						a, _ := m.client.ListApprovals("")
						if a != nil {
							m.approvals = a.Approvals
							m.approvalTable.SetRows(approvalRows(a.Approvals))
						}
					}
				}
			}
			return m, nil
		case "d":
			if m.screen == screenApprovals {
				cursor := m.approvalTable.Cursor()
				if cursor >= 0 && cursor < len(m.approvals) {
					approvalID := m.approvals[cursor].ID
					if m.confirm != approvalID+":deny" {
						m.confirm = approvalID + ":deny"
						m.msg = "Press d again to deny " + approvalID
						return m, nil
					}
					m.confirm = ""
					m.pendingApprovalID = approvalID
					m.approvalNote.SetValue("")
					m.approvalNote.Focus()
					m.screen = screenApprovalDeny
					m.err = ""
					m.msg = "Enter a reason for denying " + approvalID
				}
			} else if m.screen == screenSchedules {
				cursor := m.scheduleTable.Cursor()
				if cursor >= 0 && cursor < len(m.schedules) {
					schedule := m.schedules[cursor]
					if !schedule.Enabled {
						m.msg = "Schedule is already disabled"
						return m, nil
					}
					if m.confirm != schedule.ID+":disable" {
						m.confirm = schedule.ID + ":disable"
						m.msg = "Press d again to disable " + schedule.Name
						return m, nil
					}
					m.confirm = ""
					if _, err := m.client.DisableSchedule(schedule.ID); err != nil {
						m.err = "disable schedule failed: " + err.Error()
					} else {
						m.msg = "Disabled " + schedule.Name
						if schedules, err := m.client.ListSchedules(); err == nil && schedules != nil {
							m.schedules = schedules.Schedules
							m.scheduleTable.SetRows(scheduleRows(schedules.Schedules))
						}
					}
				}
			}
			return m, nil
		case "n":
			if m.screen == screenSchedules {
				m.scheduleName.SetValue("")
				m.scheduleRunbook.SetValue("")
				m.scheduleTarget.SetValue("server:srv_demo")
				m.scheduleReason.SetValue("")
				m.scheduleParams.SetValue("")
				m.scheduleInterval.SetValue("3600")
				m.scheduleNextRun.SetValue("")
				m.blurScheduleInputs()
				m.scheduleName.Focus()
				m.screen = screenScheduleCreate
				m.err = ""
				m.msg = "Create a schedule for a published runbook."
				return m, tea.Batch(textinput.Blink)
			}
			return m, nil
		}
	}

	return m, nil
}

func (m tuiModel) View() string {
	switch m.screen {
	case screenHome:
		return m.homeView()
	case screenServers:
		return m.serversView()
	case screenRunbooks:
		return m.runbooksView()
	case screenExecutions:
		return m.executionsView()
	case screenApprovals:
		return m.approvalsView()
	case screenSchedules:
		return m.schedulesView()
	case screenAudit:
		return m.auditView()
	case screenHelp:
		return m.helpView()
	case screenExecutionDetail:
		return m.executionDetailView()
	case screenRunbookRun:
		return m.runbookRunView()
	case screenScheduleCreate:
		return m.scheduleCreateView()
	case screenApprovalDetail:
		return m.approvalDetailView()
	case screenApprovalDeny:
		return m.approvalDenyView()
	}
	return ""
}

func (m *tuiModel) focusRunbookInput(forward bool) {
	inputs := []*textinput.Model{&m.runbookTarget, &m.runbookReason, &m.runbookParams}
	current := 0
	for i, input := range inputs {
		if input.Focused() {
			current = i
			input.Blur()
			break
		}
	}
	if forward {
		current = (current + 1) % len(inputs)
	} else {
		current = (current + len(inputs) - 1) % len(inputs)
	}
	inputs[current].Focus()
}

func (m *tuiModel) focusScheduleInput(forward bool) {
	inputs := []*textinput.Model{&m.scheduleName, &m.scheduleRunbook, &m.scheduleTarget, &m.scheduleReason, &m.scheduleParams, &m.scheduleInterval, &m.scheduleNextRun}
	current := 0
	for i, input := range inputs {
		if input.Focused() {
			current = i
			input.Blur()
			break
		}
	}
	if forward {
		current = (current + 1) % len(inputs)
	} else {
		current = (current + len(inputs) - 1) % len(inputs)
	}
	inputs[current].Focus()
}

func (m *tuiModel) blurScheduleInputs() {
	for _, input := range []*textinput.Model{&m.scheduleName, &m.scheduleRunbook, &m.scheduleTarget, &m.scheduleReason, &m.scheduleParams, &m.scheduleInterval, &m.scheduleNextRun} {
		input.Blur()
	}
}

func (m tuiModel) runbookRunView() string {
	if m.selectedRunbook == nil {
		return tuiAppStyle.Render(titleStyle.Render("Run task") + "\n" + errorStyle.Render("No runbook selected.") + "\n" + helpStyle.Render("[esc] back"))
	}
	s := titleStyle.Render("Run task: "+m.selectedRunbook.Name) + "\n\n"
	s += fmt.Sprintf("  Risk: %s\n  %s\n\n", m.selectedRunbook.Risk, m.selectedRunbook.Description)
	s += "  Target\n  " + m.runbookTarget.View() + "\n\n"
	s += "  Reason\n  " + m.runbookReason.View() + "\n\n"
	s += "  Parameters, name=value pairs\n  " + m.runbookParams.View() + "\n\n"
	if m.err != "" {
		s += errorStyle.Render(m.err) + "\n"
	}
	if m.msg != "" {
		s += selectedStyle.Render(m.msg) + "\n"
	}
	s += "\n" + helpStyle.Render("[tab] next field  [p] preflight  [enter] submit  [esc] back")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) homeView() string {
	s := titleStyle.Render("VPS Tools TUI") + "\n\n"
	s += "  [1] Servers         Browse and inspect VPS inventory\n"
	s += "  [2] Runbooks        View and launch runbooks\n"
	s += "  [3] Executions      Monitor running and completed jobs\n"
	s += "  [4] Approvals       Review and decide approval requests\n"
	s += "  [5] Schedules       View automated run schedules\n"
	s += "  [6] Audit           Search audit events\n\n"
	s += "  [h] Help            Show keybindings\n"
	s += "  [q] Quit\n\n"
	if m.err != "" {
		s += errorStyle.Render(m.err) + "\n"
	}
	if m.msg != "" {
		s += m.msg + "\n"
	}
	return tuiAppStyle.Render(s)
}

func (m tuiModel) serversView() string {
	s := titleStyle.Render("Servers") + "\n"
	if len(m.servers) == 0 {
		s += dimStyle.Render("No servers found. Use 'vps server add' to register servers.") + "\n"
	} else {
		s += m.serverTable.View() + "\n"
	}
	s += helpStyle.Render("[q] back")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) runbooksView() string {
	s := titleStyle.Render("Runbooks") + "\n"
	if len(m.runbooks) == 0 {
		s += dimStyle.Render("No runbooks available.") + "\n"
	} else {
		s += m.runbookList.View() + "\n"
	}
	s += helpStyle.Render("[q] back  [↑↓] navigate  [/] search name/title/tags")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) executionsView() string {
	s := titleStyle.Render("Executions") + "\n"
	if len(m.executions) == 0 {
		s += dimStyle.Render("No executions yet.") + "\n"
	} else {
		s += m.execTable.View() + "\n"
	}
	s += helpStyle.Render("[q] back  [r] refresh  [enter] view detail")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) executionDetailView() string {
	if m.selectedExec == nil {
		return tuiAppStyle.Render(titleStyle.Render("Execution Detail") + "\n" + dimStyle.Render("No execution selected.") + "\n" + helpStyle.Render("[q] back"))
	}
	e := m.selectedExec.Execution
	s := titleStyle.Render("Execution Detail") + "\n\n"
	s += fmt.Sprintf("  ID:       %s\n", e.ID)
	s += fmt.Sprintf("  Status:   %s\n", e.Status)
	s += fmt.Sprintf("  Actor:    %s (%s)\n", e.ActorUserID, e.ActorRole)
	s += fmt.Sprintf("  Command:  %s\n", e.CommandPreview)
	s += fmt.Sprintf("  Risk:     %s\n", e.RiskLevel)
	s += fmt.Sprintf("  Reason:   %s\n", e.Reason)
	if e.DelegatedBy != "" {
		s += fmt.Sprintf("  Delegated by: %s\n", e.DelegatedBy)
	}
	if e.ApprovalID != "" {
		s += fmt.Sprintf("  Approval: %s\n", e.ApprovalID)
	}
	s += fmt.Sprintf("  Requested: %s\n", e.RequestedAt)
	if e.StartedAt != "" {
		s += fmt.Sprintf("  Started:   %s\n", e.StartedAt)
	}
	if e.FinishedAt != "" {
		s += fmt.Sprintf("  Finished:  %s\n", e.FinishedAt)
	}
	if e.ErrorSummary != "" {
		s += fmt.Sprintf("  Error:    %s\n", e.ErrorSummary)
	}
	if len(m.selectedExec.Targets) > 0 {
		s += "\n  Targets:\n"
		for _, t := range m.selectedExec.Targets {
			marker := "[?]"
			switch t.Status {
			case "succeeded":
				marker = "[OK]"
			case "failed":
				marker = "[FAIL]"
			case "running":
				marker = "[RUN]"
			case "cancelled":
				marker = "[CANCEL]"
			}
			s += fmt.Sprintf("    %s %s exit=%d\n", marker, t.ServerID, t.ExitCode)
			if t.Stdout != "" {
				s += fmt.Sprintf("    stdout: %s\n", strings.TrimSpace(t.Stdout))
			}
			if t.Stderr != "" {
				s += fmt.Sprintf("    stderr: %s\n", strings.TrimSpace(t.Stderr))
			}
			if t.Error != "" {
				s += fmt.Sprintf("    error: %s\n", t.Error)
			}
		}
	}
	help := "[q] back  [enter] close"
	if e.Status == "created" || e.Status == "queued" {
		help += "  [c] cancel"
	}
	s += "\n" + helpStyle.Render(help)
	return tuiAppStyle.Render(s)
}

func (m tuiModel) approvalsView() string {
	s := titleStyle.Render("Approvals") + "\n"
	if len(m.approvals) == 0 {
		s += dimStyle.Render("No pending approvals.") + "\n"
	} else {
		s += m.approvalTable.View() + "\n"
	}
	s += helpStyle.Render("[q] back  [r] refresh  [a] approve  [d] deny")
	if m.msg != "" {
		s += "\n" + selectedStyle.Render(m.msg)
	}
	if m.err != "" {
		s += "\n" + errorStyle.Render(m.err)
	}
	return tuiAppStyle.Render(s)
}

func (m tuiModel) approvalDetailView() string {
	if m.selectedApproval == nil {
		return tuiAppStyle.Render(titleStyle.Render("Approval detail") + "\n" + errorStyle.Render("No approval selected.") + "\n" + helpStyle.Render("[enter] back"))
	}
	a := m.selectedApproval
	s := titleStyle.Render("Approval: "+a.ID) + "\n\n"
	s += fmt.Sprintf("  Requester: %s\n  Action: %s\n  Status: %s\n  Risk: %s\n  Target: %s:%s\n  Created: %s\n  Expires: %s\n  Reason: %s\n", a.RequesterName, a.ActionType, a.Status, a.RiskLevel, a.TargetType, a.TargetID, a.CreatedAt, a.ExpiresAt, a.Reason)
	s += "\n  Target snapshot:\n" + a.TargetSnapshot + "\n"
	if len(a.RequestPayload) > 0 {
		if payload, err := json.MarshalIndent(a.RequestPayload, "  ", "  "); err == nil {
			s += "\n  Proposed action, parameters, and evidence plan:\n  " + strings.ReplaceAll(string(payload), "\n", "\n  ") + "\n"
		}
	}
	if a.DecisionNote != "" {
		s += "\n  Decision note: " + a.DecisionNote + "\n"
	}
	s += "\n" + helpStyle.Render("[enter] back  [q] home")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) approvalDenyView() string {
	s := titleStyle.Render("Deny approval") + "\n\n"
	s += fmt.Sprintf("  Approval: %s\n\n  Reason\n  %s\n", m.pendingApprovalID, m.approvalNote.View())
	if m.err != "" {
		s += "\n" + errorStyle.Render(m.err) + "\n"
	}
	s += "\n" + helpStyle.Render("[enter] deny  [esc] back")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) schedulesView() string {
	s := titleStyle.Render("Schedules") + "\n"
	if len(m.schedules) == 0 {
		s += dimStyle.Render("No schedules found. Press n to create one.") + "\n"
	} else {
		s += m.scheduleTable.View() + "\n"
	}
	s += helpStyle.Render("[q] back  [r] refresh  [n] new schedule  [d] disable selected schedule")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) scheduleCreateView() string {
	s := titleStyle.Render("Create schedule") + "\n\n"
	s += "  Name\n  " + m.scheduleName.View() + "\n\n"
	s += "  Published runbook\n  " + m.scheduleRunbook.View() + "\n\n"
	s += "  Target\n  " + m.scheduleTarget.View() + "\n\n"
	s += "  Reason\n  " + m.scheduleReason.View() + "\n\n"
	s += "  Parameters, name=value pairs\n  " + m.scheduleParams.View() + "\n\n"
	s += "  Interval, seconds\n  " + m.scheduleInterval.View() + "\n\n"
	s += "  First run, optional RFC3339\n  " + m.scheduleNextRun.View() + "\n"
	if m.err != "" {
		s += "\n" + errorStyle.Render(m.err) + "\n"
	}
	s += "\n" + helpStyle.Render("[tab] next field  [enter] create  [esc] back")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) auditView() string {
	s := titleStyle.Render("Audit Search") + "\n"
	s += m.auditInput.View() + "\n\n"
	if len(m.auditEvents) == 0 {
		s += dimStyle.Render("Type to search or press Enter to view recent events.") + "\n"
	} else {
		for _, e := range m.auditEvents {
			s += fmt.Sprintf("  %s  %s  %-12s  %s\n",
				dimStyle.Render(e.CreatedAt[:19]), e.Result, e.Action, e.TargetType)
		}
	}
	s += "\n" + helpStyle.Render("[q] back  [enter] search")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) helpView() string {
	s := titleStyle.Render("Help") + "\n"
	s += "Navigation:\n"
	s += "  1-6        Switch to view\n"
	s += "  q          Back / Quit\n"
	s += "  h          Help\n"
	s += "  ↑↓ / j,k   Navigate lists\n"
	s += "  r          Refresh current view\n"
	s += "  enter      Select / Confirm\n"
	s += "  /          Filter list (runbook search)\n\n"
	s += "Actions:\n"
	s += "  Executions:  enter = view detail, c = cancel queued work\n"
	s += "  Approvals:   a = approve selected, d = deny selected with a reason\n\n"
	s += "  Schedules:   d = disable selected schedule\n\n"
	s += "CLI equivalents:\n"
	s += "  vps server list\n"
	s += "  vps runbook list | vps run\n"
	s += "  vps exec status <id>\n"
	s += "  vps approvals list | approve | deny\n"
	s += "  vps audit search\n\n"
	s += helpStyle.Render("[q] back")
	return tuiAppStyle.Render(s)
}

type runbookListItem struct {
	rb client.RunbookItem
}

func (i runbookListItem) Title() string {
	return fmt.Sprintf("%s  [%s]  %s", i.rb.Name, i.rb.Risk, i.rb.Command)
}
func (i runbookListItem) Description() string {
	perm := ""
	if !i.rb.Permitted {
		perm = " [restricted]"
	}
	return fmt.Sprintf("%s - %s%s", i.rb.Status, i.rb.Title, perm)
}
func (i runbookListItem) FilterValue() string {
	return i.rb.Name + " " + i.rb.Title + " " + i.rb.Command + " " + i.rb.Description + " " + i.rb.Risk + " " + i.rb.Status
}

func serverRows(servers []client.Server) []table.Row {
	rows := []table.Row{}
	for _, s := range servers {
		rows = append(rows, table.Row{s.ID, s.Name, s.Hostname, s.Environment, s.Status})
	}
	return rows
}

func execRows(executions []client.ExecutionListItem) []table.Row {
	rows := []table.Row{}
	for _, e := range executions {
		targets := fmt.Sprintf("%d/%d/%d", e.SucceededCount, e.FailedCount, e.TargetCount)
		rows = append(rows, table.Row{e.ID, e.Status, targets, truncate(e.CommandPreview, 40)})
	}
	return rows
}

func approvalRows(approvals []client.ApprovalItem) []table.Row {
	rows := []table.Row{}
	for _, a := range approvals {
		rows = append(rows, table.Row{a.ID, a.RequesterName, a.ActionType, a.Status, a.Reason})
	}
	return rows
}

func scheduleRows(schedules []client.Schedule) []table.Row {
	rows := []table.Row{}
	for _, schedule := range schedules {
		status := "enabled"
		if !schedule.Enabled {
			status = "disabled"
		}
		rows = append(rows, table.Row{schedule.Name, schedule.RunbookName, schedule.Target, schedule.NextRunAt, status})
	}
	return rows
}
