# VPS Tools Architecture Decision Records

**Working title:** VPS Tools  
**Document status:** Draft v1  
**Date:** 18 May 2026  
**Related documents:** VPS Tools PRD; VPS Tools Technical Specification; VPS Tools MVP Build Plan  
**Primary audience:** Engineering, architecture, security, product  

---

## 1. Purpose

This document records the major architecture decisions for VPS Tools.

Architecture Decision Records are intentionally short, explicit, and practical. Their purpose is to prevent the team from repeatedly reopening the same decisions without new evidence.

Each ADR should explain:

- The context.
- The decision.
- The alternatives considered.
- The consequences.
- When the decision should be revisited.

---

## 2. ADR Status Values

Use the following statuses:

| Status | Meaning |
|---|---|
| Proposed | Under consideration, not yet committed |
| Accepted | Current direction |
| Superseded | Replaced by a newer ADR |
| Deprecated | No longer recommended, but may still exist in old code |
| Rejected | Considered and explicitly not chosen |

---

## 3. ADR Index

| ADR | Title | Status |
|---|---|---|
| ADR-0001 | Use a monorepo for the MVP | Accepted |
| ADR-0002 | Use Go for CLI, API, and runner | Accepted |
| ADR-0003 | Use Charm libraries for the interactive TUI | Accepted |
| ADR-0004 | Use Cobra for CLI command structure | Accepted |
| ADR-0005 | Use ConnectRPC and Protocol Buffers for internal APIs | Accepted |
| ADR-0006 | Use PostgreSQL as the primary database | Accepted |
| ADR-0007 | Use sqlc and explicit SQL instead of a heavy ORM | Accepted |
| ADR-0008 | Use Docker Compose as the first self-hosted deployment model | Accepted |
| ADR-0009 | Use S3-compatible object storage for execution artefacts | Accepted |
| ADR-0010 | Start with agentless SSH execution | Accepted |
| ADR-0011 | Design towards short-lived SSH certificates | Accepted |
| ADR-0012 | Use a customer-managed runner model for private infrastructure access | Accepted |
| ADR-0013 | Keep the runner outside the primary authorisation decision path | Accepted |
| ADR-0014 | Use a simple job dispatch model first, with NATS JetStream as the scale path | Accepted |
| ADR-0015 | Use OpenFGA for relationship-based authorisation, but not necessarily in Phase 0 | Accepted |
| ADR-0016 | Use an internal structured policy evaluator before adopting OPA | Accepted |
| ADR-0017 | Use OIDC-first authentication for production deployments | Accepted |
| ADR-0018 | Use append-only audit events as a core product primitive | Accepted |
| ADR-0019 | Use OpenTelemetry for observability | Accepted |
| ADR-0020 | Build the open-source base before commercial-only extensions | Accepted |
| ADR-0021 | Use Next.js and TypeScript for the web console | Accepted |
| ADR-0022 | Prioritise direct CLI workflows before rich TUI workflows | Accepted |
| ADR-0023 | Use YAML as the first runbook definition format | Accepted |
| ADR-0024 | Keep MVP policy simple and deny-by-default | Accepted |
| ADR-0025 | Defer terminal session recording until after the MVP | Accepted |

---

# ADR-0001: Use a Monorepo for the MVP

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

VPS Tools will initially include a CLI, API service, runner, web console, shared API contracts, database migrations, runbook schema, audit event definitions, and deployment assets.

During the MVP, the product will change quickly. API contracts, database schema, CLI commands, runner behaviour, and audit events will evolve together.

## Decision

Use a monorepo for the MVP.

Recommended layout:

```text
vps-tools/
  apps/
    cli/
    api/
    runner/
    web/
  packages/
    proto/
    sdk-go/
    sdk-ts/
    authz/
    audit/
    runbooks/
    sshx/
  deploy/
    docker-compose/
    helm/
    systemd/
  migrations/
    postgres/
  docs/
    adr/
    operator-guide/
    developer-guide/
  scripts/
```

## Alternatives Considered

### Multiple repositories

Rejected for the MVP. It would introduce unnecessary versioning, coordination, and release overhead before the product boundaries are stable.

### One repository per deployable service

Also rejected for the MVP. The CLI, API, and runner need to evolve together during early development.

## Consequences

### Positive

- Easier refactoring across CLI, API, runner, and shared schemas.
- Simpler local development.
- Easier end-to-end testing.
- Shared code and fixtures are easier to manage.
- Better fit for fast MVP iteration.

