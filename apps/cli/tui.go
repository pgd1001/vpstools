package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pgd1001/svrtools/packages/sdk-go/client"
)

var (
	tuiAppStyle     = lipgloss.NewStyle().Padding(0, 1)
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).MarginBottom(1)
	helpStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	dimStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	tableKeyMap     = table.DefaultKeyMap()
	listKeyMap      = list.DefaultKeyMap()
)

type screen int

const (
	screenHome screen = iota
	screenServers
	screenRunbooks
	screenExecutions
	screenApprovals
	screenAudit
	screenHelp
)

type tuiModel struct {
	screen      screen
	width       int
	height      int
	err         string
	msg         string
	client      *client.Client

	// Server browser
	serverTable  table.Model
	servers      []client.Server

	// Runbook browser
	runbookList  list.Model
	runbooks     []client.RunbookItem

	// Execution monitor
	execTable    table.Model
	executions   []client.ExecutionListItem

	// Approval queue
	approvalTable table.Model
	approvals     []client.ApprovalItem

	// Audit browser
	auditInput   textinput.Model
	auditEvents  []client.AuditEvent

	quitting     bool
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

	ai := textinput.New()
	ai.Placeholder = "search audits..."
	ai.Width = 40

	rbItems := []list.Item{}
	rbList := list.New(rbItems, list.NewDefaultDelegate(), 0, 0)
	rbList.Title = "Runbooks"
	rbList.SetShowStatusBar(false)
	rbList.SetFilteringEnabled(false)
	rbList.SetShowHelp(false)

	return tuiModel{
		screen:        screenHome,
		client:        c,
		serverTable:   st,
		execTable:     et,
		approvalTable: apt,
		auditInput:    ai,
		runbookList:   rbList,
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
		m.serverTable.SetWidth(msg.Width - 4)
		m.execTable.SetWidth(msg.Width - 4)
		m.approvalTable.SetWidth(msg.Width - 4)
		m.runbookList.SetWidth(msg.Width - 4)
		m.runbookList.SetHeight(msg.Height - 8)
		return m, nil

	case tea.KeyMsg:
		if m.screen == screenAudit {
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
	case screenAudit:
		return m.auditView()
	case screenHelp:
		return m.helpView()
	}
	return ""
}

func (m tuiModel) homeView() string {
	s := titleStyle.Render("VPS Tools TUI") + "\n\n"
	s += "  [1] Servers         Browse and inspect VPS inventory\n"
	s += "  [2] Runbooks        View and launch runbooks\n"
	s += "  [3] Executions      Monitor running and completed jobs\n"
	s += "  [4] Approvals       Review and decide approval requests\n"
	s += "  [5] Audit           Search audit events\n\n"
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
	s += helpStyle.Render("[q] back  [↑↓] navigate")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) executionsView() string {
	s := titleStyle.Render("Executions") + "\n"
	if len(m.executions) == 0 {
		s += dimStyle.Render("No executions yet.") + "\n"
	} else {
		s += m.execTable.View() + "\n"
	}
	s += helpStyle.Render("[q] back  [r] refresh")
	return tuiAppStyle.Render(s)
}

func (m tuiModel) approvalsView() string {
	s := titleStyle.Render("Approvals") + "\n"
	if len(m.approvals) == 0 {
		s += dimStyle.Render("No pending approvals.") + "\n"
	} else {
		s += m.approvalTable.View() + "\n"
	}
	s += helpStyle.Render("[q] back  [r] refresh")
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
	s += "  1-5        Switch to view\n"
	s += "  q          Back / Quit\n"
	s += "  h          Help\n"
	s += "  ↑↓ / j,k   Navigate lists\n"
	s += "  r          Refresh current view\n"
	s += "  enter      Select / Confirm\n\n"
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
	return fmt.Sprintf("%s — %s%s", i.rb.Status, i.rb.Title, perm)
}
func (i runbookListItem) FilterValue() string {
	return i.rb.Name + " " + i.rb.Title + " " + i.rb.Command
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
