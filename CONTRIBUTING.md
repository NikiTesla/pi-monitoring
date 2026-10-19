# Contributing

This is an early implementation of YAML configuration, setup planning, HTTP
checks, Linux host metrics and Telegram alerts/commands. Read
[the design](docs/design.md) and [implementation status](docs/implementation.md)
before changing behavior. [Architecture and decisions](docs/architecture.md) maps
current packages and ownership. Persistent state, Docker and the dashboard remain future work.

## Naming and readability

Choose names that say what a value means in its domain. Use nouns for data and
verbs for actions: `RequestHeader`, `RecoveryCondition`, `resolveJSONPointer`.
Include units when they are not obvious, such as `MaxBodyBytes` and `ElapsedMS`.
Use `check`, `heartbeat`, `assertion` and `settings` in longer scopes rather than
single-letter stand-ins. Keep conventional names such as `ctx`, `err`, `id` and
small loop indices when their meaning is immediate. Avoid adding `Manager`,
`Helper`, `Data` or `Info` unless that word states a real responsibility.

Use named options when several boolean arguments would obscure a call. Keep Go
names descriptive while preserving established YAML keys, JSON fields and CLI
flags. Review an externally visible rename as a compatibility change.

Prefer concrete types, explicit branches, early error returns and the standard
library. Extract a function when it gives a coherent operation a useful name or
reduces nesting. Keep closely related code together; split a file when it helps a
reader find a responsibility. Do not build generic frameworks, interface layers
or future extension points without a current caller that needs them.

Separate parsing, defaults and validation so omission is distinguishable from an
explicit invalid value. Preserve scalar types and numeric precision. Keep limits,
timeouts, redirect rules, file permissions and failure categories visible. Comments
should explain intent or a constraint; remove comments that only repeat the code.
Format Go code with `gofmt` and separate standard library and project/dependency
imports. Prefer simple allocation improvements to clever code; measure before
making performance claims.

Use language and standard-library facilities available at the go.mod minimum.
Prefer slices/maps operations to bespoke membership, copying and sorting loops
where clearer. Go 1.22+ iteration variables declared with := need no capture
rebinding; preserve actual fixture/state copies and explicitly reused variables.
Use t.Context for test work and retain early cancellation/joining for workers.
With Go 1.26+, `go fix -diff ./...` provides a modernization review; inspect its
suggestions for behavior and compatibility before applying them. Newer APIs such
as WaitGroup.Go require a separate decision to raise this project's Go 1.24 floor.

CI runs the modernization diff check on stable Go. It does not edit files; review
any suggested changes and test them at the declared minimum compiler version.

## Verification

Project instructions require independent subagent source review before any project
code, tests, build, installer or application is executed. Formatting and source
inspection are allowed before that review. Resolve findings before execution.

After review, run:

```sh
go test ./...
go vet ./...
go test -race ./...
```

Use temporary directories and disposable loopback HTTP/TLS fixtures. Do not use a
live endpoint, install or start a system service, modify Docker permissions, or
contact Telegram during development verification. If an environment prevents a
check, record that limitation rather than claiming success. Test behavior and
failure boundaries; do not add tests merely to mirror a refactoring.

## Privacy and publication

Examples use placeholders and loopback addresses. Keep operator configuration,
credentials, personal IDs, private hostnames, LAN addresses, home directories and
machine inventory out of shared files. Standard product installation paths such
as `/etc/pi-monitor` are intentional defaults. Keep local overrides and credentials
in ignored files; ignore rules do not protect files that are already tracked.

Before publishing, inspect the actual files and available Git history, including
filenames and commit metadata. Redact diagnostics without losing the failure
category. Document audit scope and avoid claiming that a pattern scan proves the
absence of every secret. Preserve dependency attribution when distributing code or
binaries. Check [release readiness](docs/release-readiness.md) before a release.

Build shared binaries with `go build -trimpath` so compiler-recorded source paths
do not expose a developer's home directory. Publish freshly built release artifacts,
not ignored local binaries or development logs.
