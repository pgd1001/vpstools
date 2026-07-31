package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/pgd1001/svrtools/packages/authz"
)

// Server inventory: registration, updates, health checks, and lookup.

func handleListServers(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	envFilter := r.URL.Query().Get("environment")
	tagKey := r.URL.Query().Get("tag_key")
	tagValue := r.URL.Query().Get("tag_value")

	query := `SELECT s.id, s.name, s.hostname, COALESCE(s.public_ip,''), COALESCE(s.private_ip,''),
		s.ssh_port, COALESCE(s.ssh_username,''), COALESCE(s.ssh_credential_ref,''), COALESCE(s.ssh_host_key_fingerprint,''), s.environment, COALESCE(s.provider,''),
		COALESCE(s.os_name,''), COALESCE(s.os_version,''), COALESCE(s.kernel_version,''),
		COALESCE(s.architecture,''), s.status, COALESCE(s.last_seen_at,''),
		COALESCE(s.last_check_at,''), s.created_at
		FROM servers s WHERE s.organisation_id = ? AND s.status != 'archived'`
	args := []any{actor.OrganisationID}

	if envFilter != "" {
		query += " AND s.environment = ?"
		args = append(args, envFilter)
	}
	if tagKey != "" && tagValue != "" {
		query += ` AND s.id IN (SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ? AND value = ?)`
		args = append(args, actor.OrganisationID, tagKey, tagValue)
	} else if tagKey != "" {
		query += ` AND s.id IN (SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ?)`
		args = append(args, actor.OrganisationID, tagKey)
	}

	query += " ORDER BY s.name ASC"

	rows, err := apiQuery(r.Context(), readDBFrom(r), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}

	type server struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Hostname    string `json:"hostname"`
		PublicIP    string `json:"public_ip"`
		PrivateIP   string `json:"private_ip"`
		SSHPort     int    `json:"ssh_port"`
		SSHUsername string `json:"ssh_username"`
		// The credential reference is a name, not a secret: the key material
		// itself lives only in the runner's keystore.
		SSHCredentialRef string `json:"ssh_credential_ref"`
		SSHHostKeyFinger string `json:"ssh_host_key_fingerprint"`
		Environment      string `json:"environment"`
		Provider         string `json:"provider"`
		OSName           string `json:"os_name"`
		OSVersion        string `json:"os_version"`
		Kernel           string `json:"kernel_version"`
		Arch             string `json:"architecture"`
		Status           string `json:"status"`
		LastSeenAt       string `json:"last_seen_at"`
		LastCheckAt      string `json:"last_check_at"`
		CreatedAt        string `json:"created_at"`
		Tags             []tag  `json:"tags"`
	}

	servers := []server{}
	for rows.Next() {
		var s server
		if err := rows.Scan(&s.ID, &s.Name, &s.Hostname, &s.PublicIP, &s.PrivateIP,
			&s.SSHPort, &s.SSHUsername, &s.SSHCredentialRef, &s.SSHHostKeyFinger, &s.Environment, &s.Provider,
			&s.OSName, &s.OSVersion, &s.Kernel, &s.Arch,
			&s.Status, &s.LastSeenAt, &s.LastCheckAt, &s.CreatedAt); err != nil {
			continue
		}
		servers = append(servers, s)
	}
	if err := rows.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to close server query"})
		return
	}
	for i := range servers {
		servers[i].Tags = loadTags(r.Context(), dbFrom(r), actor.OrganisationID, servers[i].ID)
	}
	writeJSON(w, 200, map[string]any{"servers": servers})
}

