# Product Requirements Document: VPS Tools

**Working title:** VPS Tools  
**Document status:** Draft v1  
**Date:** 18 May 2026  
**Primary audience:** Product, engineering, security, DevOps leadership  
**Target customers:** DevOps managers, senior DevOps engineers, platform engineers, MSPs, and infrastructure teams managing fleets of VPS servers

---

## 1. Executive Summary

VPS Tools is a secure, CLI-first operations suite for managing fleets of VPS servers with speed, consistency, delegation, and auditability. It is aimed at DevOps managers and senior DevOps engineers who need to manage many Linux servers across providers, environments, customers, and teams, while safely delegating controlled access to junior engineers.

The product should reduce the operational overhead of common VPS management tasks such as server inventory, SSH access, patching, firewall checks, service restarts, log collection, package updates, backup verification, user management, incident response, and compliance reporting.

The core promise is:

> Manage a VPS fleet from the command line with enterprise-grade access control, safe delegation, and a complete audit trail.

The product should sit between raw SSH scripts, Ansible, commercial privileged access tools, and full runbook automation platforms. Its advantage is speed, a focused VPS operations model, CLI-native workflows, guardrails for junior engineers, and built-in operational auditability.

VPS Tools should be available as both a hosted SaaS product and a self-hosted product. The self-hosted product should have two editions: an open-source base edition with unlimited seats and a supported commercial edition with full functionality. The SaaS product should use simple per-seat pricing for managers and senior engineers, with unlimited junior engineers included.

---

## 2. Problem Statement

Teams managing VPS fleets often rely on a messy mix of SSH, shared scripts, Ansible playbooks, spreadsheets, provider dashboards, password managers, internal docs, and manual Slack approvals. This creates avoidable risk and slows down routine work.

Common pain points include:

- No single view of all VPS servers across providers, customers, environments, and projects.
- Inconsistent access control across SSH keys, sudo permissions, provider accounts, and local server users.
- Overpowered junior access because delegation is hard to model safely.
- No reliable audit trail for who ran what, where, when, and why.
- Manual server checks are slow, inconsistent, and error-prone.
- Senior engineers are interrupted for routine tasks that could safely be delegated.
- Emergency access is poorly controlled or impossible to review afterwards.
- Runbooks exist in documentation but are not consistently enforced at execution time.
- Scripts work for their original author but are not safe or transparent enough for wider team use.
- Compliance and incident reviews require manual reconstruction from shell history, logs, chat, and memory.

VPS Tools should solve this by providing a controlled command execution and VPS operations layer that gives teams fast CLI workflows without giving up governance.

---

## 3. Product Vision

VPS Tools should become the default operational control layer for small to mid-sized infrastructure teams managing VPS fleets.

The product should allow a senior engineer to define safe operations, access policies, environments, approval rules, command templates, and audit requirements once, then let authorised team members execute approved actions quickly from the CLI.

The long-term vision is a practical DevOps operating system for VPS fleets: one secure CLI, one access model, one inventory, one audit trail, and one repeatable way to perform operational work.

---

## 4. Product Principles

1. **CLI-first, not CLI-only**  
   The CLI is the primary interface. A web console may exist for administration, audit, reporting, and approvals, but operational execution should be excellent from the terminal.

2. **Secure by default**  
   No shared SSH keys, no permanent overpowered credentials, no silent execution, and no unaudited privileged operations.

3. **Least privilege delegation**  
   Junior engineers should only be able to run approved actions against approved targets under clear policy.

4. **Fast common tasks**  
   A routine server check, restart, patch status check, or log pull should take seconds, not require a custom playbook.

5. **Everything important is auditable**  
   Every sensitive action must record actor, target, command, parameters, approval state, result, timestamp, source IP/device, and reason.

6. **Composability over lock-in**  
   The product should work with existing Linux servers, SSH, Ansible, shell scripts, CI/CD systems, cloud providers, identity providers, and SIEM tools.

7. **Guardrails, not handcuffs**  
   Senior engineers need escape hatches for emergencies, but those actions should be explicit, time-limited, and heavily audited.

8. **Boring infrastructure is good infrastructure**  
   Prefer predictable, testable, well-understood technology over clever complexity.

---

## 5. Target Users and Personas

### 5.1 DevOps Manager

**Responsibilities**

- Owns operational reliability, team access, compliance, and delivery speed.
- Needs visibility into activity across environments and engineers.
- Wants junior staff to handle routine work safely.

**Needs**

- Team-level access control.
- Audit logs and reporting.
- Approval workflows.
- Compliance evidence.
- Operational dashboards.
- Clear separation between production, staging, customer, and internal infrastructure.

**Success looks like**

- Fewer senior-engineer interruptions.
- Faster incident handling.
- Cleaner audit evidence.
- Reduced risk from overprivileged accounts.

### 5.2 Senior DevOps Engineer / Platform Engineer

**Responsibilities**

- Designs infrastructure standards.
- Handles complex incidents.
- Writes scripts and runbooks.
- Reviews access and operational risk.

**Needs**

- Fast CLI access to large server groups.
- Ability to define safe tasks for others.
- Reusable command templates and runbooks.
- Policy-as-code where possible.
- Powerful emergency workflows with audit trails.
- Integration with existing tooling.

**Success looks like**

- Can perform fleet operations quickly.
- Can delegate repetitive tasks without giving away root access.
- Can review exactly what happened during incidents.

### 5.3 Junior DevOps Engineer

**Responsibilities**

- Performs routine operational checks and approved tasks.
- Escalates complex or risky issues.
- Learns operational practices under supervision.

**Needs**

- Simple, safe commands.
- Clear feedback and error messages.
- Limited options that match their permissions.
- Built-in explanations and runbook guidance.
- Ability to request access or approval when blocked.

**Success looks like**

- Can restart approved services, inspect logs, check health, run diagnostics, and collect evidence without needing broad shell access.
- Knows when an action is blocked and how to request approval.

### 5.4 Security / Compliance Reviewer

**Responsibilities**

- Reviews access policies and operational evidence.
- Investigates incidents or suspicious activity.
- Ensures least privilege and compliance requirements are followed.

**Needs**

