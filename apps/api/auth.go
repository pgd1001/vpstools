package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pgd1001/svrtools/packages/authz"
	"github.com/pgd1001/svrtools/packages/config"
)

// Request authentication and identity resolution.
//
// Every request enters the control plane through withAuth or one of the runner
// credential checks, so this file is the single place where an actor or a runner
// identity is established. Nothing downstream re-derives identity.

func withAuth(db *sql.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if bearer := bearerToken(r); bearer != "" {
			actor, err := resolveAPITokenActor(r.Context(), db, bearer)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired API token"})
				return
			}
			ctx := authz.WithActor(r.Context(), actor)
			ctx, cleanup, err := bindTenantConnection(ctx, db, actor.OrganisationID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
				return
			}
			defer cleanup()
			ctx = context.WithValue(ctx, dbKey, db)
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
			next(w, r.WithContext(ctx))
			return
		}
		sharedSecret := os.Getenv("VPS_WEB_SHARED_SECRET")
		providedSecret := r.Header.Get("X-VPS-Internal-Secret")
		if sharedSecret != "" && secureStringEqual(providedSecret, sharedSecret) {
			actor, err := resolveExternalActor(r.Context(), db, r.Header.Get("X-VPS-OIDC-Subject"), r.Header.Get("X-VPS-OIDC-Email"))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "OIDC identity is not provisioned"})
				return
			}
			ctx := authz.WithActor(r.Context(), actor)
			ctx, cleanup, err := bindTenantConnection(ctx, db, actor.OrganisationID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
				return
			}
			defer cleanup()
			ctx = context.WithValue(ctx, dbKey, db)
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
			next(w, r.WithContext(ctx))
			return
		}
		if !devAuthEnabled() {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		userID := r.Header.Get("X-VPS-User")
		// Never infer an identity. A request with no user header is
		// unauthenticated, even in development; defaulting to a senior
		// engineer would mean the deny-by-default rule has an exception that
		// grants the most privileged role.
		if strings.TrimSpace(userID) == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		actor, err := authz.ResolveDevUser(r.Context(), db, userID)
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "authentication failed: " + err.Error()})
			return
		}
		ctx := authz.WithActor(r.Context(), actor)
		ctx, cleanup, err := bindTenantConnection(ctx, db, actor.OrganisationID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
			return
		}
		defer cleanup()
		ctx = context.WithValue(ctx, dbKey, db)
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		next(w, r.WithContext(ctx))
	}
}

// devAuthEnabled reports whether the header-based development identity is
// available. It requires both an explicit opt-in and an explicitly
// non-production environment, so neither a leaked flag nor a forgotten
// environment variable is sufficient on its own.
func devAuthEnabled() bool {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("VPS_DEV_AUTH")), "true") {
		return false
	}
	return !productionModeEnabled()
}

// productionModeEnabled treats every environment that is not explicitly
// non-production as production. See config.ProductionMode for the reasoning.
func productionModeEnabled() bool {
	return config.ProductionMode()
}

func secureStringEqual(got, want string) bool {
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < len("Bearer ") || !strings.EqualFold(value[:len("Bearer ")], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[len("Bearer "):])
}

func resolveAPITokenActor(ctx context.Context, db *sql.DB, token string) (*authz.Actor, error) {
	if token == "" {
		return nil, fmt.Errorf("API token is empty")
	}
	var actor authz.Actor
	err := apiQueryRow(ctx, db, `
		SELECT u.id, u.email, u.display_name, t.organisation_id, m.role
		FROM api_tokens t
		JOIN users u ON u.id = t.user_id AND u.status = 'active'
		JOIN memberships m ON m.user_id = t.user_id AND m.organisation_id = t.organisation_id AND m.status = 'active'
		WHERE t.token_hash = ? AND t.revoked_at IS NULL AND t.expires_at > `+apiCurrentTime(), hashToken(token)).Scan(
		&actor.UserID, &actor.Email, &actor.DisplayName, &actor.OrganisationID, &actor.Role)
	if err != nil {
		return nil, fmt.Errorf("API token is invalid or expired")
	}
	_, _ = apiExec(ctx, db, "UPDATE api_tokens SET last_used_at = "+apiCurrentTime()+" WHERE token_hash = ? AND revoked_at IS NULL", hashToken(token))
	return &actor, nil
}

func validateAuthConfig() error {
	if !productionModeEnabled() {
		return nil
	}
	if devAuthEnabled() {
		return fmt.Errorf("VPS_DEV_AUTH must be disabled in production")
	}
	secret := os.Getenv("VPS_WEB_SHARED_SECRET")
	if len(secret) < 32 {
		return fmt.Errorf("VPS_WEB_SHARED_SECRET must be at least 32 characters in production")
	}
	return nil
}

func authenticateRunnerRegistration(db *sql.DB, r *http.Request) (string, error) {
	orgID, _, err := authenticateRunnerCredential(db, r)
	return orgID, err
}

