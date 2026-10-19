# Security

## Reporting a vulnerability

Use GitHub's private
[Report a vulnerability](https://github.com/NikiTesla/pi-monitoring/security/advisories/new)
form when available. Private vulnerability reporting must be enabled by the
maintainer before public launch. If unavailable, ask the maintainer to enable a
private route without including exploit details in a public issue. There is no
separate security email or response-time commitment yet.

Include the affected revision, a minimal reproduction with fake credentials and
local fixtures, impact and any mitigation. Do not include bot tokens, private
configurations, chat identifiers or raw production logs.

## Support and trust model

This is an early runtime with no stable security-supported release line. Fixes
target the current development branch. Use a supported, patched Go toolchain
and check [publication readiness](docs/release-readiness.md) before deployment.

Configuration, environment and installation directories are controlled by a
trusted operator. HTTP targets may return hostile responses. Telegram senders are
untrusted until both chat and sender allowlists pass. A compromised host, operator
account or bot account is outside the application's security boundary.

HTTP URLs can deliberately reach loopback and private services. There is no SSRF
filter or egress sandbox: do not expose configuration editing to untrusted users.
HTTP checks honor standard proxy environment variables. Protect proxy settings,
DNS and trust roots as part of the host configuration.

## Implemented controls

- TLS verification remains enabled; redirects default off. On crossing origins,
  every configured header and generated Referer is removed for the remaining
  redirect chain. Plain HTTP supports local services; use HTTPS when credentials
  cross an untrusted network.
- Timeouts, response/header limits and JSON depth/number limits bound network
  evaluation. Results omit URLs, headers, bodies and raw transport errors.
- Credential files must be private regular files, bounded to 64 KiB. Supported
  Unix systems use no-follow, nonblocking opening and descriptor checks against
  file replacement. Values are literal assignments, never shell expansions.
  Process environment values take precedence, even if empty.
- Telegram calls reject redirects and suppress token-bearing URLs and raw API
  error descriptions. Messages are plain text with previews disabled.
- Commands require an allowed chat **and** user; bot senders and anonymous
  sender-chat messages are rejected. Menu visibility grants no access.
- Reboot defaults off. Confirmations expire after 60 seconds, bind to sender and
  chat, and are consumed once. A fixed `systemctl` command runs without a shell.
  The optional policy grants the dedicated OS account permission to reboot,
  including with other logged-in users. Compromise of that account can exercise
  the OS permission directly; Telegram confirmation is an application safeguard.
- The service runs under a dedicated identity with `NoNewPrivileges`, no Linux
  capabilities and restrictions on socket families, realtime and set-ID operations.
  It preserves host filesystem visibility for collection. Protect configuration
  directories and grant only the filesystem access needed for monitored mounts.

Diagnostics retain field names and may include operator-supplied paths/IDs or YAML
parser context. Runtime-result redaction does not make arbitrary configuration
errors safe to publish. Review diagnostics before sharing.

## Reliability limits

There is no durable outbox, history or exactly-once delivery. Latest pending state
can replace earlier transitions, and ambiguous sends can duplicate alerts.
Restart resets incidents and pending messages. A host running the monitor cannot
notify while offline; use an external probe for outage detection. Local mounts
are required because a blocked filesystem probe can delay shutdown.

No dashboard or inbound listener is implemented. Reserved settings do not provide
authentication, storage or a web API. Docker runtime access is unimplemented;
do not grant Docker socket access for current monitoring.