- Immutable audit trail.
- Searchable activity history.
- Exportable reports.
- Policy change history.
- User access review reports.
- Integration with SIEM/log platforms.

**Success looks like**

- Can answer who accessed what, when, why, and what changed.

### 5.5 Managed Service Provider / Consultant

**Responsibilities**

- Manages VPS infrastructure for multiple clients.
- Needs clean tenant separation.
- Needs evidence for client reporting.

**Needs**

- Multi-tenant organisation model.
- Client/project scoping.
- Branded or exportable reports.
- Per-client access and billing boundaries.
- Fast onboarding and offboarding.

**Success looks like**

- Can manage multiple client fleets safely without cross-client leakage.

---

## 6. Product Positioning

VPS Tools should position itself as:

> A secure CLI operations layer for VPS fleets, combining the speed of SSH, the repeatability of runbooks, and the governance of privileged access management.

### 6.1 Comparable Tool Categories

VPS Tools overlaps with, but should not directly clone, the following categories:

- **SSH and shell scripts**: Fast but hard to govern and audit consistently.
- **Ansible/AWX/Automation Platform**: Strong automation and RBAC, but often heavier than needed for day-to-day VPS operations.
- **Rundeck/runbook automation**: Good for self-service operations and job execution, but often web/runbook-centred rather than CLI-native.
- **Teleport/Boundary/privileged access tools**: Strong secure access and audit concepts, but less focused on opinionated VPS fleet operations.
- **Cloud provider dashboards**: Useful per-provider, but fragmented across VPS vendors and environments.

### 6.2 Differentiation

VPS Tools should differentiate on:

- CLI-native VPS operations.
- Delegated command templates and safe task execution.
- Built-in server inventory and grouping.
- Security controls designed for practical small and mid-sized DevOps teams.
- Fast setup for heterogeneous VPS fleets.
- Audit trail as a first-class feature, not an afterthought.
- Opinionated workflows for common Linux VPS administration tasks.
- Optional integration with existing Ansible/shell scripts rather than requiring a full migration.

---

## 7. Goals

### 7.1 Business Goals

- Create a commercially viable DevOps SaaS/self-hosted product for VPS fleet management.
- Appeal to teams too mature for ad hoc SSH but too lean for complex enterprise platforms.
- Support a path from solo/small-team usage to managed team operations.
- Enable future expansion into compliance reporting, managed operations, AI-assisted runbooks, and MSP tooling.

### 7.2 Product Goals

- Provide a fast CLI for common VPS management tasks.
- Centralise server inventory and metadata.
- Provide secure access delegation for teams.
- Capture a complete audit trail for sensitive operations.
- Reduce routine senior engineer workload.
- Make fleet-wide operations safer and more repeatable.

### 7.3 Security Goals

- Remove dependence on shared static SSH keys.
- Support least privilege access.
- Support just-in-time access and approval workflows.
- Record every privileged action.
- Make policy and access changes reviewable.
- Integrate with external identity providers and logging systems.

---

## 8. Non-Goals

The MVP should not attempt to be:

- A full cloud management platform.
- A complete Ansible replacement.
- A Kubernetes platform.
- A monitoring platform equivalent to Datadog, Prometheus, or New Relic.
- A SIEM.
- A backup platform, although it may verify backup state and trigger backup scripts.
- A full configuration management database for every IT asset type.
- A general remote desktop platform.
- A secrets manager replacement, although it must integrate with or provide safe secret handling.
- A billing/provisioning system for VPS resellers in the first release.

---

## 9. Core Use Cases

### 9.1 Fleet Inventory

Users need to discover, register, tag, group, and inspect VPS servers.

Examples:

- List all production servers.
- Show all Ubuntu 22.04 servers with pending security updates.
- Show servers by provider, customer, region, environment, role, or owner.
- Detect unmanaged or stale servers.
- Record server metadata such as OS, kernel, uptime, public IP, private IP, provider, tags, installed agent status, and last check-in.

### 9.2 Secure Access

Users need to access servers without sharing permanent credentials.

Examples:

- Senior engineer opens an audited shell session to a production server.
- Junior engineer can only open a read-only diagnostic session or run approved commands.
- Emergency production access requires a reason and optional approval.
- All session start/end metadata is recorded.

### 9.3 Command Execution

Users need to run commands against one server or many servers.

Examples:

- Check disk usage across all web servers.
- Restart Nginx on selected staging servers.
- Run a diagnostic script against servers tagged `customer:acme` and `role:web`.
- Dry-run a command before execution.
- Require approval for destructive commands.

### 9.4 Delegated Runbooks

Senior engineers need to package safe operational actions for junior engineers.

Examples:

- “Restart approved service” runbook.
- “Collect Nginx error logs” runbook.
- “Check failed systemd services” runbook.
- “Apply security patches to staging” runbook.
- “Rotate application logs” runbook.

Runbooks should define parameters, allowed targets, required role, approval requirement, timeout, rollback notes, and audit metadata.

### 9.5 Patch and Update Management

Users need to inspect and optionally apply operating system updates.

Examples:

- Show pending updates by server.
- Identify critical security updates.
- Apply updates to staging first.
- Apply production updates only inside maintenance windows.
- Record update result and reboot requirement.

### 9.6 Service Management

Users need to inspect, restart, reload, enable, disable, and view logs for system services.

Examples:

- Check status of `nginx`, `php-fpm`, `mysql`, `postgresql`, `docker`, or custom services.
- Restart a service on one server.
- Restart a service across a canary subset, then continue to the full group.
- View recent systemd journal entries.

### 9.7 Firewall and Network Checks

Users need to validate basic network exposure and firewall posture.

Examples:

- List open listening ports.
- Check UFW/firewalld/iptables/nftables status.
- Confirm SSH is not exposed unexpectedly.
- Compare current open ports to expected policy.
- Run connectivity checks from runner to server.

### 9.8 User and SSH Key Management

Users need controlled ways to inspect and manage local Linux users and SSH keys.

Examples:

- List local users with sudo access.
- Detect orphaned SSH keys.
- Add temporary access for an engineer.
- Revoke user access across all servers.
- Enforce approved SSH key policy.

### 9.9 Log Collection and Diagnostics

Users need fast evidence collection during incidents.

Examples:

