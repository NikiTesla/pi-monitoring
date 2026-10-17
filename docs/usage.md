# Run monitoring on your Pi

This is an early foreground runtime with in-memory incident and notification
state. It supports scheduled HTTP checks, Linux host metrics and Telegram
alerts/status requests. It does not install a service, expose a dashboard or
create a database.

## Build and generate configuration

Requires Go 1.24 or later. On a 64-bit Raspberry Pi OS, build locally:

```sh
go build -trimpath -o bin/pi-monitor ./cmd/pi-monitor
./bin/pi-monitor init --output ./config.yaml --no-dashboard
```

Alternatively, cross-build on your development machine for a 64-bit Pi:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o bin/pi-monitor-linux-arm64 ./cmd/pi-monitor
```

Copy the binary to the Pi. Linux ARM64 targets a 64-bit OS (`uname -m` reports
`aarch64`); Linux AMD64 targets x86-64. A 32-bit Pi OS is not a release target.
No reviewed release packages have been published yet.

Plain `init` writes `config.yaml` in the current directory. To generate the
configuration at the default path used by `validate` and `run`, create the parent
directory and select the output explicitly:

```sh
sudo install -d -m 0750 /etc/pi-monitor
sudo ./bin/pi-monitor init --output /etc/pi-monitor/config.yaml --no-dashboard
sudo ./bin/pi-monitor validate --config /etc/pi-monitor/config.yaml
```

Existing files are preserved. This creates configuration only; no service is
installed or started. The configuration is mode 0600, so run the monitor as an
identity that can read it.

## Configure Telegram

Use a dedicated Telegram bot created through Telegram's BotFather. If you already
have a bot, use it only if no other program consumes its updates. This runtime
uses long polling: one process per bot token, with no active webhook. It does not
delete a webhook automatically. No public server port or tunnel is required.

In your private generated configuration, enable Telegram and fill all three ID
lists. Replace the angle-bracket placeholders below with numeric Telegram IDs;
this fragment is a template, not valid configuration until replaced:

```yaml
telegram:
  enabled: true
  token_env: "PI_MONITOR_TELEGRAM_TOKEN"
  allowed_chat_ids: [<YOUR_CHAT_ID>]
  allowed_user_ids: [<YOUR_USER_ID>]
  alert_chat_ids: [<DESTINATION_CHAT_ID>]
```

`token_env` is the **name** of the environment variable, never the bot token.
Validation does not require its value; `run` does. All three ID lists must be
nonempty when Telegram is enabled, including `allowed_user_ids` even if you only
intend to receive alerts. Ensure you validate the same file you edited by passing
its path with `--config`.

Chat IDs and user IDs have separate roles: commands require both an allowed chat
and an allowed sender. Alert destinations do not grant command access. In a group,
use the group's chat ID and your personal user ID; group chat IDs can be negative.
Start a private conversation with your bot before expecting private messages, or
add it to the destination group.

If you do not know your IDs, send a message to the bot, then inspect the `message`
object from Telegram's `getUpdates` API using a trusted local client: `chat.id`
is the chat ID and `from.id` is the user ID. Do this before starting this runtime.
Avoid pasting token-bearing API URLs into a browser or sharing API responses.
The official API reference is https://core.telegram.org/bots/api#getupdates.

Keep the token out of YAML, command arguments, Git and shared logs. Save it in a
private file named `secrets.env` beside your configuration:

```sh
PI_MONITOR_TELEGRAM_TOKEN='YOUR_BOT_TOKEN'
```

Replace the placeholder by editing the file locally, then restrict its permissions:

```sh
chmod 600 secrets.env
```

`run` and `check` automatically read this file on every launch. With
`--config /etc/pi-monitor/config.yaml`, the file is `/etc/pi-monitor/secrets.env`.
With `--config ./config.yaml`, it is `./secrets.env`. The current working directory
does not affect lookup when the configuration path is absolute. No shell wrapper,
`.zshrc` entry or manual export is needed; keep launching with the same config path
after a process restart. This does not arrange automatic process startup at boot.

The file is optional when credentials are already in the process environment.
An explicitly set environment variable takes precedence, including an empty value.
Both commands fail before making network requests if an existing file is invalid,
unreadable, larger than 64 KiB, a symlink, or accessible to group/other users.
`validate` continues to check YAML only and does not read secrets.

Use one `NAME=value` assignment per line, with optional `export ` and single or
double quotes around the value. Blank lines and whole-line `#` comments are allowed.
Values are literal: no variable expansion, command execution, escape processing,
multiline values or trailing comments. Duplicate assignments are rejected. Only
variables referenced by the command's Telegram or HTTP header settings are imported;
`check` imports only the selected check's header credentials. `.env` files are not
searched.