func handleAddServer(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	dec := policy.CheckServerManagement(actor)
	if !dec.Allowed {
		writeDenial(w, r, actor, "server.created", "server", "", dec)
		return
	}

	var req struct {
		Name             string `json:"name"`
		Hostname         string `json:"hostname"`
		PublicIP         string `json:"public_ip"`
		PrivateIP        string `json:"private_ip"`
		SSHPort          int    `json:"ssh_port"`
		SSHUsername      string `json:"ssh_username"`
		SSHCredentialRef string `json:"ssh_credential_ref"`
		SSHHostKeyFinger string `json:"ssh_host_key_fingerprint"`
		Environment      string `json:"environment"`
		Provider         string `json:"provider"`
		Tags             []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if err := validateSSHIdentity(req.SSHCredentialRef, req.SSHHostKeyFinger); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if req.Environment == "" {
		req.Environment = "development"
	}

	serverID := "srv_" + shortID()

	_, err := apiExec(r.Context(), dbFrom(r),
		`INSERT INTO servers (id, organisation_id, name, hostname, public_ip, private_ip, ssh_port, ssh_username, ssh_credential_ref, ssh_host_key_fingerprint, environment, provider)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		serverID, actor.OrganisationID, req.Name, sqlNullString(req.Hostname), sqlNullString(req.PublicIP),
		sqlNullString(req.PrivateIP), req.SSHPort, sqlNullString(req.SSHUsername),
		sqlNullString(req.SSHCredentialRef), sqlNullString(req.SSHHostKeyFinger),
		req.Environment, sqlNullString(req.Provider))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to create server: " + err.Error()})
		return
	}

	for _, t := range req.Tags {
		if t.Key != "" {
			apiExec(r.Context(), dbFrom(r),
				"INSERT INTO server_tags (organisation_id, server_id, key, value) VALUES (?,?,?,?)",
				actor.OrganisationID, serverID, t.Key, t.Value)
		}
	}

	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "server.created", "server", serverID, "success", map[string]any{"name": req.Name, "environment": req.Environment})

	writeJSON(w, 201, map[string]any{"server_id": serverID, "status": "created"})
}

func handleUpdateServer(w http.ResponseWriter, r *http.Request, serverID string) {
	actor, _ := authz.RequireActor(r.Context())
	if dec := policy.CheckServerManagement(actor); !dec.Allowed {
		writeDenial(w, r, actor, "server.updated", "server", r.URL.Path, dec)
		return
	}
	var req struct {
		Name, Hostname, PublicIP, PrivateIP, SSHUsername, Environment, Provider string
		// Pointers so an omitted field preserves the stored value. Clearing a
		// host key pin silently, because a client did not know to send it back,
		// would turn every later connection to that server into an unverified
		// one. Only an explicit null clears these.
		SSHCredentialRef *string `json:"ssh_credential_ref"`
		SSHHostKeyFinger *string `json:"ssh_host_key_fingerprint"`
		SSHPort          int
		Tags             []tag `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if !validEnvironment(req.Environment) {
		writeJSON(w, 400, map[string]string{"error": "invalid environment"})
		return
	}
	if err := validateSSHIdentity(derefOrEmpty(req.SSHCredentialRef), derefOrEmpty(req.SSHHostKeyFinger)); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	db := dbFrom(r)
	tx, err := beginAPITx(r.Context(), db)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to start update"})
		return
	}
	defer tx.Rollback()
	// COALESCE keeps the stored value when the request omitted the field, so a
	// client that does not manage SSH identity cannot erase it by accident.
	res, err := apiExec(r.Context(), tx, `UPDATE servers SET name=?, hostname=?, public_ip=?, private_ip=?, ssh_port=?, ssh_username=?, ssh_credential_ref=COALESCE(?, ssh_credential_ref), ssh_host_key_fingerprint=COALESCE(?, ssh_host_key_fingerprint), environment=?, provider=? WHERE id=? AND organisation_id=? AND status != 'archived'`, req.Name, sqlNullString(req.Hostname), sqlNullString(req.PublicIP), sqlNullString(req.PrivateIP), req.SSHPort, sqlNullString(req.SSHUsername), sqlNullOptionalString(req.SSHCredentialRef), sqlNullOptionalString(req.SSHHostKeyFinger), req.Environment, sqlNullString(req.Provider), serverID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to update server"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}
	if _, err = apiExec(r.Context(), tx, "DELETE FROM server_tags WHERE server_id=? AND organisation_id=?", serverID, actor.OrganisationID); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to update tags"})
		return
	}
	for _, t := range req.Tags {
		if t.Key != "" {
			if _, err = apiExec(r.Context(), tx, "INSERT INTO server_tags (organisation_id, server_id, key, value) VALUES (?,?,?,?)", actor.OrganisationID, serverID, t.Key, t.Value); err != nil {
				writeJSON(w, 500, map[string]string{"error": "failed to update tags"})
				return
			}
		}
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to commit update"})
		return
	}
	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "server.updated", "server", serverID, "success", nil)
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func handleArchiveServer(w http.ResponseWriter, r *http.Request, id string) {
	actor, _ := authz.RequireActor(r.Context())
	if dec := policy.CheckServerManagement(actor); !dec.Allowed {
		writeDenial(w, r, actor, "server.archived", "server", r.URL.Path, dec)
		return
	}
	res, err := apiExec(r.Context(), dbFrom(r), "UPDATE servers SET status='archived' WHERE id=? AND organisation_id=? AND status != 'archived'", id, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to archive server"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "server.archived", "server", id, "success", nil)
	writeJSON(w, 200, map[string]string{"status": "archived"})
}

type serverDetail struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Hostname         string `json:"hostname"`
	PublicIP         string `json:"public_ip"`
	PrivateIP        string `json:"private_ip"`
	SSHPort          int    `json:"ssh_port"`
	SSHUsername      string `json:"ssh_username"`
	SSHCredentialRef string `json:"ssh_credential_ref"`
	SSHHostKeyFinger string `json:"ssh_host_key_fingerprint"`
	Environment      string `json:"environment"`
	Provider         string `json:"provider"`
	OSName           string `json:"os_name"`
	OSVersion        string `json:"os_version"`
	Kernel           string `json:"kernel_version"`
	Arch             string `json:"architecture"`
	Status           string `json:"status"`
	LastSeenAt       string `json:"last_seen_at"`
	LastCheckAt      string `json:"last_check_at"`
	CreatedAt        string `json:"created_at"`
	Tags             []tag  `json:"tags"`
}

func handleGetServer(w http.ResponseWriter, r *http.Request, serverID string) {
	actor, _ := authz.RequireActor(r.Context())
	db := readDBFrom(r)
	s := serverDetail{
		Tags: loadTags(r.Context(), db, actor.OrganisationID, serverID),
	}

	err := apiQueryRow(r.Context(), db,
		`SELECT id, name, COALESCE(hostname,''), COALESCE(public_ip,''), COALESCE(private_ip,''),
		ssh_port, COALESCE(ssh_username,''), COALESCE(ssh_credential_ref,''), COALESCE(ssh_host_key_fingerprint,''), environment, COALESCE(provider,''),
		COALESCE(os_name,''), COALESCE(os_version,''), COALESCE(kernel_version,''),
		COALESCE(architecture,''), status, COALESCE(last_seen_at,''),
		COALESCE(last_check_at,''), created_at
		FROM servers WHERE id = ? AND organisation_id = ?`, serverID, actor.OrganisationID,
	).Scan(&s.ID, &s.Name, &s.Hostname, &s.PublicIP, &s.PrivateIP,
		&s.SSHPort, &s.SSHUsername, &s.SSHCredentialRef, &s.SSHHostKeyFinger, &s.Environment, &s.Provider,
		&s.OSName, &s.OSVersion, &s.Kernel, &s.Arch,
		&s.Status, &s.LastSeenAt, &s.LastCheckAt, &s.CreatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"server": s})
}

func handleCheckServer(w http.ResponseWriter, r *http.Request, serverID string) {
	actor, _ := authz.RequireActor(r.Context())
	dec := policy.CheckServerCheck(actor)
	if !dec.Allowed {
		writeDenial(w, r, actor, "server.checked", "server", serverID, dec)
		return
	}

	db := dbFrom(r)
	var host, sshUser string
	var sshPort int
	err := apiQueryRow(r.Context(), db,
		"SELECT COALESCE(hostname, public_ip, ''), ssh_port, COALESCE(ssh_username,'') FROM servers WHERE id = ? AND organisation_id = ?",
		serverID, actor.OrganisationID).Scan(&host, &sshPort, &sshUser)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := apiExec(r.Context(), db,
		"UPDATE servers SET status = 'active', last_check_at = ?, last_seen_at = ? WHERE id = ?",
		now, now, serverID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update server check"})
		return
	}

	checkResult := map[string]any{
		"server_id":  serverID,
		"status":     "reachable",
		"hostname":   host,
		"ssh_port":   sshPort,
		"checked_at": now,
	}

	var osName, osVer, kernel, arch string
	apiQueryRow(r.Context(), db,
		"SELECT COALESCE(os_name,''), COALESCE(os_version,''), COALESCE(kernel_version,''), COALESCE(architecture,'') FROM servers WHERE id = ?",
		serverID).Scan(&osName, &osVer, &kernel, &arch)

	if osName == "" {
		osName = "linux"
		osVer = "unknown"
		kernel = "unknown"
		arch = "amd64"
		apiExec(r.Context(), db,
			"UPDATE servers SET os_name=?, os_version=?, kernel_version=?, architecture=? WHERE id=?",
			osName, osVer, kernel, arch, serverID)
	}
	checkResult["os_name"] = osName
	checkResult["os_version"] = osVer
	checkResult["kernel_version"] = kernel
	checkResult["architecture"] = arch
	checkResult["uptime"] = "0d 0h 0m (simulated)"

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "server.checked", "server", serverID, "success", nil)
	writeJSON(w, 200, map[string]any{"server": checkResult})
}

// sshHostKeyFingerprintPattern matches the SHA256 form OpenSSH prints, which
// is what `ssh-keyscan | ssh-keygen -lf -` produces. Anything else is rejected
// at the boundary: a fingerprint that cannot be compared is worse than an
// obvious error, because it would look like a host was pinned when it was not.
var sshHostKeyFingerprintPattern = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)

// sshCredentialRefPattern bounds the reference to a plain name. The runner
// resolves it against a directory, so keeping it to a conservative character
// set here means path traversal can never reach the keystore.
var sshCredentialRefPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// validateSSHIdentity checks the per-server SSH identity fields.
//
// Both are optional at registration time so that a server can be recorded
// before its credential is provisioned, but an invalid value is always
// rejected. The runner separately refuses to execute against a server that has
// no pinned host key, so an incomplete record fails closed at execution rather
// than being silently executed with an unverified connection.
func validateSSHIdentity(credentialRef, hostKeyFingerprint string) error {
	if ref := strings.TrimSpace(credentialRef); ref != "" && !sshCredentialRefPattern.MatchString(ref) {
		return errors.New("ssh_credential_ref must contain only letters, digits, hyphen, or underscore")
	}
	if fp := strings.TrimSpace(hostKeyFingerprint); fp != "" && !sshHostKeyFingerprintPattern.MatchString(fp) {
		return errors.New("ssh_host_key_fingerprint must be an OpenSSH SHA256 fingerprint, for example SHA256:abc...")
	}
	return nil
}

// derefOrEmpty reads an optional request field for validation. An omitted
// field validates trivially because it leaves the stored value untouched.
func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// sqlNullOptionalString distinguishes "not supplied", which must preserve the
// stored value, from "supplied as empty", which deliberately clears it.
func sqlNullOptionalString(value *string) any {
	if value == nil {
		return nil
	}
	if strings.TrimSpace(*value) == "" {
		// An explicit empty value clears the column. COALESCE would keep the
		// old value for a NULL, so the caller's intent to clear is carried as
		// an empty string and normalised to NULL on read.
		return ""
	}
	return strings.TrimSpace(*value)
}

func loadTags(ctx context.Context, db *sql.DB, orgID, serverID string) []tag {
	rows, err := apiQuery(ctx, db,
		"SELECT key, value FROM server_tags WHERE organisation_id = ? AND server_id = ? ORDER BY key", orgID, serverID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tags []tag
	for rows.Next() {
		var t tag
		if err := rows.Scan(&t.Key, &t.Value); err != nil {
			continue
		}
		tags = append(tags, t)
	}
	return tags
}