- Pull last 500 lines of Nginx error logs.
- Collect system metrics snapshot.
- Package diagnostics for a service.
- Save collected logs against an incident reference.

### 9.10 Compliance and Audit Reporting

Managers and reviewers need evidence of activity and access.

Examples:

- Export all production actions for a date range.
- Show all actions by a specific engineer.
- Show all failed privileged commands.
- Show access granted, denied, approved, or escalated.
- Show policy changes and who made them.

---

## 10. MVP Scope

The MVP should focus on a narrow but complete wedge:

> Secure CLI-driven VPS inventory, controlled command execution, delegated runbooks, RBAC, approvals, and audit logging for Linux servers over SSH.

### 10.1 MVP Features

#### A. CLI Application

Command name: `vps`

Minimum commands:

```bash
vps login
vps logout
vps whoami
vps org switch
vps server add
vps server list
vps server inspect <server>
vps server check <server|group>
vps ssh <server>
vps exec <server|group> -- <command>
vps run <runbook> --target <server|group> [params]
vps service status <service> --target <server|group>
vps service restart <service> --target <server|group>
vps updates check --target <server|group>
vps audit search
vps audit show <event-id>
vps access request --target <server|group> --reason <reason>
vps approvals list
vps approvals approve <request-id>
vps approvals deny <request-id>
```

#### B. Central Control Plane

- API service for authentication, inventory, policy, execution orchestration, and audit.
- PostgreSQL database for relational data.
- Queue/worker system for asynchronous tasks.
- Secure storage for execution logs.
- Admin web console for setup, access control, audit review, and approval decisions.

#### C. Server Inventory

- Register servers manually by hostname/IP.
- Import servers from CSV for MVP.
- Store metadata: name, IP, provider, OS, environment, role, tags, owner, customer/project, status, last seen.
- Group servers by tags and saved filters.
- Basic health check over SSH.

#### D. Authentication

- Email/password for early development only.
- MFA required for production use.
- OIDC support for Google Workspace, Microsoft Entra ID, Okta, Auth0, or similar.
- Personal access tokens for automation with scoped permissions.
- CLI login using browser-based device flow or short-lived token exchange.

#### E. Authorisation and RBAC

- Organisation-level roles.
- Project/client/environment scopes.
- Server group permissions.
- Command/runbook permissions.
- Explicit production controls.
- Deny-by-default policy.

Minimum roles:

| Role | Capabilities |
|---|---|
| Owner | Full organisation control, billing, dangerous settings |
| Admin | Manage users, policies, inventory, runbooks, approvals |
| Senior Engineer | Execute privileged tasks, create runbooks, approve selected actions |
| Engineer | Execute approved tasks and request elevated access |
| Junior Engineer | Run delegated low-risk tasks only |
| Auditor | Read-only access to audit logs, reports, and policies |

#### F. Delegated Runbooks

Runbook definition should include:

- Name and description.
- Command or script body.
- Allowed parameters.
- Allowed target groups.
- Required role.
- Required approval rule.
- Timeout.
- Concurrency limit.
- Rollback guidance.
- Risk level.
- Output handling rules.

Example runbook definition:

```yaml
name: restart-nginx
summary: Restart Nginx on approved web servers
risk: medium
allowed_targets:
  tags:
    role: web
allowed_environments:
  - staging
  - production
required_role: engineer
approval:
  production: required
  staging: not_required
command: systemctl restart nginx && systemctl status nginx --no-pager
sudo: true
timeout_seconds: 60
concurrency: 5
output_policy: store_full
```

#### G. Command Execution

- Execute command on one server.
- Execute command on server group.
- Support dry-run/preflight checks where possible.
- Show progress and per-server result.
- Store stdout, stderr, exit code, duration, actor, reason, and target.
- Support command allowlist and blocklist.
- Require reason for privileged or production execution.
- Support concurrency limits.
- Support cancellation where technically possible.

#### H. SSH Access

- Launch SSH session through controlled workflow.
- Enforce RBAC before connection.
- Require reason for sensitive targets.
- Record session metadata in MVP.
- Full terminal recording can be post-MVP unless technically feasible within MVP.
- Support short-lived SSH certificates or ephemeral keys as preferred direction.

#### I. Approvals

- Users can request access or execution approval.
- Approvers can approve or deny from CLI or web console.
- Approval records include requester, approver, reason, target, command/runbook, expiry, timestamp, and decision notes.
- Approved access should be time-limited.

#### J. Audit Trail

Every important action should create an immutable audit event.

Minimum audited events:

- Login/logout.
- Failed login.
- Server added/removed/edited.
- Policy created/changed/deleted.
- Runbook created/changed/deleted.
- Command executed.
- SSH session started/ended.
- Approval requested/approved/denied/expired.
- Access token created/revoked.
- User invited/removed/role changed.
- Export generated.

Audit event fields:

| Field | Description |
|---|---|
| event_id | Unique event ID |
| timestamp | UTC timestamp |
| actor_id | User or automation identity |
| actor_role | Role at time of action |
| organisation_id | Tenant/organisation |
| source_ip | Source IP address |
| device_id | Known device/session ID where available |
| action | Canonical action name |
| target_type | Server, group, policy, runbook, user, etc. |
| target_id | Target identifier |
| environment | Production, staging, development, etc. |
| reason | User-provided reason where required |
| approval_id | Linked approval request if applicable |
| command_hash | Hash of command or script |
| command_preview | Redacted command preview |
| result | Success, failure, denied, cancelled |
| exit_code | Execution exit code where relevant |
| output_ref | Link to stored output/logs |
| metadata | Structured additional context |

#### K. Basic Web Console

The MVP web console should support:

- Organisation setup.
- User invitations.
- Role assignment.
- Server inventory view.
- Runbook management.
- Approval queue.
- Audit log search.
- Basic settings.

The web console should not try to replace the CLI for day-to-day operations in the MVP.

---

## 11. Post-MVP Features

### 11.1 Security Enhancements

- Full terminal session recording and replay.
- Tamper-evident audit log chain.
- Integration with external SIEM tools.
- Just-in-time privileged elevation.
- Break-glass workflows with automatic notifications.
- Device trust and conditional access.
- Secret injection without exposing secrets to users.
- Policy-as-code using a formal policy engine.

