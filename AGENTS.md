# Working on pi-monitoring

Read docs/design.md, docs/architecture.md and docs/implementation.md. The design
includes future behavior; the architecture describes current code. Prefer explicit
code, the standard library and small concrete types. Do not introduce frameworks
or speculative interfaces.

Current implementation includes strict YAML config, init/config generation, systemd installation planning, validate, one-shot check, version, and a foreground monitoring runtime with HTTP scheduling, Linux host metrics and Telegram alerts/commands. Makefile targets install/enable the supplied systemd unit and start it separately. Dashboard, Docker, heartbeats, persistent state and CLI service installation remain later work and must not be represented as implemented.

The user requires independent subagent source review before any project code, tests, build, installer, or application is executed. Formatting and source inspection are allowed before review. Review findings must be resolved before execution. Do not install/start a service or contact Telegram during development verification.

Agents own their assigned files. Do not overwrite other agents' work. New tests should verify observable behavior, validation, security boundaries or failure handling. Use temporary directories and local HTTP fixtures; no live services.

## Change workflow

- Read CONTRIBUTING.md and use the readable-code skill (including its Go reference)
  when available for substantive implementation or review.
- Preserve YAML keys, CLI flags, JSON fields and defaults unless a behavior change
  is explicitly part of the task. Separate compatibility changes from cleanup.
- Trace behavior from CLI through validation to runtime before editing. Keep
  credential resolution out of offline validation and notification out of check
  evaluation. Do not log raw URLs, headers, bodies or Bot API errors.
- The monitoring event loop owns incident state. Workers return observations;
  cancellation and joins belong to monitor.Run. Bound new queues and give every
  goroutine an owner and shutdown path.
- Use skills/verify-monitor/SKILL.md for verification of runtime, config,
  credentials, deployment and release changes. It lists relevant checks and
  the independent review handoff; it does not authorize deployment or publishing.
- Update README/usage when user behavior changes, architecture when ownership or
  technical decisions change, and implementation status when scope changes.
- After resolving independent review findings, run relevant fixtures, go vet and
  race checks for concurrency changes. Record compiler, outcomes and actual limits
  in docs/review.md for release-readiness work. Do not claim hosted CI passed
  without inspecting a hosted run.

## User experience and performance

The current interfaces are the CLI and Telegram. Keep errors actionable, exit
codes stable, status text readable without emoji/color, units explicit and stale
observations distinct from health. Keep dangerous actions opt-in with clear impact
and confirmation. Do not add a dashboard to satisfy a cosmetic request.

Target small personal-server configurations. Preserve bounded HTTP execution,
body/depth limits and delivery coalescing. Profile an observed workload before
adding caching, pools or new dependencies; do not invent benchmark claims.

## Readable code and naming

Follow CONTRIBUTING.md. Name domain entities and operations by their meaning and
intent; use descriptive names across long scopes and units where ambiguous.
Conventional short names are appropriate only when their meaning is immediate.
Prefer named options over confusing groups of positional booleans. Preserve YAML,
JSON and CLI contracts during internal refactoring. Keep parsing, defaults,
validation and side effects easy to distinguish. Prefer small concrete functions,
explicit errors and early returns; avoid speculative abstractions and cleverness.
Explain constraints in comments and measure before claiming performance gains.

Use modern Go idioms supported by the version in go.mod. Prefer standard slices,
maps and strings operations over local equivalents when they make intent clearer.
Loop variables declared with := have per-iteration scope in Go 1.22+: do not add
redundant capture rebindings. Assignments to existing variables still reuse them;
copies needed for independent mutable state are a different concern. Use t.Context
for test operations while retaining explicit cancellation and worker joins where
cleanup order requires them. Do not raise the language/API floor during cleanup.
Review go fix -diff suggestions with a Go 1.26+ toolchain, preserving nil/empty
semantics, security checks and resource lifetimes rather than applying blindly.

## Shared-file hygiene

Keep examples and documentation independent of any operator's machine. Do not add
credentials, real chat/user IDs, private hostnames, personal paths or hardware
inventory. Use placeholders, loopback fixtures and temporary directories. Standard
product paths and supported platform names belong in shared documentation. Inspect
working files and available Git history before publication, and report audit limits.
