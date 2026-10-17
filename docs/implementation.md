# Implementation status

The design describes the intended product; it is not a statement that every
component exists.

## Current implementation

- Strict YAML parsing and validation, typed defaults and environment references.
- Automatic loading of private `secrets.env` beside the selected configuration for
  `run` and `check`, with process environment variables taking precedence.
- Configuration generation, offline validation and systemd installation planning.
- Makefile systemd setup: `install-service` installs/enables the supplied unit;
  `start-service` starts it separately. Live deployment remains unverified.
- One-shot HTTP checks with status, text and typed JSON Pointer assertions.
- Foreground `run` with bounded HTTP scheduling and incident/recovery tracking.
- Linux host observations: CPU utilization, memory, load, configured filesystem
  free space and available temperature sensors; sustained host alert rules.
- Telegram notifications and authorized `/help`, `/status`, `/host`, `/checks`,
  `/check <id>` and `/incidents` commands through long polling.
- Default Telegram command menu registration on startup with independent retries.
- Optional `/reboot` with a short-lived confirmation bound to chat and sender,
  stale-command rejection and a fixed systemctl request. Disabled by default;
  `install-reboot-policy` installs the optional policy for the service account.
  Live reboot and target-host policy behavior remain unverified.
- Local fixture tests, CI configuration and Linux ARM64/AMD64 packaging tooling
  (project license and release verification still pending).

## Runtime boundaries

State and pending notifications are in memory. Restarting resets failure streaks,
incidents and delivery state. There is no durable notification outbox or history.
Telegram outages can cause delayed/coalesced notifications; delivery is not
exactly once. Do not treat this prototype as a durable incident record.

There is no dashboard, Docker integration, heartbeat API or CLI systemd
installation. `run` rejects configured Docker monitoring or heartbeats; the
reserved dashboard setting produces a warning and opens no listener. Storage
settings are reserved and do not create a database. CLI installation/start
requests fail before modifying the system.

## Remaining product work

1. Persistent incident state and bounded durable notification outbox.
2. Acknowledgement and maintenance commands.
3. Docker collectors, expected-service checks and lifecycle events.
4. SQLite history/retention, heartbeat ingestion and embedded dashboard.
5. CLI systemd installation, real Pi measurements, live Telegram acceptance
   testing and published releases after licensing and release verification.

## Review and verification

Independent source review must happen before executing project code, tests or
builds. Resolve findings first. Verification uses temporary files and disposable
loopback fixtures; it must not install/start a system service or contact Telegram.

See [review notes](review.md) for verification results and environment limitations.
Local fixtures cannot establish live bot delivery, Pi sensor availability or
long-running resource budgets.