### 11.2 Operations Enhancements

- Agent-based mode for servers behind firewalls/NAT.
- Provider integrations for Hetzner, DigitalOcean, Linode/Akamai, AWS Lightsail, Vultr, OVHcloud, Azure, and Google Cloud.
- Scheduled jobs and maintenance windows.
- Patch orchestration waves.
- Reboot orchestration.
- Drift detection.
- Baseline hardening checks.
- Backup verification integrations.
- Certificate expiry checks.
- Domain/DNS checks.
- Docker/container support.
- WordPress/LAMP stack operational modules if targeting web agencies/MSPs later.

### 11.3 Collaboration Enhancements

- Incident mode with timeline.
- Slack/Microsoft Teams approval and notifications.
- Change tickets and Jira/Linear integrations.
- Markdown runbook documentation attached to executable runbooks.
- Post-action notes and handover summaries.

### 11.4 Reporting Enhancements

- Access review reports.
- Client-facing monthly operations reports.
- Compliance packs.
- Risk scoring.
- Server hygiene score.
- Production change report.

### 11.5 AI-Assisted Enhancements

AI should be optional and carefully governed.

Potential features:

- Summarise command output.
- Suggest likely cause of common server issues.
- Generate runbook drafts from senior engineer prompts.
- Explain failed commands to junior engineers.
- Produce incident summaries from audit events and logs.
- Flag risky commands before execution.

AI must not silently execute actions or bypass approval rules.

---

## 12. Detailed Functional Requirements

### 12.1 Organisation and Tenant Management

**Requirements**

- Users can create or join organisations.
- Organisations contain users, teams, servers, groups, policies, runbooks, audit logs, and settings.
- MSP-style users may belong to multiple organisations.
- Organisation data must be isolated.
- Organisation owners can export audit logs and inventory.

**Acceptance criteria**

- A user can switch organisation context from the CLI.
- A user cannot access servers or audit data from another organisation unless explicitly invited.
- Audit events include organisation ID.

### 12.2 Server Registration

**Requirements**

- Add a server by hostname/IP and SSH connection profile.
- Validate connectivity during registration.
- Capture OS metadata.
- Assign tags during or after registration.
- Support bulk CSV import.
- Mark servers inactive rather than hard deleting by default.

**Acceptance criteria**

- A senior engineer can add a server and run a health check within five minutes.
- Invalid SSH details produce actionable errors.
- Server inventory is searchable by name, tag, IP, provider, environment, and role.

### 12.3 Server Groups

**Requirements**

- Static groups: manually selected servers.
- Dynamic groups: tag/filter based.
- Groups can be used as command targets.
- Groups can have policy constraints.

**Acceptance criteria**

- `vps server list --tag env=prod --tag role=web` returns matching servers.
- `vps exec group:web-prod -- uptime` runs only against servers the user can access.

### 12.4 CLI Authentication

**Requirements**

- CLI supports secure login.
- Tokens are stored using the OS credential store where possible.
- Tokens are short-lived or refreshable with appropriate controls.
- Logout revokes local session.
- `vps whoami` shows current user, organisation, role, and active scopes.

**Acceptance criteria**

- A user cannot run any command without authentication.
- Revoked users lose CLI access.
- Expired sessions require re-authentication.

### 12.5 RBAC and Policy

**Requirements**

- Every command is authorised before execution.
- Authorisation checks actor, role, target, environment, action, risk, approval state, and time window.
- Policies can restrict commands by exact match, template, runbook, risk level, or server group.
- Dangerous commands should be blocked or require elevated approval.

**Acceptance criteria**

- A junior engineer cannot run arbitrary shell commands on production.
- A senior engineer can create a runbook that juniors may execute only on staging.
- A denied command produces a clear reason and suggested next step.

### 12.6 Execution Engine

**Requirements**

- Supports single-target and multi-target execution.
- Supports concurrency limits.
- Captures stdout, stderr, exit code, start/end time, and result.
- Redacts configured secrets from command previews and output.
- Supports timeout.
- Handles partial failure across fleets.
- Produces structured output suitable for humans and automation.

**Acceptance criteria**

- A command across ten servers returns per-server results.
- Failed servers are clearly listed.
- Command output is linked to audit events.

### 12.7 Runbooks

**Requirements**

- Runbooks can be created, edited, versioned, enabled, disabled, and deleted.
- Runbooks support parameters with validation.
- Runbooks support allowed target rules.
- Runbooks support approval rules.
- Runbooks support command/script body.
- Runbooks support rollback notes and documentation.

**Acceptance criteria**

- A senior engineer can publish a runbook for juniors.
- A junior engineer can discover permitted runbooks using `vps runbook list`.
- Runbook changes are audited.
- Old runbook versions remain reviewable.

### 12.8 Approval Workflow

**Requirements**

- Policy can require approval based on role, target, environment, action, risk, or command.
- Approval requests include reason and expiry.
- Approvers are selected based on policy.
- Approved requests create a temporary permission grant.
- Denied and expired requests are recorded.

**Acceptance criteria**

- A production restart by a junior engineer requires approval.
- A staging diagnostic runbook does not require approval if policy allows it.
- Approved access expires automatically.

### 12.9 Audit Log

**Requirements**

- Audit events are append-only from the application perspective.
- Events are timestamped in UTC.
- Events are searchable by actor, target, action, environment, result, date range, and request ID.
- Sensitive fields are redacted.
- Audit logs can be exported.
- Audit retention can be configured by plan.

**Acceptance criteria**

- An auditor can find all commands run against a production server in a given week.
- A manager can see all actions by a junior engineer.
- A deleted user’s historic events remain visible.

### 12.10 Notifications

**MVP requirements**

- Email notifications for approval requests, approval decisions, access changes, and break-glass usage.

**Post-MVP requirements**

- Slack and Microsoft Teams notifications.
- Webhooks.
- PagerDuty/Opsgenie integration.

### 12.11 API

**Requirements**

- Public API should support key resources: servers, groups, runbooks, executions, approvals, audit events, users, and policies.
- API tokens must be scoped.
- API actions must be audited.
- API should be versioned.

**Acceptance criteria**

- CI/CD can trigger approved runbooks via scoped token.
- API token creation and revocation are audited.

---

