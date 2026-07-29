# Versioning and release tracking

VPS Tools uses two different version numbers. The product release version describes the CLI, API, runner, backup utility, and packaged deployment. A runbook version describes one runbook definition. They have different lifecycles and should never be treated as interchangeable.

## Current baseline

The repository baseline is `0.1.0-beta.1`. It represents the implemented MVP after Phases 0 through 7 and the later production, backend, recovery, and AI additions. The project is still a beta because some production evidence, hosted identity flows, independent queue operation, and enterprise-scale operations remain outside the supported boundary.

The product version is kept in the root [`VERSION`](../VERSION) file and the release history is in [`CHANGELOG.md`](../CHANGELOG.md). The version is also the default for source builds. GoReleaser replaces it with the exact Git tag during a packaged build.

## Semantic Versioning policy

- Patch releases fix defects, documentation errors, packaging problems, or security issues without changing the supported API or operational model.
- Minor releases add backward-compatible features, new commands, new API capabilities, or supported deployment extensions.
- Major releases may remove or change commands, API contracts, configuration semantics, data migration behaviour, or security boundaries.
- While the version is below `1.0.0`, minor releases may still include carefully documented breaking changes. Any such change must be called out in the changelog and migration guidance.

## Release workflow

1. Add user-visible changes to the `Unreleased` section of the changelog.
2. Update the relevant guides and known limitations in the same change.
3. Run `go test ./...`, `go vet ./...`, the web build, MCP checks, and the release checks in the CI workflow.
4. Update `VERSION`, move the release notes to a dated heading, and verify the exact version in `vps version` and `/api/v1/health`.
5. Create an annotated tag using the `v` prefix, for example `v0.1.0-beta.1`, and let the release workflow build the packages.
6. Record the release evidence, backup and restore result, rollback result, and any deployment-specific limitations before publishing.

## Version sources

| Item | Version source |
|---|---|
| Local source builds | `VERSION` and the default values in the Go entry points |
| GoReleaser packages | Git tag, injected with linker flags |
| CLI | `vps version` |
| API | `GET /api/v1/health`, field `version` |
| Runbooks | `metadata.version` in each runbook definition |

The first beta tag has not been created by this documentation change. The repository is ready for that tag once the release checks and deployment evidence have been accepted.
