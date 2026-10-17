# Working on pi-monitoring

Read docs/design.md. Prefer explicit code, the standard library and small concrete types. Do not introduce frameworks or speculative interfaces.

Current implementation includes strict YAML config, init/config generation, systemd installation planning, validate, one-shot check, version, and a foreground monitoring runtime with HTTP scheduling, Linux host metrics and Telegram alerts/commands. Makefile targets install/enable the supplied systemd unit and start it separately. Dashboard, Docker, heartbeats, persistent state and CLI service installation remain later work and must not be represented as implemented.

The user requires independent subagent source review before any project code, tests, build, installer, or application is executed. Formatting and source inspection are allowed before review. Review findings must be resolved before execution. Do not install/start a service or contact Telegram during development verification.

Agents own their assigned files. Do not overwrite other agents' work. New tests should verify observable behavior, validation, security boundaries or failure handling. Use temporary directories and local HTTP fixtures; no live services.

## Readable code and naming

Follow CONTRIBUTING.md. Name domain entities and operations by their meaning and
intent; use descriptive names across long scopes and units where ambiguous.
Conventional short names are appropriate only when their meaning is immediate.
Prefer named options over confusing groups of positional booleans. Preserve YAML,
JSON and CLI contracts during internal refactoring. Keep parsing, defaults,
validation and side effects easy to distinguish. Prefer small concrete functions,
explicit errors and early returns; avoid speculative abstractions and cleverness.
Explain constraints in comments and measure before claiming performance gains.

## Shared-file hygiene

Keep examples and documentation independent of any operator's machine. Do not add
credentials, real chat/user IDs, private hostnames, personal paths or hardware
inventory. Use placeholders, loopback fixtures and temporary directories. Standard
product paths and supported platform names belong in shared documentation. Inspect
working files and available Git history before publication, and report audit limits.