## 13. CLI UX Requirements

### 13.1 CLI Design Principles

- Predictable command structure.
- Good defaults.
- Safe prompts for risky actions.
- Machine-readable output support.
- Clear denied/blocked messages.
- Helpful examples.
- No hidden destructive behaviour.

### 13.2 Output Modes

The CLI should support:

```bash
--output table
--output json
--output yaml
--quiet
--verbose
```

### 13.3 Confirmation Rules

Commands should require explicit confirmation when:

- Targeting production.
- Targeting more than a configured number of servers.
- Running a privileged action.
- Running a destructive command.
- Restarting services.
- Rebooting servers.
- Changing firewall, user, or SSH settings.

Confirmation can be bypassed only when policy permits and `--yes` is used by an authorised role.

### 13.4 Example CLI Workflows

#### List production web servers

```bash
vps server list --tag env=prod --tag role=web
```

#### Check health across a group

```bash
vps server check group:prod-web
```

#### Run a diagnostic command

```bash
vps exec group:staging-web -- df -h
```

#### Run approved delegated runbook

```bash
vps run restart-nginx --target server:web-01 --reason "Recovering from 502 errors"
```

#### Request temporary production access

```bash
vps access request --target group:prod-web --duration 30m --reason "Investigating elevated 5xx rate"
```

#### Search audit logs

```bash
vps audit search --actor jane@example.com --target server:web-01 --since 2026-05-01
```

---

## 14. Data Model: Conceptual Entities

### 14.1 Organisation

- id
- name
- plan
- settings
- created_at

### 14.2 User

- id
- email
- name
- status
- mfa_enabled
- created_at
- last_login_at

### 14.3 Membership

- organisation_id
- user_id
- role
- teams
- status

### 14.4 Server

- id
- organisation_id
- name
- hostname
- public_ip
- private_ip
- provider
- region
- os_name
- os_version
- kernel_version
- environment
- tags
- status
- last_seen_at
- created_at

### 14.5 Server Group

- id
- organisation_id
- name
- type: static/dynamic
- filter_definition
- server_ids
- created_at

### 14.6 Runbook

- id
- organisation_id
- name
- version
- description
- risk_level
- command_template
- parameters_schema
- allowed_targets
- approval_policy
- timeout
- concurrency
- enabled
- created_by
- updated_at

### 14.7 Execution

- id
- organisation_id
- actor_id
- runbook_id
- command_hash
- command_preview
- target_definition
- status
- reason
- approval_id
- started_at
- finished_at

### 14.8 Execution Result

- id
- execution_id
- server_id
- status
- exit_code
- stdout_ref
- stderr_ref
- duration_ms
- error_summary

### 14.9 Approval Request

- id
- organisation_id
- requester_id
- approver_id
- action_type
- target_definition
- reason
- status
- expires_at
- decided_at
- decision_note

### 14.10 Audit Event

- id
- organisation_id
- actor_id
- action
- target_type
- target_id
- result
- timestamp
- source_ip
- metadata

---

## 15. Security Requirements

### 15.1 Identity and Access

- MFA required for privileged roles.
- OIDC support for production customers.
- Role and policy checks on every action.
- No shared user accounts.
- Service accounts must be scoped and auditable.
- Access grants must expire.

### 15.2 Credential Handling

- No plaintext secrets in logs, audit events, CLI config, or command output where avoidable.
- Secrets encrypted at rest.
- Secret access audited.
- Prefer short-lived credentials over static credentials.
- Support integration with external secret managers post-MVP.

### 15.3 Transport Security

- TLS for all API traffic.
- SSH host key verification should be supported and encouraged.
- CLI should warn on changed host keys.
- Server-side outbound connections should be restricted where possible.

### 15.4 Audit Integrity

- Audit logs should be append-only at the application level.
- Audit exports should include checksums.
- Post-MVP should support tamper-evident log chaining.
- Administrators should not be able to silently alter historic audit events.

### 15.5 Command Safety

- Deny-by-default for arbitrary privileged commands.
- Command previews shown before approval.
- Risk classification for commands and runbooks.
- Redaction for known secret patterns.
- Dangerous commands blocked by default for junior roles.

### 15.6 Production Safeguards

- Production environment must be explicitly tagged.
- Production commands require reason.
- Risky production actions require approval unless policy exempts specific senior roles.
- Multi-server production actions require confirmation.
- Emergency access is time-limited and heavily audited.

---

## 16. Non-Functional Requirements

### 16.1 Performance

- CLI commands should return initial feedback in under two seconds where possible.
- Single-server health check should complete in under ten seconds under normal network conditions.
- Fleet operations should stream progress incrementally.
- Audit search should return common queries in under three seconds for normal customer sizes.

### 16.2 Reliability

- Control plane should tolerate worker failure without losing execution state.
- Execution results should be recoverable after transient failures.
- Partial failures must be explicit.
- The product should fail closed for authorisation errors.

### 16.3 Scalability

MVP target scale:

- 1 organisation to 500 servers.
- 1 to 100 users per organisation.
- 1,000 executions per day per organisation.
- 90-day audit retention for standard plan.

Post-MVP target scale:

- 5,000+ servers per organisation.
- 1 million+ audit events per organisation.
- Multi-region execution workers.

### 16.4 Availability

- MVP hosted control plane target: 99.5% monthly availability.
- Production target after maturity: 99.9% monthly availability.
- Self-hosted edition should document backup and recovery requirements.

### 16.5 Compatibility

MVP server support:

- Ubuntu 20.04, 22.04, 24.04.
- Debian 11 and 12.
- Basic support for AlmaLinux/Rocky Linux should be considered but not required for the first MVP if it slows delivery.

MVP client support:

- macOS.
- Linux.
- Windows via native binary and/or WSL-friendly workflow.

### 16.6 Observability

The product itself should expose:

- API logs.
- Worker logs.
- Execution metrics.
- Queue depth.
- Failed execution count.
- Audit ingestion failures.
- Authentication failures.
- Policy denial metrics.

---

## 17. Deployment Model

VPS Tools should be designed as a hybrid product from the beginning, with both SaaS and self-hosted deployment options.

The deployment model is part of the product strategy, not an afterthought. Different customer segments will have different security, compliance, budget, and control requirements. The product should therefore support three commercial delivery paths:

