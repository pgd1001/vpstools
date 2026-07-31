package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pgd1001/svrtools/packages/jobsign"
)

// A server's recorded SSH identity must reach the runner through the signed
// dispatch. If it did not, the runner would refuse every job, so this covers
// the whole path from registration through to the claim response.
func TestRegisteredSSHIdentityReachesDispatch(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	fingerprint := "SHA256:" + strings.Repeat("A", 43)
	body := `{"name":"identity-target","hostname":"identity.example.com","ssh_username":"operator",` +
		`"ssh_credential_ref":"identity-target","ssh_host_key_fingerprint":"` + fingerprint + `",` +
		`"environment":"development"}`
	w := doRequest(t, mux, http.MethodPost, "/api/v1/servers", body, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("register server failed: %d %s", w.Code, w.Body.String())
	}

	w = doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:identity-target","command":"uptime","reason":"ssh identity dispatch test"}`, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("queue execution failed: %d %s", w.Code, w.Body.String())
	}

	claim := claimWithToken(t, mux, "rnr_local", "test-runner-token")
	if claim.Code != http.StatusOK {
		t.Fatalf("claim failed: %d %s", claim.Code, claim.Body.String())
	}
	var dispatched struct {
		ExecutionID        string `json:"execution_id"`
		TargetID           string `json:"target_id"`
		LeaseID            string `json:"lease_id"`
		RunnerID           string `json:"runner_id"`
		Command            string `json:"command"`
		Host               string `json:"host"`
		Port               int    `json:"port"`
		User               string `json:"user"`
		CredentialRef      string `json:"credential_ref"`
		HostKeyFingerprint string `json:"host_key_fingerprint"`
		Timeout            int    `json:"timeout"`
		ExpiresAt          int64  `json:"expires_at_unix"`
		Signature          string `json:"signature"`
	}
	if err := json.Unmarshal(claim.Body.Bytes(), &dispatched); err != nil {
		t.Fatalf("decode dispatched job: %v", err)
	}
	if dispatched.CredentialRef != "identity-target" {
		t.Fatalf("credential reference did not reach the runner, got %q", dispatched.CredentialRef)
	}
	if dispatched.HostKeyFingerprint != fingerprint {
		t.Fatalf("host key fingerprint did not reach the runner, got %q", dispatched.HostKeyFingerprint)
	}

	claims := jobsign.Claims{
		ExecutionID: dispatched.ExecutionID, TargetID: dispatched.TargetID,
		LeaseID: dispatched.LeaseID, RunnerID: dispatched.RunnerID,
		Command: dispatched.Command, Host: dispatched.Host, Port: dispatched.Port,
		User: dispatched.User, Timeout: dispatched.Timeout,
		CredentialRef: dispatched.CredentialRef, HostKeyFingerprint: dispatched.HostKeyFingerprint,
		ExpiresAtUnix: dispatched.ExpiresAt,
	}
	if err := mustTestSigner(t).Verify(claims, dispatched.Signature, time.Now()); err != nil {
		t.Fatalf("dispatched job did not verify: %v", err)
	}

	// Stripping the pin must break the signature. Otherwise an attacker could
	// remove it in transit and have the runner connect to whatever answers on
	// the address, which is exactly what the pin exists to prevent.
	unpinned := claims
	unpinned.HostKeyFingerprint = ""
	if err := mustTestSigner(t).Verify(unpinned, dispatched.Signature, time.Now()); err == nil {
		t.Fatal("a job stripped of its host key pin verified against the issued signature")
	}

	// Substituting a different credential must likewise break the signature,
	// since it would otherwise let a job authenticate as another identity.
	swapped := claims
	swapped.CredentialRef = "some-other-server"
	if err := mustTestSigner(t).Verify(swapped, dispatched.Signature, time.Now()); err == nil {
		t.Fatal("a job with a substituted credential reference verified against the issued signature")
	}
}

// A fingerprint that cannot be compared is worse than an obvious error: it
// would look like the host was pinned when it was not. Malformed values are
// therefore refused at the API boundary.
func TestServerRegistrationRejectsMalformedSSHIdentity(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	cases := map[string]string{
		"unprefixed fingerprint": `{"name":"bad-1","ssh_host_key_fingerprint":"abcdef"}`,
		"truncated fingerprint":  `{"name":"bad-2","ssh_host_key_fingerprint":"SHA256:tooshort"}`,
		"md5 fingerprint":        `{"name":"bad-3","ssh_host_key_fingerprint":"MD5:aa:bb:cc"}`,
		"traversal in reference": `{"name":"bad-4","ssh_credential_ref":"../../etc/shadow"}`,
		"path in reference":      `{"name":"bad-5","ssh_credential_ref":"nested/key"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			w := doRequest(t, mux, http.MethodPost, "/api/v1/servers", body, "user_senior")
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d %s", w.Code, w.Body.String())
			}
		})
	}
}

// A well-formed identity must still be accepted, so the validation above is
// not simply refusing everything.
func TestServerRegistrationAcceptsWellFormedSSHIdentity(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	body := `{"name":"good-target","ssh_credential_ref":"good_target-01",` +
		`"ssh_host_key_fingerprint":"SHA256:` + strings.Repeat("B", 43) + `"}`
	w := doRequest(t, mux, http.MethodPost, "/api/v1/servers", body, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d %s", w.Code, w.Body.String())
	}
}
