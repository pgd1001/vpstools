package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pgd1001/svrtools/packages/authz"
)

// resolveExternalActor maps a verified OIDC subject to a pre-provisioned local
// user. Email is only a bootstrap fallback, and the subject is persisted so a
// later email change cannot silently change the account mapping.
func resolveExternalActor(ctx context.Context, db *sql.DB, subject, email string) (*authz.Actor, error) {
	subject = strings.TrimSpace(subject)
	email = strings.ToLower(strings.TrimSpace(email))
	if subject == "" || email == "" {
		return nil, fmt.Errorf("verified OIDC identity is incomplete")
	}
	var userID string
	err := apiQueryRow(ctx, db, `SELECT id FROM users WHERE external_subject = ? AND status = 'active'`, subject).Scan(&userID)
	if err != nil {
		if err := apiQueryRow(ctx, db, `SELECT id FROM users WHERE email = ? AND status = 'active' AND (external_subject IS NULL OR external_subject = '')`, email).Scan(&userID); err != nil {
			return nil, fmt.Errorf("OIDC user is not provisioned")
		}
		if _, err := apiExec(ctx, db, `UPDATE users SET external_subject = ?, external_provider = 'zitadel', last_login_at = `+apiCurrentTime()+`, updated_at = `+apiCurrentTime()+` WHERE id = ?`, subject, userID); err != nil {
			return nil, fmt.Errorf("failed to bind OIDC identity")
		}
	} else {
		_, _ = apiExec(ctx, db, `UPDATE users SET last_login_at = `+apiCurrentTime()+`, updated_at = `+apiCurrentTime()+` WHERE id = ?`, userID)
	}

	return authz.ResolveDevUser(ctx, db, userID)
}