### Negative

- Repository can become large over time.
- Access control by component is harder.
- CI may become slower if not managed carefully.

## Revisit When

- Teams become large enough to need clearer ownership boundaries.
- Commercial/open-source packaging requires stronger code separation.
- CI times become a serious drag.

---

# ADR-0002: Use Go for CLI, API, and Runner

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

VPS Tools needs:

- A cross-platform CLI.
- A robust terminal UI.
- A networked API service.
- A secure execution runner.
- Good SSH support.
- Easy binary distribution.
- Strong concurrency primitives.
- Operational simplicity for self-hosted users.

## Decision

Use Go as the primary implementation language for the CLI, API, and runner.

## Alternatives Considered

### Rust

Rust offers excellent performance and memory safety, but the development speed and ecosystem complexity are less favourable for the MVP.

### Node.js / TypeScript

Strong for web and developer tooling, but less ideal for the runner and single-binary CLI distribution.

### Python

Good for scripting and automation, but less suitable for distributing a robust cross-platform CLI and long-running runner service.

## Consequences

### Positive

- Single static binaries are straightforward.
- Good cross-platform support.
- Strong standard library.
- Mature networking and SSH libraries.
- Good concurrency model.
- Operationally simple for self-hosted deployments.
- Strong fit for CLI and backend services.

### Negative

- UI development outside terminal/web is not Go’s strength.
- Some business logic may be more verbose than in dynamic languages.
- Web console still needs a separate TypeScript/JavaScript stack.

## Revisit When

- A component has requirements that clearly do not fit Go.
- Web/backend integration becomes more important than binary distribution.
- Plugin ecosystem requirements demand another runtime.

---

# ADR-0003: Use Charm Libraries for the Interactive TUI

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The product is CLI-first, but a pure flag-based CLI would be painful for workflows such as server browsing, runbook selection, approval review, execution monitoring, and audit search.

The TUI needs to feel modern and reliable without building a terminal UI framework from scratch.

## Decision

Use Charm libraries for the interactive TUI:

- Bubble Tea for TUI architecture.
- Bubbles for reusable components.
- Huh for forms.
- Lip Gloss for styling and layout.
- Glamour for Markdown rendering.

## Alternatives Considered

### Build a custom TUI framework

Rejected. It would waste time and introduce unnecessary maintenance risk.

### Use a simpler prompt-only library

Rejected as the main approach. Prompt-only interaction is useful for setup flows, but not enough for execution monitoring, browsing, and audit review.

### Make the web console the only rich UI

Rejected. The product’s core promise is CLI-first operations.

## Consequences

### Positive

- High-quality terminal UX without reinventing the wheel.
- Good fit with Go.
- Supports interactive workflows while preserving scriptable commands.
- Allows a modern terminal experience for power users.

### Negative

- TUI complexity can grow quickly.
- Needs careful testing of state transitions and terminal edge cases.
- Time spent polishing the TUI could delay core execution work.

## Revisit When

- Charm libraries no longer meet UX requirements.
- TUI complexity becomes too high for the team to maintain.
- A different terminal framework offers a major practical advantage.

---

# ADR-0004: Use Cobra for CLI Command Structure

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The CLI will have many nested commands and must support help text, flags, shell completions, command validation, and eventually documentation generation.

## Decision

Use Cobra for the CLI command structure.

## Alternatives Considered

### Standard library flag package

Rejected. It is too limited for a multi-command product CLI.

### urfave/cli

Viable, but Cobra is more commonly used for Kubernetes-style operational CLIs and has a strong ecosystem.

### Custom CLI parser

Rejected. No good reason to build this ourselves.

## Consequences

### Positive

- Mature command tree support.
- Good help and flag handling.
- Supports shell completion generation.
- Familiar to DevOps users.
- Works well with Go.

### Negative

- Cobra projects can become messy without command structure discipline.
- Requires conventions for flags, output modes, and errors.

## Revisit When

- Cobra becomes a blocker for command UX.
- The CLI structure changes to a model that Cobra does not support well.

---

# ADR-0005: Use ConnectRPC and Protocol Buffers for Internal APIs

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The CLI, runner, API service, and web console need a stable API contract. The API needs to support typed clients, streaming execution events, browser access, and future SDKs.

## Decision

Use ConnectRPC with Protocol Buffers for the API contract.

## Alternatives Considered

### REST with OpenAPI

