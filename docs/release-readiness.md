# Release readiness

Status: suitable for review as an early prototype; publication preparation is
incomplete. The supported milestone includes configuration, setup planning,
one-shot and scheduled HTTP checks, Linux host metrics and Telegram alerts/commands.
Runtime state is in memory; it is not yet a production monitoring release.

## Publication work

- Choose the project's license and include its license text before presenting the
  repository as open source. A license has not been selected.
- Review distribution notices for the chosen release toolchain. The sole direct third-party dependency is
  `gopkg.in/yaml.v3` at `v3.0.1`; its source includes MIT and Apache 2.0 notices in
  `LICENSE` and `NOTICE`. Preserve the relevant dependency and Go runtime notices
  when packaging binaries. Notices are now included in `third_party/` and
  `THIRD_PARTY_NOTICES.md`; no dependency code is vendored here.
- Run the configured GitHub CI on the chosen hosting platform. It covers
  formatting, tests, vet, race checks and Linux cross-builds on the minimum and
  current stable Go toolchains. No hosted run has been observed yet.
- Complete race-detector verification on a supported environment. Earlier tests
  were blocked by a ThreadSanitizer memory-layout limitation.
- Before distributing releases, rerun ARM64/AMD64 packaging with the selected
  project LICENSE and supported patched toolchain. The script's archives and
  checksums were verified in an isolated copy with an explicitly fake license
  fixture, including byte-for-byte repeatability; those are not releases.
  Configure a private vulnerability-reporting route on the chosen hosting
  platform. See [release preparation](releasing.md).
- Run a final review of the actual publication contents and newly added Git history.
  No repository has been published by this work.

A final public repository location and module path are also undecided. Update the
local `pi-monitoring` module path if the publication location requires it.

The latest local source verification passed tests and vet after independent
review. Race verification again stopped before tests with the previously recorded
ThreadSanitizer memory-layout limitation; hosted CI must resolve this gate.

## Privacy review scope

The 2026-10-15 review covers publishable working-tree source, tests, examples, project
instructions and documentation, including hidden files outside Git's internal
metadata. All project files were untracked and there were zero reachable commits,
so there was no committed content or author metadata to audit. Machine-local Git
configuration, user home files and unrelated projects are not publication contents.
Two pre-existing binaries under the ignored `bin/` directory are local build
artifacts and contain embedded local source paths. Exclude them from publication.
Fresh Linux ARM64/AMD64 builds made with `-trimpath` were checked for personal
source paths and had none of the `/home/` or `/Users/` path patterns. They are
verification artifacts, not packaged releases.

Source inspection and pattern scans found no apparent real credentials, personal
identifiers, private hostnames, LAN addresses or personal home-directory paths in
shared files. Tests contain deliberately fake secret strings and generated local
HTTP/TLS fixtures. Environment variable names and loopback URLs are intentional.
The operator-specific deployment inventory and conversational design history were
removed from the design, and verification notes describe environment limitations
without identifying a machine. Standard installation paths and supported platforms
remain documented because they are product requirements.

This is a scoped source review, not proof that every possible secret was detected.
Repeat it after staging files and before publishing. Ignore rules cover local
environment files, common key/certificate files, local YAML overrides and database
artifacts; they do not remove an already tracked secret.
