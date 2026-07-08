package authz

import (
	"context"
	"database/sql"
	"fmt"
)

type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func Deny(reason, message string) Decision {
	return Decision{Allowed: false, Reason: reason, Message: message}
}

func Allow() Decision {
	return Decision{Allowed: true}
}

// Policy defines the structured rules for the MVP.
// Kept simple: role-based checks, environment restrictions, reason requirements.
type Policy struct{}

func NewPolicy() *Policy {
	return &Policy{}
}

// CheckExecution evaluates whether an actor can execute a command/runbook against targets.
func (p *Policy) CheckExecution(ctx context.Context, db *sql.DB, actor *Actor, environment Env, risk RiskLevel, reason string) Decision {
	if actor.IsAuditor() {
		return Deny("auditor_cannot_execute", "Auditors cannot execute operations. Use the audit search view instead.")
	}

	if risk == RiskCritical && !actor.HasRole(RoleOwner, RoleAdmin) {
		return Deny("critical_requires_admin", "Critical-risk commands require Owner or Admin role. Request an admin to run this.")
	}

	if risk == RiskHigh && !actor.IsSenior() {
		return Deny("high_risk_requires_senior", "High-risk commands require senior engineer or above.\nJunior engineers can execute a pre-approved runbook instead.")
	}

	if !actor.CanExecuteRaw() {
		return Deny("junior_raw_command_denied", "Junior engineers cannot run arbitrary commands.\nUse 'vps runbook list' to see permitted runbooks, then 'vps runbook run <name> --target <server>'.")
	}

	if environment == EnvProduction && reason == "" {
		return Deny("production_requires_reason", "Production environment actions require a reason. Use --reason \"...\" to explain why.")
	}

	if environment == EnvProduction && !actor.IsSenior() {
		return Deny("junior_production_denied", "Junior engineers cannot target production environments directly.")
	}

	return Allow()
}

// CheckServerManagement evaluates whether an actor can add/modify servers.
func (p *Policy) CheckServerManagement(actor *Actor) Decision {
	if !actor.CanManageServers() {
		return Deny("manage_servers_requires_privileged", fmt.Sprintf("Adding servers requires senior_engineer or above (current: %s)", actor.Role))
	}
	return Allow()
}

// CheckServerCheck evaluates whether an actor can run a server health check.
func (p *Policy) CheckServerCheck(actor *Actor) Decision {
	if !actor.IsPrivileged() {
		return Deny("check_requires_privileged", "Server health checks require senior_engineer or above.")
	}
	return Allow()
}

// CheckRunnerManagement evaluates whether an actor can register/manage runners.
func (p *Policy) CheckRunnerManagement(actor *Actor) Decision {
	if !actor.CanManageRunners() {
		return Deny("manage_runners_requires_privileged", fmt.Sprintf("Managing runners requires senior_engineer or above (current: %s)", actor.Role))
	}
	return Allow()
}

// CheckRunbookExecution evaluates whether an actor can execute a runbook.
func (p *Policy) CheckRunbookExecution(actor *Actor, environment Env, risk RiskLevel, reason string, rbDef map[string]any) Decision {
	if actor.IsAuditor() {
		return Deny("auditor_cannot_execute", "Auditors cannot execute operations.")
	}

	if risk == RiskCritical && !actor.HasRole(RoleOwner, RoleAdmin) {
		return Deny("critical_requires_admin", "Critical-risk runbooks require Owner or Admin role.")
	}

	if risk == RiskHigh && !actor.IsSenior() {
		return Deny("high_risk_requires_senior", "High-risk runbooks require senior engineer or above.")
	}

	if environment == EnvProduction && reason == "" {
		return Deny("production_requires_reason", "Production environment actions require a reason.")
	}

	if environment == EnvProduction && !actor.IsSenior() {
		return Deny("junior_production_denied", "Junior engineers cannot target production environments directly.")
	}

	return Allow()
}

func (p *Policy) DenyMessage(decision Decision) string {
	if decision.Message != "" {
		return decision.Message
	}
	return fmt.Sprintf("Action denied: %s", decision.Reason)
}