Alternatively, for an interactive Bash session, read the token without echoing or
saving its value in history:

```bash
read -r -s -p 'Telegram bot token: ' PI_MONITOR_TELEGRAM_TOKEN
export PI_MONITOR_TELEGRAM_TOKEN
printf '\n'
```

For zsh, use its prompt syntax instead:

```zsh
read -rs 'PI_MONITOR_TELEGRAM_TOKEN?Telegram bot token: '
export PI_MONITOR_TELEGRAM_TOKEN
printf '\n'
```

Keep configuration private too (`chmod 600 config.yaml`). When using a supervisor,
its process identity must be able to read the config and the adjacent secrets file.

## Choose what to monitor

Generated YAML enables host metrics and basic low-memory, low-disk, high-CPU and
high-temperature rules. Review thresholds for your Pi before running. Set
`host.mounts` to filesystems you want observed. Disk rules evaluate each configured
mount independently. Use local filesystems: a probe of an unresponsive network
filesystem can block collection and shutdown. Unsupported or unreadable observations are unavailable, not
zero. CPU utilization needs two samples and is unavailable on the first sample.
Temperature depends on exposed Linux thermal sensors. Missing observations cannot
recover an open incident.

To monitor an application's health endpoint, replace `http_checks: []` with:

```yaml
http_checks:
  - id: "example-api"
    url: "http://localhost:8080/health"
    interval: "30s"
    timeout: "5s"
    expect:
      status: 200
    alert:
      failures: 3
      recovery_successes: 2
```

Use the actual health URL of your application. For JSON assertions, see
[the HTTP example](../examples/http.yaml). All configured assertions must pass.
HTTP timeouts must be shorter than intervals. Scheduling bounds work and prevents
concurrent executions of the same check. The scheduler checks for due work every
100 milliseconds; it is not intended for subsecond monitoring.

For authenticated endpoints, reference a process environment or `secrets.env` value:

```yaml
    headers:
      Authorization:
        env: "APP_HEALTH_AUTHORIZATION"
```

Its value must include the complete header, including any `Bearer ` prefix.
`validate` performs local checks without requiring secrets or contacting endpoints.

## Start and request status

```sh
./bin/pi-monitor validate --config ./config.yaml
./bin/pi-monitor run --config ./config.yaml
```

Leave the process running; Ctrl+C or SIGTERM cancels it. For background operation
and startup after reboot, use the manual systemd setup below. `init --install`
is unavailable; `init --install --dry-run` only describes the future layout.

Send commands from an allowed user in an allowed chat:

On startup with Telegram enabled, the monitor registers its default command list
and a command menu button. Open **Menu** in the private bot chat, or type `/` to
choose a command. `/check` needs an ID: use `/checks` to find it, then send
`/check example-api`. Registration retries in the background on API failure;
monitoring and typed commands continue. Menu entries are visible by default, but
commands still require both an allowed chat and sender. Existing chat-specific
or language-specific Telegram menu settings can override these defaults.

| Command | Response |
| --- | --- |
| `/help` | Available commands |
| `/status` | Overall monitoring summary |
| `/host` | Latest host observations and their time |
| `/checks` | Latest HTTP results and freshness |
| `/check example-api` | Request an immediate evaluation of that check |
| `/incidents` | Current open incidents |
| `/reboot` | Request a one-time confirmation to reboot the server (disabled by default) |

A request for an immediate check can be declined when that check is already
running or capacity is full. Cached status is labelled with its observation time;
missing/stale results are not proof of health. In groups, address commands to
this bot with `/checks@YourBotUsername` when needed.