1. Hosted SaaS.
2. Self-hosted open-source base edition.
3. Self-hosted supported commercial edition with full functionality.

### 17.1 Hosted SaaS

The SaaS edition should be the default route for most small and mid-sized teams that want fast setup, low operational overhead, managed upgrades, and simple pricing.

In the SaaS model:

- VPS Tools hosts and operates the control plane.
- Customers use the CLI and web console against the hosted service.
- Customers may deploy customer-managed runners inside their own network where private server access is required.
- SaaS handles authentication, policy, audit, approvals, inventory, reporting, and orchestration.
- Execution runners handle network access to customer servers.

This model is best for:

- Small and mid-sized DevOps teams.
- SaaS companies with lean infrastructure teams.
- Agencies and consultants who want low setup overhead.
- Customers who do not want to maintain another internal platform.

### 17.2 Self-Hosted Open-Source Base Edition

The self-hosted open-source base edition should provide a credible, useful product with unlimited seats. It should not be a crippled demo.

The open-source edition should be suitable for:

- Solo operators.
- Small teams.
- Homelab and technical users.
- Security-conscious teams evaluating the product.
- Organisations that require source visibility before commercial adoption.

Expected characteristics:

- Unlimited seats.
- Self-hosted control plane.
- Core CLI functionality.
- Server inventory.
- Basic command execution.
- Basic runbooks.
- Basic RBAC.
- Basic audit trail.
- Community support.
- Public documentation.
- Docker Compose deployment as the primary installation route.

The open-source edition should create adoption, trust, and technical credibility. It should also act as the bottom of the commercial funnel for the supported edition and SaaS.

### 17.3 Self-Hosted Supported Commercial Edition

The self-hosted supported commercial edition should provide full product functionality for customers that need control over deployment, data residency, internal security review, or regulated operating environments.

Expected characteristics:

- Full feature set.
- Unlimited or contract-defined seat model.
- Commercial support.
- Upgrade assistance.
- Advanced RBAC and policy features.
- Advanced audit retention and export.
- OIDC/SSO.
- Customer-managed storage.
- SIEM/logging integrations.
- Advanced approval workflows.
- Terminal session recording where available.
- High-availability deployment option.
- Helm chart or production-grade deployment option post-MVP.

This model is best for:

- Larger infrastructure teams.
- MSPs.
- Security-sensitive organisations.
- Customers with strict compliance or data residency requirements.
- Teams that cannot send operational metadata to a SaaS control plane.

### 17.4 Edition Boundary Principles

The edition boundaries should be clear and defensible.

The open-source edition should include enough capability to be genuinely useful, but advanced governance, enterprise integrations, long-term audit management, commercial support, and operational scale features should belong to paid editions.

Do not restrict open-source adoption by seat count. The main commercial distinction should be based on support, advanced governance, hosted convenience, enterprise integrations, and operational scale.

Recommended boundary:

| Capability | Open-Source Self-Hosted | Supported Self-Hosted | SaaS |
|---|---:|---:|---:|
| Unlimited seats | Yes | Yes or contract-based | Juniors unlimited; managers/senior engineers paid |
| Core CLI | Yes | Yes | Yes |
| Server inventory | Yes | Yes | Yes |
| Basic command execution | Yes | Yes | Yes |
| Basic runbooks | Yes | Yes | Yes |
| Basic RBAC | Yes | Yes | Yes |
| Basic audit trail | Yes | Yes | Yes |
| Advanced RBAC/policy | Limited | Yes | Yes |
| Approvals | Limited or standard | Yes | Yes |
| OIDC/SSO | Optional/community or limited | Yes | Yes, plan-dependent |
| Advanced audit retention/export | Limited | Yes | Yes, plan-dependent |
| SIEM/webhook integrations | Limited | Yes | Yes |
| Terminal session recording | No or limited | Yes | Yes, plan-dependent |
| Commercial support | No | Yes | Yes |
| Managed upgrades | No | No | Yes |
| High availability guidance | Community | Yes | Included in SaaS platform |

### 17.5 Agentless vs Agent-Based

MVP should start with agentless SSH execution because it reduces onboarding friction.

However, agent-based mode should be planned for:

- NAT/private servers.
- Better health reporting.
- Long-running jobs.
- Reduced inbound SSH exposure.
- Offline check-in model.

The architecture should not paint the product into an agentless-only corner.

### 17.6 Recommended MVP Deployment Scope

The MVP should support:

- Hosted SaaS control plane.
- Customer-managed runner for SaaS customers who need private network access.
- Docker Compose self-hosted deployment for the open-source base edition.
- A clear upgrade path from open-source self-hosted to supported self-hosted.

Production-grade Helm/high-availability deployment can follow after the MVP unless a launch customer requires it.

---

## 18. Suggested Technical Architecture

### 18.1 Components

1. **CLI**
   - Written in Go or Rust for single-binary distribution.
   - Handles login, command construction, output rendering, and local config.

2. **API Service**
   - Handles authentication, authorisation, inventory, policies, runbooks, approvals, and audit queries.

3. **Execution Worker / Runner**
   - Performs SSH connections and command execution.
   - Can be SaaS-hosted or customer-hosted.

4. **Web Console**
   - Admin, inventory, approval, and audit interface.

5. **Database**
   - PostgreSQL for core data.

6. **Queue**
   - Redis, NATS, or similar for execution jobs.

7. **Object Storage**
   - Stores stdout/stderr logs, session recordings, exports, and large artefacts.

8. **Policy Engine**
   - Initially application-level rules.
   - Post-MVP can move to OPA, Cedar, or another formal policy engine.

### 18.2 Execution Flow

1. User runs a CLI command.
2. CLI authenticates with control plane.
3. Control plane validates user, organisation, role, policy, target, and approval state.
4. If approval is required, request is created and command is not executed.
5. If approved or allowed, execution job is queued.
6. Worker claims job.
7. Worker connects to target server via SSH or agent channel.
8. Worker runs command/runbook with timeout and output capture.
9. Results are streamed to CLI where possible.
10. Execution results and audit events are stored.
11. Notifications are sent where required.

---

## 19. Integrations

### 19.1 MVP Integrations