func authenticateRunnerCredential(db *sql.DB, r *http.Request) (string, string, error) {
	token := runnerToken(r)
	// There is no tokenless path. A runner credential is the only thing that
	// establishes which organisation's servers a runner may touch, so an
	// anonymous request cannot be resolved to a tenant under any configuration.
	if token == "" {
		return "", "", fmt.Errorf("runner registration credential required")
	}
	var orgID, runnerID string
	err := apiQueryRow(r.Context(), db, `SELECT organisation_id, COALESCE(runner_id,'') FROM runner_credentials WHERE token_hash = ? AND revoked_at IS NULL AND expires_at > `+apiCurrentTime(), hashToken(token)).Scan(&orgID, &runnerID)
	if err != nil {
		return "", "", fmt.Errorf("invalid or expired runner registration credential")
	}
	if runnerID != "" {
		statusRequest := r
		statusContext, cleanup, bindErr := bindTenantConnection(r.Context(), db, orgID)
		if bindErr != nil {
			return "", "", bindErr
		}
		defer cleanup()
		statusRequest = r.WithContext(statusContext)
		var status string
		if err := apiQueryRow(statusRequest.Context(), db, "SELECT status FROM runners WHERE id = ? AND organisation_id = ?", runnerID, orgID).Scan(&status); err != nil || status == "revoked" {
			return "", "", fmt.Errorf("runner credential is no longer valid")
		}
	}
	return orgID, runnerID, nil
}

// authenticateBoundRunner authorises a request that acts as a specific runner:
// claiming work, renewing a lease, submitting a result, or heartbeating.
//
// These endpoints require a credential bound to that runner. An organisation
// wide registration credential is only good for the initial registration
// handshake; if it were accepted here, any holder could claim work scoped to
// any other runner in the organisation and runner_scopes would enforce
// nothing.
func authenticateBoundRunner(db *sql.DB, r *http.Request, requestedRunnerID string) (string, error) {
	orgID, credentialRunnerID, err := authenticateRunnerCredential(db, r)
	if err != nil {
		return "", err
	}
	if credentialRunnerID == "" {
		return "", fmt.Errorf("this endpoint requires a runner-bound credential; register the runner first")
	}
	if requestedRunnerID == "" {
		return "", fmt.Errorf("runner_id is required")
	}
	if credentialRunnerID != requestedRunnerID {
		return "", fmt.Errorf("runner credential is bound to a different runner")
	}
	return orgID, nil
}

func runnerToken(r *http.Request) string {
	return r.Header.Get("X-VPS-Runner-Token")
}

func handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	actor, err := authz.RequireActor(r.Context())
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": err.Error()})
		return
	}
	var orgName string
	apiQueryRow(r.Context(), dbFrom(r), "SELECT name FROM organisations WHERE id = ?", actor.OrganisationID).Scan(&orgName)
	writeJSON(w, 200, map[string]any{
		"user_id":         actor.UserID,
		"email":           actor.Email,
		"name":            actor.DisplayName,
		"organisation_id": actor.OrganisationID,
		"organisation":    orgName,
		"role":            actor.Role,
	})
}

type createAPITokenRequest struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	ExpiresIn int    `json:"expires_in"`
}

func handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	actor, err := authz.RequireActor(r.Context())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if !actor.IsPrivileged() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "API token management requires a privileged role"})
		return
	}
	var req createAPITokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		req.UserID = actor.UserID
	}
	if req.Name == "" {
		req.Name = "cli-token"
	}
	if len(req.Name) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name must be 100 characters or fewer"})
		return
	}
	if req.ExpiresIn == 0 {
		req.ExpiresIn = 30 * 24 * 60 * 60
	}
	if req.ExpiresIn < 60 || req.ExpiresIn > 90*24*60*60 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expires_in must be between 60 seconds and 90 days"})
		return
	}

	var provisionedUser string
	err = apiQueryRow(r.Context(), dbFrom(r), `
		SELECT u.id FROM users u
		JOIN memberships m ON m.user_id = u.id AND m.organisation_id = ? AND m.status = 'active'
		WHERE u.id = ? AND u.status = 'active'`, actor.OrganisationID, req.UserID).Scan(&provisionedUser)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user is not an active member of your organisation"})
		return
	}

	token := newToken()
	if token == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate API token"})
		return
	}
	expiresAt := time.Now().UTC().Add(time.Duration(req.ExpiresIn) * time.Second).Format("2006-01-02 15:04:05")
	tokenID := "pat_" + shortID()
	tokenPrefix := token
	if len(tokenPrefix) > 8 {
		tokenPrefix = tokenPrefix[:8]
	}
	_, err = apiExec(r.Context(), dbFrom(r), `
		INSERT INTO api_tokens (id, organisation_id, user_id, name, token_prefix, token_hash, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, tokenID, actor.OrganisationID, provisionedUser, req.Name, tokenPrefix, hashToken(token), expiresAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create API token"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "auth.token.created", "api_token", tokenID, "success", map[string]any{
		"user_id": provisionedUser, "name": req.Name, "token_prefix": tokenPrefix, "expires_at": expiresAt,
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"token_id": tokenID, "token": token, "user_id": provisionedUser,
		"organisation_id": actor.OrganisationID, "expires_at": expiresAt,
	})
}

func handleRevokeAPIToken(w http.ResponseWriter, r *http.Request, tokenID string) {
	actor, err := authz.RequireActor(r.Context())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if !actor.IsPrivileged() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "API token management requires a privileged role"})
		return
	}
	if tokenID == "" || strings.Contains(tokenID, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid token id"})
		return
	}
	result, err := apiExec(r.Context(), dbFrom(r), `
		UPDATE api_tokens SET revoked_at = `+apiCurrentTime()+`
		WHERE id = ? AND organisation_id = ? AND revoked_at IS NULL`, tokenID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke API token"})
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active API token not found"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "auth.token.revoked", "api_token", tokenID, "success", nil)
	writeJSON(w, http.StatusOK, map[string]string{"token_id": tokenID, "status": "revoked"})
}