Viable and familiar, but weaker for streaming and shared typed contracts across Go and TypeScript.

### gRPC directly

Powerful, but browser support and operational friction are less attractive than ConnectRPC.

### GraphQL

Good for flexible web UIs, but unnecessary complexity for command execution, runner protocols, and operational APIs.

## Consequences

### Positive

- Strong typed contracts.
- Good fit for Go and TypeScript clients.
- Supports streaming use cases.
- Enables future SDK generation.
- Reduces API drift between CLI, runner, and web console.

### Negative

- Protobuf schema discipline is required.
- Some users may expect REST endpoints for integration.
- API debugging can be less familiar than plain JSON REST for some developers.

## Revisit When

- External integration demand strongly favours REST/OpenAPI.
- ConnectRPC ecosystem support becomes a limitation.
- The web console requires API patterns that are awkward in RPC form.

---

# ADR-0006: Use PostgreSQL as the Primary Database

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

VPS Tools needs reliable relational storage for organisations, users, memberships, servers, runners, runbooks, executions, approvals, and audit events.

The product also needs JSON fields for metadata and flexible audit attributes.

## Decision

Use PostgreSQL as the primary database.

## Alternatives Considered

### MySQL / MariaDB

Viable, but PostgreSQL has stronger support for JSONB, indexing patterns, and common SaaS data modelling.

### SQLite

Useful for local development or embedded tools, but not appropriate as the primary control plane database.

### Document database

Rejected. The product has strong relational requirements and needs transactional consistency.

## Consequences

### Positive

- Mature and reliable.
- Excellent relational model.
- JSONB support for metadata.
- Strong indexing options.
- Good fit for SaaS and self-hosted deployments.
- Widely understood by target users.

### Negative

- Requires operational care for self-hosted users.
- Audit scale may require partitioning later.
- Multi-tenant SaaS design must be disciplined from the start.

## Revisit When

- Audit volume outgrows a straightforward PostgreSQL model.
- Customers require an embedded/offline mode.
- Multi-region active-active requirements emerge.

---

# ADR-0007: Use sqlc and Explicit SQL Instead of a Heavy ORM

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The database model will include security-sensitive queries, organisation scoping, audit event writes, execution lifecycle transitions, and access checks.

Hidden ORM behaviour could make performance and security harder to reason about.

## Decision

Use explicit SQL with sqlc-generated Go types.

## Alternatives Considered

### GORM or another Go ORM

Rejected for the MVP. It adds abstraction but can obscure query behaviour.

### Hand-written database scanning only

Viable but more verbose and error-prone than sqlc.

### Query builder

Viable for some dynamic search cases, but not necessary as the primary data access pattern.

## Consequences

### Positive

- SQL remains visible and reviewable.
- Strong type generation.
- Better control over organisation scoping and indexes.
- Easier to reason about audit and execution lifecycle writes.
- Good fit for performance-sensitive queries.

### Negative

- Dynamic query construction needs careful handling.
- Developers must be comfortable with SQL.
- More manual schema/query management than a full ORM.

## Revisit When

- Query complexity becomes too dynamic for sqlc alone.
- A subset of the app would benefit from a query builder.

---

# ADR-0008: Use Docker Compose as the First Self-Hosted Deployment Model

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The product needs a self-hosted open-source base edition. The first deployment model should be simple enough for small teams to run without Kubernetes.

## Decision

Use Docker Compose as the first supported self-hosted deployment model.

## Alternatives Considered

### Kubernetes / Helm first

Rejected for the MVP. Many target users do not need or want Kubernetes for a VPS management tool.

### Bare-metal packages first

Useful later, but more work across distributions and init systems.

### Single binary all-in-one server

Tempting for simplicity, but it does not match the eventual architecture of API, runner, database, and storage components.

## Consequences

### Positive

- Easy local development.
- Familiar to target users.
- Good fit for open-source self-hosted base.
- Keeps MVP deployment practical.
- Makes the stack explicit.

### Negative

- Not ideal for high availability.
- Requires users to understand container basics.
- Production deployment needs careful documentation.

## Revisit When

- Supported self-hosted customers require Kubernetes.
- HA deployment becomes a commercial requirement.
- Packaging for specific Linux distributions becomes important.

---

# ADR-0009: Use S3-Compatible Object Storage for Execution Artefacts

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

Execution output, logs, audit exports, diagnostic bundles, and later session recordings can become large. Storing all artefacts directly in PostgreSQL would be inefficient and awkward.