- OIDC identity provider.
- Email notifications.
- CSV import/export.
- Webhook for audit/event export.

### 19.2 Post-MVP Integrations

- Slack.
- Microsoft Teams.
- Jira.
- Linear.
- ServiceNow.
- PagerDuty.
- Opsgenie.
- Datadog.
- Prometheus/Grafana.
- Splunk.
- Elastic.
- HashiCorp Vault.
- 1Password Secrets Automation.
- AWS Secrets Manager.
- Cloudflare.
- Hetzner.
- DigitalOcean.
- Linode/Akamai.
- Vultr.
- OVHcloud.

---

## 20. User Stories

### 20.1 DevOps Manager

- As a DevOps manager, I want to see all production commands run this week so I can review operational risk.
- As a DevOps manager, I want to assign junior engineers to approved runbooks only so they can help without broad access.
- As a DevOps manager, I want approval workflows for production actions so risky changes are controlled.
- As a DevOps manager, I want monthly access review reports so I can prove least privilege.

### 20.2 Senior DevOps Engineer

- As a senior engineer, I want to register servers with tags so I can target groups quickly.
- As a senior engineer, I want to run a command across selected servers so I can diagnose fleet issues quickly.
- As a senior engineer, I want to create safe runbooks so common work can be delegated.
- As a senior engineer, I want to see output and failures per server so I can handle partial failure cleanly.

### 20.3 Junior DevOps Engineer

- As a junior engineer, I want to see which tasks I am allowed to run so I do not need to guess.
- As a junior engineer, I want to restart an approved staging service so I can resolve simple issues.
- As a junior engineer, I want to request temporary production access so I can help during incidents.
- As a junior engineer, I want clear denial messages so I know whether to escalate.

### 20.4 Auditor

- As an auditor, I want to search all activity by user, server, and date so I can investigate incidents.
- As an auditor, I want audit exports so I can attach evidence to compliance records.
- As an auditor, I want to see policy changes so I know who changed access rules.

---

## 21. Acceptance Criteria for MVP

The MVP can be considered complete when:

1. A new organisation can be created.
2. Users can be invited and assigned roles.
3. A server can be added and tagged.
4. A CLI user can authenticate securely.
5. A senior engineer can run an authorised command against one server.
6. A senior engineer can run an authorised command against a server group.
7. A junior engineer is blocked from unauthorised arbitrary production commands.
8. A junior engineer can run a delegated runbook where policy allows it.
9. A production action can require approval.
10. An approver can approve or deny from CLI or web console.
11. Every execution creates an audit event.
12. Execution stdout/stderr and exit codes are stored.
13. Audit logs can be searched and exported.
14. Access changes and runbook changes are audited.
15. Documentation exists for onboarding the first customer.

---

## 22. Success Metrics

### 22.1 Activation Metrics

- Time to first server registered.
- Time to first successful command execution.
- Time to first delegated runbook.
- Percentage of invited users who complete CLI login.

### 22.2 Engagement Metrics

- Weekly active CLI users.
- Weekly executions per organisation.
- Runbook executions versus arbitrary commands.
- Number of servers actively managed.
- Number of delegated actions performed by non-senior users.

### 22.3 Security Metrics

- Percentage of production actions with reason attached.
- Number of denied unauthorised attempts.
- Number of emergency accesses.
- Time from user removal to access revocation.
- Percentage of privileged users with MFA enabled.

### 22.4 Business Metrics

- Trial-to-paid conversion.
- Active organisations.
- Servers under management.
- Retention by organisation.
- Expansion revenue from additional users/servers.

---

## 23. Pricing and Packaging Hypothesis

Pricing should be simple, predictable, and aligned with the product’s delegation model.

The core pricing principle is:

> Charge for the people who govern and perform privileged operations, not for every junior user who benefits from safe delegation.

This avoids punishing customers for giving junior engineers controlled, auditable access. It also supports the main product promise: senior engineers and managers can safely delegate routine operational work without creating licensing friction.

### 23.1 Deployment and Commercial Packages

VPS Tools should support three main packages:

1. **Self-Hosted Open-Source Base**
2. **Self-Hosted Supported**
3. **Hosted SaaS**

### 23.2 Self-Hosted Open-Source Base

The open-source base edition should have unlimited seats.

Purpose:

- Build developer trust.
- Drive adoption.
- Support transparency.
- Create a low-friction entry point.
- Provide a credible product for small teams and technical evaluators.

Packaging:

- Unlimited users.
- Unlimited junior users.
- Community support.
- Core CLI, inventory, basic runbooks, basic RBAC, and basic audit.
- Self-managed deployment and upgrades.
- No commercial SLA.

### 23.3 Self-Hosted Supported

The supported self-hosted edition should provide full functionality for organisations that want or require local control.

Packaging:

- Full product functionality.
- Commercial support.
- Advanced RBAC and policy controls.
- Advanced approvals.
- OIDC/SSO.
- Advanced audit retention and export.
- SIEM/logging integrations.
- Terminal session recording where available.
- Upgrade guidance.
- Production deployment guidance.

Commercial model options:

- Annual organisation licence.
- Server band pricing.
- Contract-defined seat and support terms.

The supported self-hosted edition should not be positioned as a cheaper SaaS alternative. It should be positioned as the right option for customers with control, compliance, data residency, or security review requirements.

### 23.4 Hosted SaaS

The SaaS edition should use simple per-seat pricing based on organisation size.

The billable seats should be:

- Managers.
- Senior DevOps engineers.
- Equivalent privileged operators or approvers.

Managers and senior engineer seats should be the same price.

Junior DevOps engineers should be unlimited and included at no extra cost.

This creates a clean commercial message:

> Pay for managers and senior engineers. Add unlimited junior engineers for safe delegated access.

This model is easy to understand, easy to budget for, and aligned with the product’s core value.

### 23.5 SaaS Seat Definitions

| Seat type | Billable? | Notes |
|---|---:|---|
| Owner | Yes | Commercially equivalent to manager/senior engineer |
| Admin | Yes | Commercially equivalent to manager/senior engineer |
| DevOps Manager | Yes | Same price as senior engineer |
| Senior Engineer | Yes | Same price as manager |
| Engineer | To be defined | Should be billable only if materially privileged |
| Junior Engineer | No | Unlimited included seats |
| Auditor | To be defined | Could be free read-only or billable in higher plans |
| Service account | No by default | Should be limited by policy, not normal seat pricing |