Replies use spaced sections and status icons: 🟢 healthy, 🔴 failed, 🟠 stale or
unknown, and ⚪ unavailable. Host metrics appear one per line, with unavailable
sensors labelled explicitly. Observation times use UTC. Alerts distinguish new,
ongoing and recovered incidents; host incident values retain their observation
time so an opening value is not mistaken for a current measurement.

HTTP alerts open after the configured consecutive failures and recover after
consecutive successes. Host rules require their threshold and recovery durations.
`alerts.startup_grace` delays opening alerts during startup. Slow/unavailable
Telegram delivery does not block monitoring.

## Enable server reboot from Telegram

Reboot is optional and disabled by default. For the supplied systemd service,
first install or upgrade the service with `make install-service`. This preserves
existing deployed configuration. Then explicitly grant the dedicated `pi-monitor`
account permission to request a reboot:

```sh
make install-reboot-policy
```

This installs [the optional polkit rule](../deploy/50-pi-monitor-reboot.rules).
It requires systemd/logind, `/usr/bin/systemctl` and polkit with a
`/etc/polkit-1/rules.d` directory. The rule grants only logind reboot actions,
including reboot with other users logged in; it does not authorize ignoring
shutdown inhibitors. It works with the unit's `NoNewPrivileges=true` setting.
See the [logind authorization reference](https://wiki.freedesktop.org/www/Software/systemd/logind/)
and [polkit rules reference](https://polkit.pages.freedesktop.org/polkit/polkit.8.html).

In the existing `telegram:` section of `/etc/pi-monitor/config.yaml`, add:

```yaml
  allow_reboot: true
```

Restart monitoring with `sudo systemctl restart pi-monitor.service`. Send
`/reboot` from an allowed user in an allowed chat, then copy the exact
`/reboot <token>` command in the reply within 60 seconds. This reboots the whole
host and interrupts its applications. Confirmation belongs to the requesting
user and chat; a new prompt replaces the previous one. Ignore the prompt to
cancel. Delayed commands older than 60 seconds are rejected, and confirmations
cannot be reused after submission or a monitor restart. Keep the host clock
synchronized; message timestamps more than five seconds ahead are rejected.

The monitor invokes one fixed systemctl command without a shell, with a
10-second timeout. Failed or ambiguous requests are not retried automatically;
check server state before starting another confirmation. The bot may go offline
before its final reply is delivered. An accepted request is not proof that the
host has completed rebooting. An enabled monitoring service starts again at boot.
The bot, host and network must still be responsive enough to receive the command.

To disable this feature, set `allow_reboot: false` and restart monitoring. To
revoke the service account's OS permission too, remove
`/etc/polkit-1/rules.d/50-pi-monitor-reboot.rules`; polkit reloads rule changes
automatically. Use the dedicated service for this feature: running the bot as
root grants much broader host access. Live reboot and target-host policy behavior
require deployment verification; development tests use a fake reboot function.

## Run in the background with systemd

On a Linux system using systemd, [the service unit](../deploy/pi-monitor.service)
runs the binary as a dedicated unprivileged account. It restarts after failures
with a 10-second delay and starts at boot once enabled. Logs go to the journal.
The application continues running in the foreground under systemd's supervision.

From the repository root, after configuring `config.yaml` and any adjacent
`secrets.env`, the Makefile can perform the setup below:

```sh
make install-service
make start-service
```

Use `make install-service CONFIG=./custom.local.yaml` to select another source
configuration. Installation builds the binary, uses sudo, creates the service
account if needed, installs the binary/unit and enables startup at boot. Existing
deployed configuration and secrets are preserved; edit `/etc/pi-monitor` to update
them. An adjacent source `secrets.env` is copied only when no deployed secrets file
exists. Installation does not restart an already running service; after upgrading
its binary, run `sudo systemctl restart pi-monitor.service` to load the new version.
`make start-service` starts monitoring immediately. Stop any foreground instance
first so only one process polls your bot. Terminal exports are not inherited;
store required credentials in `secrets.env` before starting.

These are manual deployment commands, run on the target host from the repository
root after building and configuring `config.yaml`. Stop any foreground instance
before starting the service so only one process polls your Telegram bot.

Create the service account once (skip this command if it already exists):

```sh
sudo useradd --system --user-group --no-create-home --shell /usr/sbin/nologin pi-monitor
```

Install the binary and configuration. The binary and unit installation commands
replace previous versions; `cp -n` preserves existing configuration and secrets.
If `/etc/pi-monitor/config.yaml` already exists, edit that file for this deployment.

```sh
make build
sudo install -o root -g root -m 0755 bin/pi-monitor /usr/local/bin/pi-monitor
sudo install -d -o root -g pi-monitor -m 0750 /etc/pi-monitor
sudo cp -n ./config.yaml /etc/pi-monitor/config.yaml
sudo chown root:pi-monitor /etc/pi-monitor/config.yaml
sudo chmod 0640 /etc/pi-monitor/config.yaml
```

If you use `secrets.env`, copy it too. It must be readable by the service account
and inaccessible to group/other users; the application reads it directly, so no
systemd `EnvironmentFile` is needed. Shell exports from your terminal are not
inherited by this system service.

```sh
sudo cp -n ./secrets.env /etc/pi-monitor/secrets.env
sudo chown pi-monitor:pi-monitor /etc/pi-monitor/secrets.env
sudo chmod 0600 /etc/pi-monitor/secrets.env
```

Validate as the service account, install the unit, then enable and start it:

```sh
sudo -u pi-monitor /usr/local/bin/pi-monitor validate --config /etc/pi-monitor/config.yaml
sudo install -o root -g root -m 0644 deploy/pi-monitor.service /etc/systemd/system/pi-monitor.service
sudo systemctl daemon-reload
sudo systemctl enable --now pi-monitor.service
```

`enable --now` starts monitoring immediately and enables startup after reboot.
An enabled Telegram configuration can send alerts and consume bot updates once
the service starts. Inspect status and follow logs with:

```sh
systemctl status pi-monitor.service
sudo journalctl -u pi-monitor.service -f
```

After editing configuration or secrets, use
`sudo systemctl restart pi-monitor.service`. Repeated startup failures are rate limited; after correcting
the cause, run `sudo systemctl reset-failed pi-monitor.service` and
`sudo systemctl start pi-monitor.service`. A deliberate `systemctl stop` leaves it
stopped until manually started or the next boot. To stop it and disable boot startup:

```sh
sudo systemctl disable --now pi-monitor.service
```

The service account needs read/traverse access to monitored mounts; restricted
mounts or sensors may report unavailable. The unit preserves the host filesystem
view so collected disk metrics describe the host. State remains in memory and
resets on every process restart. Live systemd deployment and reboot behavior
still require verification on the target host.

## Limits and diagnosis

State is in memory. Restarting resets incidents, failure streaks and pending
notifications; alerts can repeat after restart. Delivery may be duplicated after
an ambiguous network failure. Pending alerts are bounded/coalesced during an
outage; this is not a durable incident archive. Acknowledgement and maintenance
commands are not implemented. Pending command replies also coalesce per chat:
rapid requests can replace an earlier pending reply, particularly during outages.

`run` rejects enabled Docker monitoring and configured heartbeats. Dashboard and
storage settings are reserved: no listener or database is created. There is no
Docker/container monitoring or metric history yet.

The monitor runs on the Pi, so it cannot send a message when the Pi loses power
or network access. Detecting that requires another machine or external heartbeat
service. Do not interpret silence as proof of health.

For HTTP failures, `check` prints sanitized JSON with a category and reason:

```sh
./bin/pi-monitor check example-api --config ./config.yaml
```

This evaluates once and sends no Telegram notification. Exit codes are 0 for
success, 1 for a failed check/operation and 2 for invalid configuration/usage.
Results omit raw URLs, header values and response bodies. Keep secrets out of
server names, check IDs and filesystem labels because those identify observations.
HTTPS verification remains enabled and HTTP proxy environment variables are
honored. Redirects default off; prefer the final HTTPS health URL.

To remove a manual systemd deployment, disable it with the command above, remove
`/etc/systemd/system/pi-monitor.service` and run `sudo systemctl daemon-reload`.
Then remove `/usr/local/bin/pi-monitor` if no longer needed. For a foreground-only
deployment, stop the foreground process and remove its binary.
Delete private configuration and credentials only if no longer needed. There is
no service or database created by the application to uninstall.
