# Release candidate evidence

Copy this file into the release evidence directory and complete it for every candidate. A green code test suite is necessary, but it does not replace the deployment checks below.

## Candidate

- Version, exact release tag:
- Git commit, full SHA:
- Build date:
- Operator:
- Environment:
- CI run URL:
- Checksums file, path or URL:
- SBOM document(s), path or URL:

The validator requires every field above to contain a candidate-specific value.
Do not use `TBD`, `N/A`, or `not run` for a supported release.

## Environment-only gates

These checks cannot be marked complete from repository CI. A checked status must
include a link or path to the captured evidence in the same row.

| Gate | Status | Evidence |
|---|---|---|
| Target-host acceptance output | [ ] | |
| Clean-machine installation | [ ] | |
| Backup and restore result | [ ] | |
| Measured RPO and RTO | [ ] | |
| Rollback result | [ ] | |
| Identity-provider verification | [ ] | |

Record the detailed result fields below as well. The checked rows are a quick
review aid, not a substitute for the result and evidence path.

- Target-host acceptance output:

## Automated checks

- [ ] `go test ./... -race -count=1`
- [ ] `go vet ./...`
- [ ] `npm run build` in `apps/web`
- [ ] `npm run check` and `npm run smoke` in `mcp`
- [ ] `make release-check`
- [ ] `scripts/self-contained-smoke.ps1` or the equivalent Linux smoke test, including runbook, automation, MCP, backup, replication, restore, and post-restore history checks
- [ ] Browser and TUI workflow tests
- [ ] Documentation examples executed against this candidate

## Backup and restore

- Backup and restore result:
- Backup timestamp:
- Backup manifest verified:
- Replicated backup location:
- Restore rehearsal start, UTC:
- Restore rehearsal end, UTC:
- Measured RTO:
- Measured RPO:
- Encryption key recovery source checked:
- Readiness after restore:
- Execution, artefact, and audit reconciliation result:
- Audit hash-chain verification result and checked event count:
- Evidence links:

## Monitoring and incident response

- Prometheus scrape configured:
- Alert routing configured:
- API outage alert tested:
- Readiness alert tested:
- Backup failure alert tested:
- Runner failure and lease recovery tested:
- Automation pause and resume tested:
- Incident owner and escalation path:

## Release decision

- Rollback result:
- Identity-provider verification result:
- Accepted residual risks, or `None`:
- Rollback version:
- Approver:
- Decision, pilot / release / hold:
