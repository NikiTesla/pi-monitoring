# pi-monitoring

A personal-server monitor built in Go, designed around Telegram and a local dashboard. The first deployment target is a Raspberry Pi; Linux ARM64 and AMD64 are intended targets.

**Early monitoring runtime: scheduled HTTP checks, Linux host metrics and Telegram alerts/commands.** Run it in the foreground or use the Makefile systemd setup to monitor and request status from your bot. Dashboard, Docker, heartbeats, persistent history and the CLI service installer remain unavailable. See [setup instructions](docs/usage.md), [the design](docs/design.md) and [implementation status](docs/implementation.md).

## Build

Requires Go 1.24 or later.

With Make installed, the local workflow is:

```sh
make build
make init
# Edit config.yaml and configure credentials before starting.
make validate
make start
```

`make start` (also `make run`) runs in the foreground; stop it with Ctrl+C.
Application targets build first. They use `./config.yaml` by default; override it
with, for example, `make start CONFIG=./custom.local.yaml`. `make init` preserves
existing configuration. `make check CHECK_ID=example-api` evaluates one configured
HTTP check and may contact its endpoint. `make help` lists all targets. Credentials
are loaded from `secrets.env` beside the selected configuration as described below.

To build directly without Make:

```sh
go build -trimpath -o bin/pi-monitor ./cmd/pi-monitor
```

## Generate and validate YAML

```sh
./bin/pi-monitor init
./bin/pi-monitor validate --config ./config.yaml
```

`init` preserves existing files. Host monitoring and basic alert rules default on. Docker and Telegram default off. The reserved dashboard setting defaults on, but no dashboard or HTTP listener is started.

```sh
./bin/pi-monitor init --output ./alternative.yaml --docker --no-dashboard
./bin/pi-monitor init --install --dry-run
```

Installation dry-run explains the future CLI systemd layout without modifying the system. Actual `--install` and `--start` deliberately fail before making changes; use `make install-service` for the Makefile setup.

## Monitor and use Telegram

Configure your bot token environment reference, allowed chat/user IDs and alert
destinations as described in [the setup guide](docs/usage.md). Save the token in
`secrets.env` beside the config (mode 0600); it is loaded automatically on each
launch, with process environment variables taking precedence. Then run:

```sh
./bin/pi-monitor run --config ./config.yaml
```

For background operation and startup after reboot, follow the
[manual systemd setup](docs/usage.md#run-in-the-background-with-systemd) using the
included [service unit](deploy/pi-monitor.service).

```sh
make install-service
make start-service
```

Installation uses sudo, preserves deployed configuration and secrets, and enables
startup at boot. `make start-service` starts monitoring immediately.

Commands include `/status`, `/host`, `/checks`, `/check <id>` and `/incidents`.
Optional `/reboot` restarts the whole server after a one-time confirmation. It is
disabled by default; see [reboot setup](docs/usage.md#enable-server-reboot-from-telegram)
for the config switch and service-account permission.
The bot registers a Telegram command menu on startup; open **Menu** in its private
chat or type `/` to choose a command. `/check` still requires a check ID.
HTTP checks generate failure and recovery alerts; host rules use sustained
thresholds and separate recovery conditions. State and pending notifications are
in memory and reset on restart. Only one process should poll a given bot token.

## Check an HTTP endpoint

Create a configuration based on [the HTTP example](examples/http.yaml), adapting its URL, then run:

```sh
./bin/pi-monitor validate --config examples/http.yaml
./bin/pi-monitor check example-api --config examples/http.yaml
```

Checks support exact HTTP status, body text equality/containment and typed JSON scalar comparisons through JSON Pointer. Configuration is YAML only. All assertions must pass. Missing values never satisfy ordinary comparisons, including `ne`. `exists` distinguishes a missing field from a present null. Numbers are compared without lossy conversion to floating point.

Timeouts and body size are bounded. HTTP redirects default off. Credentials can reference environment variables; check results contain sanitized reasons rather than raw bodies, URLs or credential values. `validate` performs local validation and does not need secrets or contact endpoints; `check` requires only credentials used by the selected check.

Exit codes are `0` for success, `1` for a failed check/operation and `2` for invalid configuration or command usage. A one-shot check never sends Telegram notifications.

See [the usage guide](docs/usage.md) for per-host configuration, credential setup,
runtime limitations and failure diagnosis. `check` evaluates once; `run` schedules
repeated checks using `interval`.

## Development

Obtain independent source review before executing verification, as required by
[contributing](CONTRIBUTING.md). Then use:

```sh
make test
make vet
make race
```

`make fmt` formats Go source. The equivalent direct verification commands are:

```sh
go test ./...
go test -race ./...
go vet ./...
```

See [contributing](CONTRIBUTING.md) for naming, readability, verification and privacy practices. Keep the implementation explicit and small. Prefer existing facilities to new abstractions. The initial reviews and verification results are recorded in [docs/review.md](docs/review.md). Race-detector verification remains outstanding; see the recorded environment limitation in that document.

Feature status and limitations must remain visible: accepting a future configuration section does not implement its runtime.

Publication is not ready yet: a project license has not been selected. See [release readiness](docs/release-readiness.md) for the remaining publication work and the scope of the privacy review. [Release preparation](docs/releasing.md) describes the Linux ARM64/AMD64 packaging script and verification gates. GitHub CI is configured for formatting, tests, vet, race checks and cross-builds; hosted results remain to be obtained.
