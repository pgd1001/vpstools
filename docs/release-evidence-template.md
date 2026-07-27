# Release candidate evidence

Copy this file into the release evidence directory and complete it for every candidate. A green code test suite is necessary, but it does not replace the deployment checks below.

## Candidate

- Version:
- Git commit:
- Build date:
- Operator:
- Environment:

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

- Accepted residual risks:
- Rollback version:
- Approver:
- Decision, pilot / release / hold:
