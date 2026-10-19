# Review and verification

## Go modernization — 2026-10-19

Independent source review checked Go 1.24 API availability, slice ownership,
authorization, message output and test cleanup before verification. No obsolete
loop-capture rebindings were found. Actual mutable fixture copies remain necessary.

Replaced local collection operations with slices/maps APIs; used string iterators,
CutPrefix/CutSuffix, bounded min/max clamping and a message builder. Tests use
t.Context while retaining explicit cancellation and worker joins. The Go 1.24
minimum, public configuration and CLI/result contracts are unchanged. CI now
checks `go fix -diff ./...` on stable Go; contributor/agent guidance explains
version-aware modernization and preservation of behavior.

Verification passed:

- All seven packages on Go 1.24.4 and Go 1.27.2.
- Go 1.27.2 vet and modernization analysis; the modernization diff is empty.
- Linux AMD64 and Darwin ARM64 cross-builds, including the non-Linux collector.
  Cross-built binaries were not executed.
- Formatting and `git diff --check`.

An initial supported-compiler run hit the TLS rejection fixture's one-second
deadline during parallel compilation, producing a timeout rather than proving
certificate rejection. After independent review, that fixture uses a bounded
five-second deadline and a precise diagnostic; the certificate-rejection and
cancellation assertions remain intact. The final suites passed on both compilers.
Dedicated short-timeout coverage is unchanged.

Race testing was not repeated: the host limitation recorded below still applies.
The packaging/vulnerability results below predate this refactor; dependencies and
packaging code did not change. Rebuild final release artifacts and complete hosted
CI before publication. No service, live Telegram or real reboot was exercised.

## Portfolio-readiness review — 2026-10-19

Independent source review inspected all production packages, tests, Makefile,
systemd/polkit configuration, packaging and CI before any project execution.
Review confirmed that tests use temporary files, loopback HTTP/TLS fixtures and
fake reboot actions; no test executes a host reboot or contacts live Telegram.

Findings addressed:

- Installer targets now refuse differing existing unit/policy files before
  installation mutations, preserving operator customizations for manual review.
- Release archives now contain the optional reboot policy and SECURITY.md.
  Documentation distinguishes source-checkout and extracted-binary workflows.
- Validation rejects host/HTTP durations whose freshness arithmetic would overflow;
  boundary cases cover both the largest accepted values and one-nanosecond excess.
- MIT licensing, the confirmed GitHub module path and the privacy-audit scope now
  reflect the actual repository, including its one reachable commit.

The service drops Linux capabilities and restricts socket families, set-ID,
realtime and personality operations while preserving the host mount view.
Live target-host compatibility remains unverified. CI pins actions to verified
official release commits and adds supported-toolchain vulnerability checks.

The final source pass confirmed that all findings were resolved before execution.
Separate read-only reviews also cleared the temporary installer/package harnesses.

| Verification | Result |
| --- | --- |
| `go test -p 2 -timeout 30s ./...`, Go 1.24.4 | All seven packages passed |
| Same fixture suite, Go 1.27.2 | All seven packages passed |
| `go vet -p 2 ./...`, Go 1.27.2 | Passed |
| `go mod verify`, Go 1.27.2 | All modules verified |
| `govulncheck` v1.9.0, Go 1.27.2 | No known vulnerabilities found on the audit date |
| `go test -race -p 2 -timeout 30s ./...`, Go 1.27.2 | Runtime stopped before tests: unsupported VMA range (47 found, 48 supported) |
| Formatting, `git diff --check`, packaging shell syntax | Passed |
| `systemd-analyze verify deploy/pi-monitor.service` | Passed; unit was not installed or started |
| Installer targets, dry-run and isolated fake-command harness | Conflicting files preserved before mutation; identical files passed preflight |
| Linux AMD64/ARM64 packaging, Go 1.27.2 | Both built twice; corresponding archives were byte-identical and checksums passed |
| Archive contents | Required docs, skill, policy, unit and notices present; normalized ownership and safe paths; local source path absent from binaries |
| Distributed notices | Match the actual Go 1.27.2 toolchain and YAML v3.0.1 dependency |
| Packaged ARM64 binary | Version, init, offline validate and loopback JSON check passed |
| Repository skill validator and local Markdown link targets | Passed |

Packaging used `v0.1.0-verify` and fresh ignored output directories. These are
verification artifacts, not a published release. AMD64 was cross-built but not
executed. Dependencies/toolchains and Go temporary/cache files used ignored local
storage because the environment's temporary filesystem was full. Local fixture
listeners and the read-only systemd verifier needed sandbox exceptions. No
verification failure was bypassed or relabeled as success.

The local race limitation remains a gate for hosted CI on the reviewed revision.
No hosted run or GitHub repository setting was inspected or changed. Privacy
inspection covered 68 tracked/new publication files and one reachable commit;
no apparent live secrets were found, and author-email exposure remains a decision
before public visibility. See [readiness](release-readiness.md) for audit limits.

## Earlier verification

Previous source-reviewed revisions passed local fixture tests and go vet using
Go 1.24.4. The ARM64 race runtime stopped before tests with
`ThreadSanitizer: unsupported VMA range` (47 found, 48 supported). That was an
environment limitation, not a passing race check. Earlier packaging verification
used an isolated fake license and is not release evidence for this revision.

No real reboot, live Telegram request, service installation/start, policy
installation or publication was performed. Host acceptance and long-running
resource measurements remain separate work. See [readiness](release-readiness.md)
for current publication gates and privacy-review limits.