The “Engineer” role needs careful product and pricing definition. If an engineer can perform broad privileged operations, the role should likely be billable. If the role is closer to a delegated operator with constrained runbook access, it should be treated like a junior/non-billable seat.

### 23.6 SaaS Tier Hypothesis

Potential SaaS tiers:

#### Team

- Per billable manager/senior engineer seat.
- Unlimited junior engineers.
- Core CLI.
- Server inventory.
- Delegated runbooks.
- RBAC.
- Approvals.
- Standard audit retention.
- Customer-managed runner support.

#### Business

- Everything in Team.
- OIDC/SSO.
- Advanced audit export.
- Webhooks.
- Longer audit retention.
- Advanced policy controls.
- Advanced reporting.
- Priority support.

#### Enterprise / MSP

- Everything in Business.
- Multi-organisation or client management.
- SIEM integrations.
- Custom audit retention.
- Custom contracts.
- Advanced compliance reporting.
- Dedicated support.
- Optional self-hosted supported deployment.

### 23.7 Pricing Guardrails

The pricing model should avoid:

- Charging for every junior user.
- Charging per execution in the early product.
- Complex combinations of user, server, execution, runner, and audit pricing.
- Making self-hosted open source feel artificially restricted.
- Creating licensing barriers to good security practice.

The pricing model may still use server bands or fair-use limits to prevent abuse, especially in SaaS.

Useful commercial dimensions:

- Billable manager/senior engineer seats.
- Server bands or soft limits.
- Audit retention.
- Advanced security features.
- Enterprise integrations.
- Support level.
- SaaS versus self-hosted.
- MSP/client management.

---

## 24. Risks and Mitigations

### Risk 1: Product becomes too broad

**Mitigation:** Keep MVP focused on inventory, secure execution, delegated runbooks, approvals, and audit.

### Risk 2: Competes too directly with Ansible/AWX or Rundeck

**Mitigation:** Position as CLI-first governed VPS operations, with integration rather than replacement as the initial strategy.

### Risk 3: Security expectations are high

**Mitigation:** Design for least privilege, auditability, MFA, short-lived credentials, and clear threat modelling from day one.

### Risk 4: SSH credential handling is difficult

**Mitigation:** Prefer short-lived certificates or ephemeral keys. Avoid shared static keys as a product norm.

### Risk 5: Audit logs are incomplete or not trustworthy

**Mitigation:** Make audit events part of the core execution path, not an optional logging side effect.

### Risk 6: Junior delegation is unsafe

**Mitigation:** Use runbooks, explicit target constraints, approval rules, and deny-by-default policies.

### Risk 7: Onboarding friction kills adoption

**Mitigation:** Provide agentless SSH MVP, CSV import, simple examples, and a guided setup flow.

### Risk 8: Fleet execution creates outages

**Mitigation:** Support concurrency limits, canary execution, dry-run where possible, production confirmations, and approval gates.

---

## 25. Open Questions

1. Should agentless SSH be the only MVP execution mode?
2. Should terminal session recording be required for MVP or deferred?
3. What is the first target market: internal DevOps teams, MSPs, web agencies, or SaaS operators?
4. Which VPS providers should be supported first for import/discovery?
5. Should arbitrary command execution be allowed for senior roles in MVP?
6. How strict should command risk classification be in the first release?
7. What compliance frameworks, if any, should the product target initially?
8. Should Ansible playbook execution be supported in MVP?
9. How should the intermediate “Engineer” role be priced in SaaS if it sits between senior engineer and junior engineer?
10. Should auditor seats be free read-only seats, billable seats, or plan-dependent?
11. What exact functionality belongs in the open-source base edition versus the supported commercial edition?
12. Should SaaS include server bands or fair-use limits alongside the simple privileged-seat model?

Resolved product decisions:

- Deployment model will be hybrid: hosted SaaS and self-hosted.
- Self-hosted will have two flavours: open-source base with unlimited seats, and supported commercial with full functionality.
- SaaS pricing will be based on organisation size using simple per-seat pricing for managers and senior engineers.
- Manager and senior engineer seats will be the same price.
- Junior engineer seats will be unlimited in SaaS.

---

## 26. Recommended MVP Build Phases

### Phase 1: Foundations

- CLI skeleton.
- API service.
- Authentication.
- Organisation model.
- PostgreSQL schema.
- Basic web console shell.
- Audit event framework.

### Phase 2: Inventory and Connectivity

- Add/list/inspect servers.
- SSH connection profiles.
- Server tags and groups.
- Health check.
- CSV import.

### Phase 3: Execution Engine

- Single-server command execution.
- Group execution.
- Output capture.
- Execution logs.
- Timeouts and concurrency limits.
- Result streaming.

### Phase 4: RBAC and Delegation

- Roles.
- Target scopes.
- Command/runbook permissions.
- Deny-by-default policy.
- Junior/senior workflows.

### Phase 5: Runbooks and Approvals

- Runbook creation and execution.
- Parameter validation.
- Approval request lifecycle.
- CLI and web approval actions.

### Phase 6: Audit and Reporting

- Audit search.
- Export.
- Execution history.
- User activity reports.
- Policy change history.

### Phase 7: Hardening and Beta

- MFA.
- OIDC.
- Token hardening.
- Redaction.
- Documentation.
- First customer onboarding.
- Security review.

---

## 27. MVP Release Recommendation

The first public beta should avoid claiming to be a complete server management platform. It should be positioned around a sharper promise:

> Safely delegate and audit VPS operations from the CLI.

The first release should prove three things:

1. Senior engineers can move faster across a VPS fleet.
2. Junior engineers can safely perform useful work without broad server access.
3. Managers and auditors can see exactly what happened.

If those three promises work, the product has a strong foundation for broader VPS automation, compliance, MSP tooling, and AI-assisted operations later.

---

## 28. One-Sentence Product Pitch

VPS Tools is a secure CLI control plane for VPS fleets that lets DevOps teams run, delegate, approve, and audit server operations without relying on shared SSH access or ad hoc scripts.

