# Initial implementation review and verification

Date: 2026-10-15.

Scope: setup/configuration and one-shot HTTP checks. This is not a completed monitoring daemon.

## Independent source review before execution

Three implementation subagents had separate file ownership: configuration, HTTP checks, and setup. The coordinating agent integrated the CLI and documentation. Review assignments crossed ownership: setup author reviewed configuration/CLI, configuration author reviewed HTTP/setup, and HTTP author reviewed configuration/setup.

All source reviews and fixes below completed before the first project build, test or CLI execution. Reviewers reported no remaining blocking findings in their reviewed areas.

| Finding | Resolution |
| --- | --- |
| Redirects could leak a query secret in Go-generated Referer | Strip Referer and configured headers after crossing origins; regression test |
| Case-insensitive duplicate header names selected values nondeterministically | Reject canonical-name collisions |
| YAML null could silently retain an existing default | Reject null except explicitly supported JSON assertion values |
| Explicit YAML scalar tags could bypass intended type/value semantics | Validate boolean/null spellings and integer syntax |
| Configuration body limit differed from checker limit | Align at a maximum of 16 MiB |
| Numeric notation could be interpreted differently by parser and checker | Decimal-only configuration numbers, base-10 integers and exact bounded comparisons |
| Empty environment credentials were accepted | Fail selected-header resolution for missing or empty values |
| Invalid init flag combinations used an operational exit code | Return usage exit code 2 |
| Speculative runtime-wide secret resolver was unused | Remove it; selected check resolves only its own headers |

The real systemd installer remains gated because the daemon does not exist yet. Dry-run is explicitly a future plan, not a report of completed installation.

## Verification results

- `go test ./...`: passed all four packages, including local HTTP/TLS fixtures. The sandbox initially denied loopback listeners; the suite passed when allowed to bind local test ports.
- `go vet ./...`: passed.
- Linux ARM64 build: passed.
- Linux AMD64 build with `CGO_ENABLED=0`: passed; cross-built binary was not executed.
- Built-binary smoke checks: version, configuration generation, validation, preservation of existing files, install dry-run, gated real installation and invalid init flags passed. Generated files were confined to a temporary directory.
- `go test -race ./...`: attempted, but the verification environment's ThreadSanitizer runtime exits before tests with `unsupported VMA range`. Race verification remains outstanding on a supported host; this is not a successful race-test result.

No system service was installed/enabled/started, no Docker settings or permissions were changed, and no Telegram messages were sent. Tests used local disposable fixtures.

## Remaining validation

The current tests do not validate a daemon, dashboard, Telegram integration, Docker collector, history store, resource budgets or prolonged Pi operation: those features remain to be implemented. Run the race detector on a compatible host before claiming race-clean release validation. Select a project license and audit dependencies before publishing.

## Follow-up review fixes (2026-10-15)

An independent subagent reviewed all six changed Go source/test files before
execution and reported no blocking findings. The fixes and regression tests cover:

- Rejection of fractional, floating-point and quoted values for integer YAML
  settings, including schema version, Telegram IDs, replica counts and HTTP limits,
  status and alert counts.
- Rejection of invalid UTF-8 anywhere in a JSON response, while accepting valid
  Unicode, including the replacement character.
- Exit code 2 and a structured configuration failure for missing or empty HTTP
  header credentials, without sending a request.
- Independent Telegram notification destinations and command authorization lists,
  while preserving rejection of invalid IDs. Telegram runtime remains unimplemented.

`go test ./...` passed all four packages after allowing disposable loopback HTTP/TLS
fixtures; the sandbox initially blocked listener creation. `go vet ./...` passed.
Dependency downloads were disabled. No service was installed or started and no
Telegram request was made. The race detector was not rerun; the previously recorded
environment limitation remains outstanding.

## Readability and publication review (2026-10-15)

An independent subagent compared all Go source and tests with the saved source
before the refactor, approved the changes and reported no blocking findings before
any project execution. Internal names now distinguish request headers, expectations,
alert policy, recovery conditions, credential references and result timing. YAML
keys, JSON output fields, CLI flags, defaults and exit behavior remain unchanged.

Parsing/defaults, YAML node handling and typed validation have separate files.
HTTP transport/redirect policy, JSON parsing and JSON assertions are grouped by
responsibility. CLI commands have dedicated handlers, and configuration generation
uses named options. Fixed regular expressions compile once; body comparisons and
JSON decoding avoid body-sized string copies. No measured speedup is claimed.

Verification after review:

- `go test ./...`: passed all four packages with disposable loopback fixtures.
- `go vet ./...`: passed.
- `gofmt -l cmd internal`: no unformatted Go files.
- Linux ARM64 and AMD64 builds with `CGO_ENABLED=0` and `-trimpath`: passed.
  The binaries were not executed. Personal source-path patterns were absent from
  both new artifacts.
- The race detector was not rerun; the previously recorded environment limitation
  remains unresolved.

Dependency downloads were disabled during verification. No service was installed
or started, Docker permissions were not changed, and Telegram was not contacted.
See [release readiness](release-readiness.md) for the privacy audit scope, the older
ignored binaries that must not be published, and outstanding publication work.

## Release preparation follow-up (2026-10-15)

An independent subagent reviewed all existing Go source/tests and the new header
validation, regression test, CI, packaging script, usage documentation and notices
before execution. Review findings were resolved: packaging now propagates tar
failures before gzip and includes documents referenced by the packaged README.
No blocking findings remained.

Environment-derived HTTP header values now reject invalid control bytes before
transport. A regression test confirms configuration failure, no outgoing request
and no credential disclosure. Existing YAML, JSON and CLI contracts are preserved.

Verification after review:

- `go test ./...`: passed all four packages with disposable loopback fixtures.
- `go vet ./...`: passed; Go formatting and shell syntax checks passed.
- Race verification was retried and failed before tests with `unsupported VMA
  range`; it remains unverified. The configured GitHub workflow has not run here.
- Packaging built Linux ARM64 and AMD64 twice in an isolated source copy with an
  explicitly fake LICENSE fixture. Both archive pairs were byte-for-byte identical,
  checksums verified, and archive members included the documented notices/examples.
- The packaged ARM64 binary passed version, init and validate smoke checks. The
  AMD64 binary was cross-built and not executed. Fixture packages are not releases.
- Packaging in the actual source tree without LICENSE rejected the request before
  output creation. No project license was chosen by this work.

The sandbox initially blocked the default build cache and loopback listeners;
verification used an isolated writable cache and permission for local fixtures.
The temporary filesystem lacked build space, so later build artifacts used ignored
storage outside that filesystem. These failures were environmental, not passed
checks. No service was installed/started and no Telegram endpoint was contacted.

Publication inspection included new shared files, hidden workflow files and
available Git history. All project files remain untracked with zero reachable
commits; staged contents and future commit metadata still require a final audit.
Pattern inspection found only intentional documentation references to personal-path
patterns, not actual operator paths in shared files. This scoped inspection is not
proof of absence of every secret. No repository or artifacts were published.

## Foreground monitoring and Telegram (2026-10-15)

Independent agents reviewed all project source/tests and the new host collector,
monitoring runtime, Telegram client, CLI integration and setup documentation before
execution. Telegram code was reviewed by the host agent; the Telegram agent
reviewed the other implementation scopes. Findings resolved before execution:
Telegram identity discovery now retries independently of collection; polling
honors rate-limit delays and skips duplicate updates; command authorization rejects
anonymous sender-chat identities; scheduling rotates through due checks; missing
or stale observations break pending failure/recovery continuity.

Verification used Go 1.24.4 and a temporary writable build cache. Formatting and
`go vet ./...` passed. Linux ARM64 and AMD64 builds with `CGO_ENABLED=0` and
`-trimpath` passed. Binaries were written outside the source tree; existing local
binaries were preserved. The race detector failed before test execution with
`ThreadSanitizer: unsupported VMA range` (47 found, 48 supported); race behavior
remains unverified on this environment.

The sandbox blocked local fixture listeners. An escalated test run using only
loopback fixtures passed config, host, HTTP checks, monitor, setup and CLI packages,
but exposed a Telegram cancellation fixture cleanup hang. The test fixture was
subsequently corrected and independently reviewed before rerunning verification.

The final `go test -timeout 30s ./...` run passed all seven packages with disposable
loopback fixtures. The race detector limitation above remains unresolved.

No service was installed or started, Docker permissions were not changed, and no
Telegram API endpoint was contacted. Live bot delivery, real Pi sensors, unattended
operation and resource budgets remain unverified. State and pending notifications
are memory-only. Filesystem probes can block on unresponsive remote mounts; use
local Pi filesystems. No repository or binaries were published. Available Git
history has zero reachable commits; all shared project files remain untracked.

## Telegram setup diagnostics (2026-10-15)

Enabled Telegram configuration now reports the specific missing ID list or invalid
environment variable name instead of a combined requirements error. Regression
coverage checks field identification and that an invalid token-like value is not
included in the error. Setup documentation distinguishes local generation from
explicit `/etc/pi-monitor/config.yaml` output and explains token environment names.

An independent subagent reviewed source and tests before execution and found no
blocking issues. `go test ./...` passed all seven packages with disposable local
HTTP fixtures; `go vet ./...` passed. Verification used a temporary build cache
because the default cache is read-only in the sandbox. Tests required permission
for loopback listeners. The race attempt could not complete: the environment
reports `ThreadSanitizer: unsupported VMA range`; race verification remains open.
No service was installed or started and no live Telegram API was contacted.
Interactive initialization and token-file loading are not implemented by this change.

## Automatic credential loading (2026-10-15)

`run` and `check` now load a private regular `secrets.env` beside the selected YAML
configuration on each process launch. Only credentials referenced by the command
are imported; explicit process environment values, including empty values, win.
The bounded literal-assignment parser does not invoke a shell or expand variables.
Invalid existing files fail before runtime network requests; `validate` remains
independent of secrets. Generated comments and setup documentation describe the
new behavior. Interactive initialization and automatic service installation remain
unimplemented.

Independent source review identified a FIFO replacement race in the first opening
implementation. It was fixed with nonblocking, no-follow opening and descriptor
validation, covered by a regression test, and independently re-reviewed before any
project execution. Review found no remaining blocking issues. Other regression
tests cover repeated startup loading, lookup beside the config rather than in the
working directory, process environment precedence, referenced-variable filtering,
permissions, symlinks, file size, malformed syntax, redaction and CLI integration.
Telegram token loading is tested through the unsupported-feature guard before any
Telegram client is created; HTTP integration uses disposable loopback fixtures.

`go test -timeout 30s ./...` passed all seven packages and `go vet ./...` passed.
`go build -trimpath -o bin/pi-monitor ./cmd/pi-monitor` rebuilt the local binary.
The race check again stopped before running tests with `ThreadSanitizer: unsupported
VMA range`; race verification remains unresolved. A temporary build cache and
permission for local fixture listeners were used. No live Telegram API was
contacted and no service was installed or started. No release was published.

## Optional Telegram host reboot (2026-10-17)

Independent source review found no blocking issues before project execution.
Review covered strict opt-in configuration, chat/user authorization, fresh
message timestamps, expiring random confirmation tokens bound to the requesting
chat/user, single-use and restart-invalidated confirmations, worker cancellation
and shutdown, fixed systemctl arguments, and the optional narrow logind policy.
The supplied service retains `NoNewPrivileges=true`.

`go test -p 2 -timeout 30s ./...` passed all seven packages and
`go vet -p 2 ./...` passed with Go 1.24.4. Formatting checks and
`make -n install-reboot-policy` passed. Reboot tests use a fake action; the
production command is only constructed and inspected, never executed by tests.
Telegram and HTTP tests use disposable loopback fixtures. Build/cache files were
moved to disk after the initial attempts exhausted temporary-filesystem space;
the passing suite required permission for local fixture listeners.

`go test -race -p 2 -timeout 30s ./...` stopped before test execution with
`ThreadSanitizer: unsupported VMA range` (47 found, 48 supported). Race behavior
remains unverified in this environment. No real reboot, live Telegram request,
service installation/start or policy installation was performed. Actual target
systemd/logind/polkit authorization and reboot completion remain unverified.
Changed shared files were inspected; no publication or full repository privacy
audit was performed. Available Git history still has no reachable commits.
