package authz

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type contextKey string

const actorKey contextKey = "actor"

type Actor struct {
	UserID         string `json:"user_id"`
	Email          string `json:"email"`
	DisplayName    string `json:"display_name"`
	OrganisationID string `json:"organisation_id"`
	Role           string `json:"role"`
}

func WithActor(ctx context.Context, actor *Actor) context.Context {
	return context.WithValue(ctx, actorKey, actor)
}

func GetActor(ctx context.Context) *Actor {
	if a, ok := ctx.Value(actorKey).(*Actor); ok {
		return a
	}
	return nil
}

func RequireActor(ctx context.Context) (*Actor, error) {
	a := GetActor(ctx)
	if a == nil {
		return nil, fmt.Errorf("authentication required")
	}
	return a, nil
}

type Role string

const (
	RoleOwner          Role = "owner"
	RoleAdmin          Role = "admin"
	RoleSeniorEngineer Role = "senior_engineer"
	RoleJuniorEngineer Role = "junior_engineer"
	RoleAuditor        Role = "auditor"
)

func (a *Actor) HasRole(roles ...Role) bool {
	for _, r := range roles {
		if string(r) == a.Role {
			return true
		}
	}
	return false
}

func (a *Actor) IsSenior() bool {
	return a.HasRole(RoleOwner, RoleAdmin, RoleSeniorEngineer)
}

func (a *Actor) IsAuditor() bool {
	return a.Role == string(RoleAuditor)
}

func ResolveDevUser(ctx context.Context, db *sql.DB, userID string) (*Actor, error) {
	if userID == "" {
		return nil, fmt.Errorf("no user specified")
	}
	var a Actor
	err := db.QueryRowContext(ctx,
		`SELECT u.id, u.email, u.display_name, m.organisation_id, m.role
		FROM users u
		JOIN memberships m ON m.user_id = u.id AND m.status = 'active'
		WHERE u.id = ? AND u.status = 'active'
		LIMIT 1`, userID,
	).Scan(&a.UserID, &a.Email, &a.DisplayName, &a.OrganisationID, &a.Role)
	if err != nil {
		// Fallback: try to match by email prefix
		err2 := db.QueryRowContext(ctx,
			`SELECT u.id, u.email, u.display_name, m.organisation_id, m.role
			FROM users u
			JOIN memberships m ON m.user_id = u.id AND m.status = 'active'
			WHERE u.email LIKE ? AND u.status = 'active'
			LIMIT 1`, userID+"%",
		).Scan(&a.UserID, &a.Email, &a.DisplayName, &a.OrganisationID, &a.Role)
		if err2 != nil {
			return nil, fmt.Errorf("user not found or inactive: %s", userID)
		}
	}
	return &a, nil
}

// IsPrivileged reports whether the role can perform administrative operations.
// Owner, Admin can do all admin ops. Senior can manage inventory.
func (a *Actor) IsPrivileged() bool {
	return a.HasRole(RoleOwner, RoleAdmin, RoleSeniorEngineer)
}

// CanManageServers reports whether the actor can add/modify servers.
func (a *Actor) CanManageServers() bool {
	return a.IsPrivileged()
}

// CanManageRunners reports whether the actor can register/manage runners.
func (a *Actor) CanManageRunners() bool {
	return a.IsPrivileged()
}

// CanExecuteRaw reports whether the actor can run arbitrary commands.
// Juniors cannot run raw commands; they need runbooks.
func (a *Actor) CanExecuteRaw() bool {
	return a.IsSenior()
}

type Env string

const (
	EnvDevelopment Env = "development"
	EnvStaging     Env = "staging"
	EnvProduction  Env = "production"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

func ClassifyRisk(command string) RiskLevel {
	cmd := strings.ToLower(strings.TrimSpace(command))
	dangerous := []string{"rm ", "dd ", "mkfs", "fdisk", "reboot", "shutdown", "kill ", "killall",
		"iptables", "nf_tables", "sysctl", "chmod 777", "> /dev/sda", "mkfs.", "format"}
	highRisk := []string{"systemctl restart", "service ", "docker rm", "docker stop",
		"kubectl delete", "pip uninstall", "npm uninstall -g", "apt-get purge", "yum remove"}
	mediumRisk := []string{"systemctl stop", "systemctl start", "systemctl reload",
		"docker restart", "docker run", "kubectl apply", "curl ", "wget ", "pip install",
		"npm install -g", "apt-get install", "yum install"}

	for _, p := range dangerous {
		if strings.Contains(cmd, p) {
			return RiskCritical
		}
	}
	for _, p := range highRisk {
		if strings.Contains(cmd, p) {
			return RiskHigh
		}
	}
	for _, p := range mediumRisk {
		if strings.Contains(cmd, p) {
			return RiskMedium
		}
	}
	return RiskLow
}
