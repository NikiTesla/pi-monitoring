# Architecture and technical decisions

This describes the implemented runtime. [design.md](design.md) is the broader
product proposal; [implementation.md](implementation.md) tracks future work.

## Code map

| Location | Responsibility |
| --- | --- |
| `cmd/pi-monitor` | CLI parsing, exit codes, signals and credential loading |
| `internal/config` | Strict YAML decoding, defaults, validation and generation |
| `internal/setup` | Exclusive config creation and future installation planning |
| `internal/httpcheck` | Bounded HTTP evaluation and sanitized assertion results |
| `internal/host` | Linux observations and sustained threshold/recovery rules |
| `internal/monitor` | Scheduling, incident state, commands and delivery ownership |
| `internal/telegram` | Bounded Bot API requests and rate-limit responses |
| `deploy` | Administrator-installed systemd unit and optional reboot policy |

The CLI composes these packages. HTTP evaluation does not know about Telegram;
host rules return transitions without delivering them. Small interfaces live at
the consuming test seams, rather than defining a provider framework.

## Decisions and tradeoffs

| Decision | Reason and consequence |
| --- | --- |
| One Go process, mostly standard library | Small deployment surface and direct control over deadlines, transport and cleanup. Linux metrics require platform-specific code. |
| Typed YAML with strict validation | Typos fail early. Defaults apply only to omitted values; zero/empty/invalid retain their meaning. YAML anchors, aliases and merge keys are rejected. |
| JSON Pointer with exact decimal comparisons | A small assertion language handles nested health payloads without an expression engine. Objects and arrays are not compared as scalar values. |
| One event loop owns monitoring state | HTTP/host results and commands update incidents sequentially. No mutex is needed around check state. |
| Four HTTP workers, no execution backlog | Slow checks cannot create unlimited queued work. A busy or missed run is unknown and breaks result streaks; it is not an endpoint failure. |
| Separate per-chat delivery loops | Telegram outages do not stall collection. Latest state replaces older pending state for an entity, so transitions can be lost during outages. |
| In-memory incidents and delivery | No database or migration lifecycle yet. Restart loses state; this is the main reliability limit before a durable monitoring release. |
| Dedicated systemd identity | Ordinary collection needs no root or Linux capabilities. Host visibility is preserved for disk measurements; this is not a complete filesystem sandbox. |

## Ownership and shutdown

`monitor.Run` validates settings, creates the child context and owns every worker.
Its deferred cleanup cancels the context and waits for workers before returning.
The main select loop alone updates check state, host snapshots, incidents and reboot
confirmation state. The polling goroutine owns bot identity and update offset.

The collector owns its previous CPU counters. Each mailbox owns a mutex-protected
map and a delivery goroutine. Replacement is checked before deleting a delivered
entry, so a newer observation is not removed by completion of an older send.
Mailboxes coalesce alerts by entity and command replies under one reply key;
rapid replies can replace each other. They are not durable command queues.

HTTP requests inherit cancellation and a configured timeout shorter than their
interval. Host reads are synchronous OS operations. Configure local filesystems:
an unresponsive network mount can block `statfs`, collection and shutdown.
Systemd's stop timeout bounds the service shutdown externally.

## Observation and incident semantics

HTTP incidents track completed consecutive failures/recovery successes.
Missing, canceled, missed and stale observations break streaks and cannot recover
an incident. The startup grace delays incident opening. A current healthy sample
can coexist with an open incident until recovery requirements are met.

Host rules keep independent state for each rule and disk mount. Trigger and
recovery thresholds point in opposite directions with a hysteresis margin.
Unavailable readings and collection gaps reset pending duration windows.
Missing sensors are represented by absent values, never invented zero readings.

Telegram displays result freshness separately from incident status. Incident
opening measurements retain their original observation time. Commands require
both an allowed chat and sender; reboot also requires fresh timestamps and an
expiring random confirmation tied to that pair.

## Resource limits and performance

- Four concurrent HTTP checks, no overlap of the same check, and a rotating
  scheduling start position prevent a fixed leading check from always winning.
- Response bodies for assertions default to 1 MiB, with a 16 MiB ceiling, measured
  after decompression. HTTP response headers are limited to 64 KiB. JSON nesting
  is limited to 128 levels.
- A shared HTTP transport pools connections when responses permit reuse. Status-only
  requests close without draining arbitrary bodies, favoring bounded work over reuse.
- Telegram responses are limited to 2 MiB; commands use a 32-entry channel and
  requests have deadlines. Rate-limit delays and retry backoff are bounded.
- Mailbox size depends on configured entities and chats. There is no unbounded
  history, but config size, check count and destination count are not capped.
- Scheduling scans configured checks every 100 ms. This is intended for a small
  personal server configuration, not thousands of targets.

No throughput, memory or Pi power budget has been established. Profile a realistic
configuration before changing algorithms or claiming performance gains. Add a
benchmark when an observed workload justifies it.

## Extension boundaries

Add assertion operators with config validation, deterministic evaluation and
redaction tests. Add host metrics with missing/reset-counter behavior and fixtures.
Keep delivery independent of evaluation. Persistent incidents/outbox need an
explicit crash/retry contract before adding storage. A dashboard needs an
authentication and listener design; there is no web server or browser UI today.
