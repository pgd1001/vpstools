package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pgd1001/svrtools/packages/jobsign"
)

// queueJobForClaim creates an execution so there is work available to claim.
func queueJobForClaim(t *testing.T, mux *http.ServeMux) {
	t.Helper()
	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:demo","command":"uptime","reason":"trust boundary test"}`, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("queue execution failed: %d %s", w.Code, w.Body.String())
	}
}

func claimWithToken(t *testing.T, mux *http.ServeMux, runnerID, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?runner_id="+runnerID, nil)
	req.Header.Set("X-VPS-Runner-Token", token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// The control plane is the only component allowed to decide what a runner
// executes, so every dispatched job must carry a signature that authenticates
// the command and the host it targets.
func TestClaimedJobIsSignedAndVerifies(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()
	queueJobForClaim(t, mux)

	w := claimWithToken(t, mux, "rnr_local", "test-runner-token")
	if w.Code != http.StatusOK {
		t.Fatalf("claim failed: %d %s", w.Code, w.Body.String())
	}
	var dispatched struct {
		ExecutionID string `json:"execution_id"`
		TargetID    string `json:"target_id"`
		LeaseID     string `json:"lease_id"`
		RunnerID    string `json:"runner_id"`
		Command     string `json:"command"`
		Host        string `json:"host"`
		Port        int    `json:"port"`
		User        string `json:"user"`
		Timeout     int    `json:"timeout"`
		ExpiresAt   int64  `json:"expires_at_unix"`
		Signature   string `json:"signature"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &dispatched); err != nil {
		t.Fatalf("decode dispatched job: %v", err)
	}
	if dispatched.Signature == "" {
		t.Fatal("dispatched job carried no signature")
	}
	claims := jobsign.Claims{
		ExecutionID: dispatched.ExecutionID, TargetID: dispatched.TargetID,
		LeaseID: dispatched.LeaseID, RunnerID: dispatched.RunnerID,
		Command: dispatched.Command, Host: dispatched.Host, Port: dispatched.Port,
		User: dispatched.User, Timeout: dispatched.Timeout, ExpiresAtUnix: dispatched.ExpiresAt,
	}
	if err := mustTestSigner(t).Verify(claims, dispatched.Signature, time.Now()); err != nil {
		t.Fatalf("dispatched job did not verify: %v", err)
	}

	// The signature must actually bind the command: a substituted command must
	// not verify against the issued signature.
	tampered := claims
	tampered.Command = "rm -rf /"
	if err := mustTestSigner(t).Verify(tampered, dispatched.Signature, time.Now()); err == nil {
		t.Fatal("a substituted command verified against the issued signature")
	}
}

// An organisation-wide bootstrap credential must not be usable to claim work.
// If it were, any holder could take jobs scoped to any runner in the org and
// runner_scopes would enforce nothing.
func TestUnboundCredentialCannotClaimWork(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()
	queueJobForClaim(t, mux)

	if _, err := db.Exec(`INSERT INTO runner_credentials (id, organisation_id, token_hash, expires_at)
		VALUES ('rct_unbound','org_demo',?,datetime('now','+1 hour'))`, hashToken("unbound-token")); err != nil {
		t.Fatalf("insert unbound credential: %v", err)
	}

	w := claimWithToken(t, mux, "rnr_local", "unbound-token")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unbound credential claim = %d %s, want 401", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "runner-bound") {
		t.Fatalf("unexpected rejection reason: %s", w.Body.String())
	}
}

// A credential bound to one runner must not be usable to act as another, even
// inside the same organisation.
func TestBoundCredentialCannotImpersonateAnotherRunner(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()
	queueJobForClaim(t, mux)

	if _, err := db.Exec(`INSERT INTO runners (id, organisation_id, name, runner_type, status)
		VALUES ('rnr_other','org_demo','other-runner','customer_managed','active')`); err != nil {
		t.Fatalf("insert second runner: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO runner_scopes (id, organisation_id, runner_id, scope_type, scope_value)
		VALUES ('rsc_other','org_demo','rnr_other','all','*')`); err != nil {
		t.Fatalf("insert second runner scope: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO runner_credentials (id, organisation_id, runner_id, token_hash, expires_at)
		VALUES ('rct_other','org_demo','rnr_other',?,datetime('now','+1 hour'))`, hashToken("other-runner-token")); err != nil {
		t.Fatalf("insert second runner credential: %v", err)
	}

	// rnr_other's credential must not be able to claim as rnr_local.
	w := claimWithToken(t, mux, "rnr_local", "other-runner-token")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("cross-runner claim = %d %s, want 401", w.Code, w.Body.String())
	}

	// Its own identity still works, confirming the check is about binding and
	// not a blanket denial.
	if w := claimWithToken(t, mux, "rnr_other", "other-runner-token"); w.Code != http.StatusOK {
		t.Fatalf("bound runner claiming its own work = %d %s, want 200", w.Code, w.Body.String())
	}
}

// Submitting a result while naming a different runner must be rejected, so a
// runner cannot close out work that was leased to someone else.
func TestResultSubmissionIsBoundToTheClaimingRunner(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()
	queueJobForClaim(t, mux)

	w := claimWithToken(t, mux, "rnr_local", "test-runner-token")
	if w.Code != http.StatusOK {
		t.Fatalf("claim failed: %d %s", w.Code, w.Body.String())
	}
	var dispatched struct {
		ExecutionID string `json:"execution_id"`
		TargetID    string `json:"target_id"`
		LeaseID     string `json:"lease_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &dispatched); err != nil {
		t.Fatalf("decode dispatched job: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO runners (id, organisation_id, name, runner_type, status)
		VALUES ('rnr_thief','org_demo','thief','customer_managed','active')`); err != nil {
		t.Fatalf("insert thief runner: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO runner_credentials (id, organisation_id, runner_id, token_hash, expires_at)
		VALUES ('rct_thief','org_demo','rnr_thief',?,datetime('now','+1 hour'))`, hashToken("thief-token")); err != nil {
		t.Fatalf("insert thief credential: %v", err)
	}

	body := `{"execution_id":"` + dispatched.ExecutionID + `","target_id":"` + dispatched.TargetID +
		`","runner_id":"rnr_local","lease_id":"` + dispatched.LeaseID + `","exit_code":0,"stdout":"pwned"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/result", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-VPS-Runner-Token", "thief-token")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("result submitted under another runner's identity = %d %s, want 401", recorder.Code, recorder.Body.String())
	}
}

// No credential at all must never resolve to a tenant, in any configuration.
// The previous behaviour silently mapped anonymous runner calls to org_demo.
func TestAnonymousRunnerCallIsRejectedEvenWithDevAuth(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()
	queueJobForClaim(t, mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?runner_id=rnr_local", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous claim = %d %s, want 401", w.Code, w.Body.String())
	}
	if _, _, err := authenticateRunnerCredential(db, req); err == nil {
		t.Fatal("an anonymous runner request resolved to an organisation")
	}
}

// Development auth must never invent an identity. A request with no user
// header is unauthenticated even when the dev bypass is enabled.
func TestDevAuthDoesNotDefaultToAPrivilegedUser(t *testing.T) {
	db, _, cleanup := testAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/whoami", nil)
	w := httptest.NewRecorder()
	withAuth(db, handleWhoAmI)(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("headerless dev request = %d %s, want 401", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "user_senior") {
		t.Fatalf("headerless request resolved to a privileged user: %s", w.Body.String())
	}
}

// An environment that is not explicitly non-production must disable the dev
// bypass, so 'staging' or a typo cannot leave it enabled.
func TestDevAuthIsDisabledOutsideExplicitNonProductionEnvironments(t *testing.T) {
	db, _, cleanup := testAPI(t)
	defer cleanup()
	t.Setenv("VPS_DEV_AUTH", "true")

	for _, environment := range []string{"staging", "prod-eu", "typo", ""} {
		t.Setenv("VPS_ENV", environment)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/whoami", nil)
		req.Header.Set("X-VPS-User", "user_senior")
		w := httptest.NewRecorder()
		withAuth(db, handleWhoAmI)(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("VPS_ENV=%q allowed the dev bypass: %d %s", environment, w.Code, w.Body.String())
		}
	}
}

// Registration must hand back an identity-bound credential, otherwise a runner
// has no way to satisfy the bound-credential requirement on the job endpoints.
func TestRegistrationIssuesABoundCredential(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runners",
		strings.NewReader(`{"name":"fresh-runner","platform":"linux","version":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-VPS-Runner-Token", "test-runner-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("registration = %d %s, want 201", w.Code, w.Body.String())
	}
	var registration struct {
		RunnerID    string `json:"runner_id"`
		RunnerToken string `json:"runner_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &registration); err != nil {
		t.Fatalf("decode registration: %v", err)
	}
	if registration.RunnerToken == "" {
		t.Fatal("registration did not return a runner-bound credential")
	}

	var boundRunner string
	if err := db.QueryRow(`SELECT COALESCE(runner_id,'') FROM runner_credentials WHERE token_hash = ?`,
		hashToken(registration.RunnerToken)).Scan(&boundRunner); err != nil {
		t.Fatalf("lookup issued credential: %v", err)
	}
	if boundRunner != registration.RunnerID {
		t.Fatalf("issued credential bound to %q, want %q", boundRunner, registration.RunnerID)
	}

	queueJobForClaim(t, mux)
	if w := claimWithToken(t, mux, registration.RunnerID, registration.RunnerToken); w.Code != http.StatusOK {
		t.Fatalf("newly registered runner could not claim work: %d %s", w.Code, w.Body.String())
	}
}
