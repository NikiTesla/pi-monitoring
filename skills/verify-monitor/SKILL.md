---
name: verify-monitor
description: Review and verify pi-monitoring changes using isolated fixtures, including config, runtime, credential, deployment and release boundaries. Use for substantive changes or publication-readiness audits in this repository.
---

# Verify pi-monitoring

Read the root AGENTS.md, CONTRIBUTING.md and docs/architecture.md. Select checks
for the changed behavior; this skill does not authorize live deployment or
publication. This repository-local skill is linked from AGENTS.md; an agent can
read this file directly without installing it globally.

## Review before execution

Before project code, tests, builds or scripts run, obtain independent subagent
source review. Formatting and source inspection may happen first. Give the
reviewer the diff, relevant callers/tests and execution restrictions. Ask for
concrete correctness/security findings and whether verification is safe. Resolve
findings and have fixes reviewed before execution. If subagents are unavailable,
complete source inspection and report the missing review; do not bypass it.

## Choose fixtures and checks

- Config/assertions: unknown/duplicate fields, omitted versus explicit values,
  scalar precision, invalid and oversized responses. Keep validate offline.
- Credentials/HTTP: fake secrets, private temp files, replacement/symlink cases,
  local redirect/TLS fixtures and diagnostics that do not echo credentials.
- Runtime: no overlapping checks, saturation as unknown, stale data breaking
  streaks, independent delivery retries and cancellation/joining of workers.
- Telegram: loopback Bot API fixtures, chat and sender authorization, rate limits
  and token redaction. Reboot tests must use fake actions; constructing and
  inspecting the fixed command is allowed, executing it is not.
- Deployment: inspect paths, permissions and preservation of existing config,
  unit and policy files. Use dry-run or an isolated fake-command harness;
  never install/start a service or change OS authorization for verification.
- Release: read docs/releasing.md. Build in fresh temporary directories, inspect
  archive members/notices, compare repeat builds and verify checksums. Never
  package operator config or reuse ignored binaries.

After review, use repository formatting, tests and vet. Run race checks for
concurrency or release work and cross-build supported architectures when relevant.
Use a supported patched compiler for release/vulnerability checks; the go.mod
version is only a compatibility floor. A sandbox blocking local fixture listeners
or a race runtime failure is a verification limit, not a reason to weaken tests.

For modernization work, inspect `go fix -diff ./...` with Go 1.26+ after source
review, and verify changes with the minimum compiler too. Check loop declaration
scope, nil versus empty slices, aliasing and test cleanup order; an equivalent
standard-library operation should preserve the relevant contract. New APIs must
fit go.mod rather than silently increasing the minimum supported compiler.

For publication, inspect tracked/added/hidden files and available history including
author metadata. Avoid printing secret matches. Report scope and limits; scans
cannot prove absence of all private data. Link SECURITY.md and release-readiness
for outstanding hosting steps. Report only checks that actually completed.
