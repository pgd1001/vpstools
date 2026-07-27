# Security policy

VPS Tools controls infrastructure operations. Please do not disclose a suspected vulnerability in a public issue or include production credentials, hostnames, command output, or artefacts in a report.

## Reporting

Use a private GitHub security advisory for this repository, or contact the repository owner through the GitHub account that owns `pgd1001/vpstools`. Include the affected commit or release, a concise reproduction, impact, and any suggested mitigation. Redact secrets from all attachments.

If a report concerns a live customer deployment, contain the exposure first, rotate affected credentials, preserve the relevant audit evidence, and then report the vulnerability privately.

## Supported releases

Only the current `master` branch and the latest published release are expected to receive security fixes. Older releases should be upgraded after validating the documented backup and rollback procedure.

Security fixes must pass the normal race-enabled tests, vulnerability scan, packaged self-contained smoke tests, and release evidence checks before publication.

## Deployment expectations

- Set `VPS_ENV=production` and disable development identity headers.
- Use OIDC for the web console and short-lived bearer tokens for CLI, SDK, and automation clients.
- Keep runner credentials, artefact encryption keys, API tokens, and OIDC secrets outside source control.
- Protect backup copies and verify restore and key recovery before relying on them.
- Do not expose runner health or metrics endpoints beyond the protected monitoring path.