## Decision

Use S3-compatible object storage for large artefacts.

For self-hosted local deployments, use MinIO or compatible storage.

## Alternatives Considered

### Store all output in PostgreSQL

Rejected. It would increase database size and complicate retention and export management.

### Store files on local disk only

Rejected as the primary model. It is harder to scale, back up, and operate consistently across SaaS and self-hosted deployments.

### Use cloud-provider-specific storage only

Rejected. The self-hosted edition needs a portable model.

## Consequences

### Positive

- Works across SaaS and self-hosted models.
- Separates metadata from large artefacts.
- Easier retention and export handling.
- Enables future session recording storage.

### Negative

- More components to operate.
- Object permissions must be carefully managed.
- Output access needs signed URLs or proxying through the API.

## Revisit When

- Artefact volume requires tiered storage.
- Customers need external archive integrations.
- Regulatory retention requirements become a major product area.

---

# ADR-0010: Start with Agentless SSH Execution

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The MVP needs to onboard existing VPS servers quickly. Requiring an installed agent from day one would slow adoption and increase operational friction.

Most target customers already understand SSH and already have SSH access patterns.

## Decision

Start with agentless SSH execution through the runner.

## Alternatives Considered

### Agent-only model

Rejected for MVP. It would add installation, upgrade, connectivity, and security complexity before the product value is proven.

### Provider API execution model

Rejected. VPS providers vary widely, and provider-specific execution is not a portable foundation.

### Manual copy/paste scripts

Rejected. It would not provide a reliable controlled execution path.

## Consequences

### Positive

- Faster onboarding.
- Works with existing servers.
- Easier to demo and test.
- Does not require per-server daemon maintenance.

### Negative

- SSH credential handling is sensitive.
- Servers behind private networks require a customer-managed runner.
- Long-running or offline tasks are harder.
- Interactive session recording is more complex.

## Revisit When

- Customers need servers behind NAT without runner access.
- Long-running health reporting becomes important.
- A persistent agent becomes necessary for stronger security or usability.

---

# ADR-0011: Design Towards Short-Lived SSH Certificates

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The MVP may need to support practical SSH key-based access, but long-lived shared keys are not a good end-state for a security-focused product.

The product should move towards short-lived, policy-issued SSH credentials.

## Decision

Design the access model towards short-lived SSH certificates, even if the first MVP implementation supports simpler SSH credentials.

## Alternatives Considered

### Long-lived shared SSH keys

Rejected as the product norm. They are difficult to revoke, hard to audit, and unsafe for delegated operations.

### Per-user static SSH keys

Better than shared keys, but still weaker than short-lived certificates and harder to manage at fleet scale.

### Agent installed on every server

Useful later, but not the first access model.

## Consequences

### Positive

- Stronger long-term security posture.
- Supports just-in-time access.
- Better revocation and expiry model.
- Fits privileged access management expectations.

### Negative

- Requires server bootstrap to trust an SSH CA.
- More complex than basic SSH keys.
- Needs careful documentation and migration path.

## Revisit When

- The MVP access model becomes too dependent on static credentials.
- Terminal sessions become a first-class feature.
- Commercial customers require stronger privileged access controls.

---

# ADR-0012: Use a Customer-Managed Runner Model for Private Infrastructure Access

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

Many customer VPS servers will be private, firewall-restricted, or reachable only from trusted networks. SaaS cannot assume direct inbound access to customer servers.

## Decision

Support customer-managed runners that connect outbound to the control plane and execute jobs inside the customer’s network.

## Alternatives Considered

### SaaS-hosted runner only

Rejected. It would not work for private infrastructure and would raise security concerns.

### Require inbound access from the SaaS control plane

Rejected. This is a poor security posture and difficult for customers to accept.

### Agent installed on every server only

Rejected for MVP. More operational friction than a runner.

## Consequences

### Positive

- Works with private networks.
- Better customer security posture.
- Same model works for SaaS and self-hosted.
- Runner can be scoped by organisation, environment, project, or server group.

### Negative

- Runner lifecycle must be managed.
- Runner compromise must be considered seriously.
- More moving parts than pure SaaS.

## Revisit When

- Agent-based execution becomes part of the product.
- Regional SaaS runners are introduced.
- Enterprise customers require advanced runner isolation.

---

# ADR-0013: Keep the Runner Outside the Primary Authorisation Decision Path

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The runner is powerful because it can reach customer servers. If it also becomes the primary authorisation engine, a compromised or misconfigured runner could allow unauthorised operations.

## Decision

The control plane is the primary authorisation decision point. The runner executes only jobs already authorised by the control plane.

The runner may verify job signatures, scope, expiry, and target constraints, but it must not invent authority locally.

## Alternatives Considered

### Runner performs full authorisation locally

Rejected. Too risky and hard to keep consistent.

### CLI performs authorisation locally

Rejected. The CLI cannot be trusted as an enforcement point.

### Duplicate full authorisation in both API and runner

Rejected for MVP. Too complex and risks divergence.

## Consequences

### Positive

- Cleaner trust boundary.
- Centralised policy decisions.
- Easier audit consistency.
- Runner compromise impact is reduced.

### Negative

- Runner depends on control plane availability for new work.
- Offline execution is not supported in the MVP.
- The job signature/scope model still needs care.

## Revisit When

- Offline execution becomes a product requirement.
- Edge runners need more local autonomy.
- A formal distributed policy model is introduced.

---

# ADR-0014: Use a Simple Job Dispatch Model First, with NATS JetStream as the Scale Path

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The technical specification identifies NATS JetStream as a strong fit for durable job and event messaging. However, the first vertical slice should avoid unnecessary infrastructure complexity.

## Decision

Start with a simple job dispatch model for Phase 0 and early MVP, then move to NATS JetStream when execution scale, reliability, or runner topology justifies it.

Acceptable early implementation:

- API writes execution jobs to PostgreSQL.
- Runner polls for claimable jobs.
- Row locking prevents duplicate claims.
- Execution events are written back through the API or directly under controlled service credentials.

Target implementation:

- NATS JetStream for durable execution jobs and events.
- Scoped runner subjects.
- Better streaming and event handling.

## Alternatives Considered

### NATS JetStream from day one

Viable, but may slow Phase 0.

### Redis queues

Viable, but NATS is a better long-term fit for hybrid runner/event messaging.

### PostgreSQL-only forever

Rejected as the long-term model. It will eventually become limiting for distributed runners and event streaming.

## Consequences

### Positive

- Faster first vertical slice.
- Lower initial operational complexity.
- Clear scale path.
- Avoids premature queue abstraction work.

### Negative

- Migration to NATS must be planned.
- Early job dispatch may not represent final production topology.
- Polling is less elegant than event-driven dispatch.

## Revisit When

- Runner count grows.
- Execution event volume increases.
- Streaming progress becomes awkward.
- Job dispatch reliability needs improve beyond PostgreSQL polling.

---

# ADR-0015: Use OpenFGA for Relationship-Based Authorisation, but Not Necessarily in Phase 0

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

VPS Tools needs to model relationships between users, organisations, roles, servers, groups, runbooks, environments, and approval scopes.

OpenFGA is a strong fit for relationship-based authorisation, but introducing it before the first execution/audit vertical slice may slow progress.

## Decision

Use OpenFGA as the intended relationship authorisation system, but do not require full OpenFGA integration in Phase 0.

Phase 0 may use a simple internal authorisation stub, provided the data model and code boundaries are compatible with later OpenFGA integration.

## Alternatives Considered

### OpenFGA from the first spike

Viable, but may add complexity before the product’s core execution path is proven.

### Internal RBAC only forever

Rejected. Long-term relationships will become more complex than simple roles.

### Casbin or custom ACL engine

Viable alternatives, but OpenFGA better matches relationship-based access patterns.

## Consequences

### Positive

- Avoids blocking the first vertical slice.
- Preserves a strong long-term authorisation model.
- Allows relationships to evolve without hardcoding everything as simple roles.

### Negative

- Migration from internal checks to OpenFGA must be managed carefully.
- There is risk of building too much custom authz before OpenFGA arrives.
- Tests must ensure authorisation behaviour is preserved during migration.

## Revisit When

- Phase 4 RBAC and policy work begins.
- Team/project/server group relationships become necessary.
- Commercial customers require stronger access modelling.

---

# ADR-0016: Use an Internal Structured Policy Evaluator Before Adopting OPA

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

VPS Tools needs policy decisions based on role, action, environment, target, risk level, reason, approval state, time window, and command/runbook type.

OPA is powerful, but Rego and full policy-as-code may be too much for the MVP.

## Decision

Use an internal structured policy evaluator for the MVP. Design it so that OPA or another policy engine can be introduced later if policy requirements justify it.

## Alternatives Considered

### OPA/Rego from day one

Rejected for MVP. Powerful but likely to slow early development and make simple policies harder for users to understand.

### Hardcoded policy only

Rejected. Some structured configurability is needed.

### Full custom policy language

Rejected. Building a policy language is not a core MVP requirement.

## Consequences

### Positive

- Faster MVP implementation.
- Policies can remain understandable.
- Easier to provide clear denial messages.
- Avoids forcing customers to learn Rego early.

### Negative

- Internal evaluator may become too limited.
- Must avoid creating a poor custom policy language by accident.
- Future OPA migration should be anticipated.

## Revisit When

- Policy complexity exceeds simple structured rules.
- Customers need policy-as-code.
- Advanced compliance or enterprise controls become a priority.

---

# ADR-0017: Use OIDC-First Authentication for Production Deployments

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

VPS Tools is an access-control product. Authentication needs to integrate with existing identity providers and support MFA, SSO, and user lifecycle controls.

Building a full identity system would be a mistake.

## Decision

Use OIDC-first authentication for production deployments.

For local development and early self-hosted testing, a bootstrap/dev auth mode is acceptable, but production documentation should strongly recommend OIDC.

## Alternatives Considered

### Build native email/password auth as the primary model

Rejected. It creates security and account lifecycle work that existing identity providers already solve.

### SAML-first

Rejected for MVP. Important for some enterprise customers later, but OIDC is a better first target.

### SSH-key-only identity

Rejected. It does not solve organisation membership, web console, approvals, or audit identity properly.

## Consequences

### Positive

- Integrates with customer identity providers.
- Supports MFA through the IdP.
- Better fit for SaaS and self-hosted production deployments.
- Avoids building unnecessary IAM functionality.

### Negative

- Self-hosted setup is more complex.
- Local development needs a simple dev auth path.
- IdP integration edge cases must be handled.

## Revisit When

- Enterprise customers require SAML.
- Offline/self-contained deployments need local identity.
- User lifecycle requirements become more advanced.

---

# ADR-0018: Use Append-Only Audit Events as a Core Product Primitive

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

Auditability is one of the product’s core differentiators. It cannot be treated as ordinary logging.

The product must answer who did what, where, when, why, with what approval, and what happened.

## Decision

Use append-only audit events as a core product primitive.

Audit events must be created for sensitive actions, including:

- Authentication events where available.
- Server inventory changes.
- Runner registration and scope changes.
- Execution requested, started, completed, failed, cancelled, or denied.
- Runbook creation, publication, disabling, and execution.
- Approval requested, approved, denied, and expired.
- Policy changes.
- User and membership changes.
- Audit exports.

## Alternatives Considered

### Rely on application logs

Rejected. Logs are not structured or durable enough for product audit requirements.

### Build full event sourcing

Rejected for MVP. Too complex for current needs.

### Store only execution history

Rejected. The product also needs access, policy, approval, and inventory history.

## Consequences

### Positive

- Audit is part of the product model from day one.
- Stronger security and compliance posture.
- Easier incident review.
- Supports future reporting and commercial features.

### Negative

- Every sensitive workflow must be audit-aware.
- Audit storage and retention need planning.
- Event schemas need versioning discipline.

## Revisit When

- Audit volume requires partitioning or cold storage.
- Customers need tamper-evident cryptographic guarantees.
- Compliance packs become a major product feature.

---

# ADR-0019: Use OpenTelemetry for Observability

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

VPS Tools needs observability across API, runner, execution lifecycle, database operations, object storage, and job dispatch.

The product must work for SaaS and self-hosted customers without locking into a single vendor.

## Decision

Use OpenTelemetry for traces, metrics, and logs where practical.

Expose Prometheus-compatible metrics for self-hosted deployments.

## Alternatives Considered

### Vendor-specific observability SDK

Rejected. The product must support self-hosted and multiple customer environments.

### Logs only

Rejected. Execution lifecycle and runner behaviour need metrics and traces.

### Build custom metrics/logging protocol

Rejected. Not a core product requirement.

## Consequences

### Positive

- Vendor-neutral.
- Works for SaaS and self-hosted.
- Strong ecosystem.
- Supports future operational diagnostics.

### Negative

- Requires instrumentation discipline.
- Adds some complexity to local/self-hosted deployments.
- Poorly designed telemetry can become noisy.

## Revisit When

- Self-hosted users need simpler observability.
- SaaS operational needs require additional vendor-specific tooling.

---

# ADR-0020: Build the Open-Source Base Before Commercial-Only Extensions

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

VPS Tools will have a hybrid product model:

- Open-source self-hosted base edition with unlimited seats.
- Supported self-hosted commercial edition with full functionality.
- Hosted SaaS edition with paid manager/senior engineer seats and unlimited junior engineers.

The product should earn trust by making the open-source base genuinely useful.

## Decision

Build the open-source base product first, with licence and edition boundaries considered but not overbuilt.

Commercial-only extensions should come after the core product value is proven.

## Alternatives Considered

### Build SaaS first only

Rejected. The product strategy explicitly includes self-hosted and open-source adoption.

### Build commercial features first

Rejected. It risks delaying the core technical proof.

### Make the open-source edition a limited demo

Rejected. It would damage trust and adoption.

## Consequences

### Positive

- Builds developer trust.
- Supports community adoption.
- Keeps the MVP focused on core value.
- Reduces pressure to build billing early.

### Negative

- Commercial boundaries need discipline later.
- Some features may need refactoring into commercial modules.
- Open-core licensing needs legal review.

## Revisit When

- Commercial packaging is being finalised.
- Supported self-hosted customers are ready to pay.
- The licence strategy is legally reviewed.

---

# ADR-0021: Use Next.js and TypeScript for the Web Console

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The web console is needed for administration, approvals, inventory review, audit search, and later reporting. It is not the primary operational interface, but it must be usable and professional.

## Decision

Use Next.js and TypeScript for the web console.

## Alternatives Considered

### Go-rendered server-side web UI

Viable for simple admin screens, but less flexible for a modern SaaS console.

### Single-page React app without Next.js

Viable, but Next.js provides a stronger full-stack web application structure.

### No web console in MVP

Rejected. Approvals, audit review, and administration are easier with at least a minimal web interface.

## Consequences

### Positive

- Modern web development stack.
- Good TypeScript support.
- Works with generated TypeScript API clients.
- Suitable for future SaaS dashboard and reporting.

### Negative

- Adds a second language/runtime stack.
- Requires separate build and deployment process.
- Can expand scope if not controlled.

## Revisit When

- Web console scope remains extremely small.
- Maintenance burden exceeds value.
- A commercial UI framework or admin template is adopted.

---

# ADR-0022: Prioritise Direct CLI Workflows Before Rich TUI Workflows

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The TUI is important, but the product must remain scriptable and automation-friendly. Building rich screens too early can delay the core execution and audit path.

## Decision

Build direct CLI workflows first, then add TUI workflows around proven commands.

For example:

1. Build `vps server list`.
2. Build `vps exec`.
3. Build `vps run`.
4. Then build server browser, execution monitor, and runbook launcher.

## Alternatives Considered

### TUI-first development

Rejected. It risks polishing UI before the execution model is solid.

### No TUI in MVP

Rejected. The TUI is a meaningful differentiator for operator usability.

## Consequences

### Positive

- Keeps product scriptable.
- Reduces UX rework while backend semantics are changing.
- Faster first vertical slice.
- Easier automated testing.

### Negative

- Early demos may look less polished.
- TUI integration can lag behind direct command capability.

## Revisit When

- Core CLI workflows are stable.
- User testing shows the TUI should become the dominant interface for certain tasks.

---

# ADR-0023: Use YAML as the First Runbook Definition Format

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

Runbooks need to be human-readable, versionable, reviewable, and editable by DevOps users. They should work well in Git and support structured fields.

## Decision

Use YAML as the first runbook definition format.

Runbooks should have a formal schema and versioned API kind.

## Alternatives Considered

### JSON

Machine-friendly but less comfortable for human-authored operational documents.

### HCL

Good for infrastructure configuration, but less universal than YAML for this audience.

### Markdown with front matter

Useful for documentation, but less suitable as the primary execution schema.

### Database-only runbook builder

Rejected for MVP. Git-friendly files are important for technical users.

## Consequences

### Positive

- Familiar to DevOps users.
- Works well with Git workflows.
- Easy to read and review.
- Supports structured validation.

### Negative

- YAML parsing quirks can cause confusion.
- Schema validation must be clear.
- Templating must be tightly controlled to avoid injection risk.

## Revisit When

- Users demand GitOps-style runbook repositories.
- A richer workflow language is required.
- YAML complexity becomes a source of support issues.

---

# ADR-0024: Keep MVP Policy Simple and Deny-by-Default

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

The product manages privileged server operations. Unsafe defaults would undermine the product’s credibility.

The MVP needs enough policy to be safe, but not an enterprise policy language.

## Decision

Use simple structured policies in the MVP and make execution deny-by-default.

Default behaviour:

- Junior users cannot run arbitrary commands.
- Junior users can run explicitly permitted runbooks.
- Senior engineers can run authorised raw commands.
- Production actions require a reason.
- Risky actions can require approval.
- Auditors cannot execute operations.

## Alternatives Considered

### Allow-by-default for convenience

Rejected. This is incompatible with the product’s safety promise.

### Full policy-as-code from day one

Rejected for MVP. Too much complexity too early.

### Only hardcoded role permissions

Rejected. The product needs at least basic configurability.

## Consequences

### Positive

- Safer default posture.
- Clear junior/senior separation.
- Easier to explain to customers.
- Supports delegated operations model.

### Negative

- Initial setup may require more configuration.
- Users may hit denials until runbooks and policies are configured.
- Policy UX must be clear to avoid frustration.

## Revisit When

- Customers need advanced exception handling.
- Policy rules become difficult to express.
- Enterprise compliance requirements appear.

---

# ADR-0025: Defer Terminal Session Recording Until After the MVP

**Status:** Accepted  
**Date:** 18 May 2026  

## Context

Terminal session recording would strengthen the security story, especially for interactive SSH. However, it adds complexity around proxying, storage, playback, privacy, retention, and access controls.

The MVP can still prove the core value through controlled command/runbook execution and metadata/output audit trails.

## Decision

Defer full terminal session recording until after the MVP.

MVP should record:

- Session or command metadata.
- Actor.
- Target.
- Reason.
- Approval state.
- Command preview/hash.
- stdout/stderr/exit code for non-interactive execution.
- Start/end times.

## Alternatives Considered

### Build terminal recording into MVP

Rejected. It would increase scope and delay the first usable product.

### Do not support interactive SSH at all

Viable for MVP if necessary. Controlled command/runbook execution is more important.

### Rely on shell history

Rejected. Shell history is incomplete, unreliable, and not a product-grade audit trail.

## Consequences

### Positive

- Keeps MVP focused.
- Reduces storage and playback complexity.
- Avoids difficult privacy and retention questions early.
- Still supports strong audit for controlled executions.

### Negative

- Interactive SSH audit is weaker in the MVP.
- Some security-conscious customers may require session recording before adoption.
- Commercial supported edition may need this soon after beta.

## Revisit When

- Interactive SSH becomes a core workflow.
- Security-sensitive beta customers require recording.
- Supported self-hosted commercial edition is being scoped.

---

## 4. ADR Maintenance Process

### 4.1 When to Create a New ADR

Create a new ADR when a decision:

- Affects multiple components.
- Has meaningful security implications.
- Introduces or removes an important dependency.
- Changes deployment, data, API, or runtime architecture.
- Impacts open-source versus commercial packaging.
- Is likely to be debated later.

### 4.2 When Not to Create a New ADR

Do not create ADRs for:

- Small implementation details.
- Formatting preferences.
- Minor library choices that are easy to reverse.
- One-off bug fixes.
- Temporary experiments that will not ship.

### 4.3 ADR Format Template

```markdown
# ADR-XXXX: Title

**Status:** Proposed | Accepted | Superseded | Deprecated | Rejected  
**Date:** YYYY-MM-DD  

## Context

What problem are we solving? What constraints matter?

## Decision

What decision are we making?

## Alternatives Considered

What else did we consider, and why was it not chosen?

## Consequences

### Positive

- Benefit 1.
- Benefit 2.

### Negative

- Trade-off 1.
- Trade-off 2.

## Revisit When

When should this decision be reviewed?
```

---

## 5. Near-Term ADRs Still Needed

The following ADRs should be added once the implementation gets closer to those decisions:

1. Final licence strategy for open-source and commercial editions.
2. Exact SSH credential storage model for MVP.
3. Exact runner job signing mechanism.
4. Exact object storage access model for output retrieval.
5. Exact audit hash-chain or tamper-evidence strategy.
6. Exact local development identity provider approach.
7. Exact production SaaS hosting provider and region strategy.
8. Exact container base image and hardening standard.
9. Exact web console component library, if one is chosen.
10. Exact versioning policy for runbook schemas and protobuf APIs.

