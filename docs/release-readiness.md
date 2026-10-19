# Publication and release readiness

Last audited: 2026-10-19. Repository: `github.com/NikiTesla/pi-monitoring`.

The project can be presented as an **early monitoring runtime**, with explicit
in-memory reliability limits. A public portfolio repository and a production-ready
monitoring release have different acceptance criteria. Dashboard, Docker,
heartbeats and persistent state are future work.

## Prepared in the repository

- MIT license selected by the author; dependency/runtime attribution retained.
- README describes current features, a short startup path and honest limitations.
- Architecture documents package responsibilities, ownership, limits and tradeoffs.
- Security policy describes reporting, trusted configuration and opt-in reboot.
- Independent source review covered production code, tests and deployment tooling.
  Findings were addressed before project execution; see [verification](review.md).
- Fixture tests passed on Go 1.24.4 and 1.27.2; vet and the current vulnerability
  scan passed. Repeat Linux packages/checksums and a packaged ARM64 local smoke
  test passed. The race runtime could not start on this host; hosted verification
  remains required.
- CI uses immutable action commits, read-only permissions and timeouts. It checks
  the source compatibility floor, previous/current stable Go, race behavior,
  modern Go idioms, module integrity, Linux builds and vulnerability reachability. Scheduled runs
  recheck vulnerabilities even when source has not changed.
- Release archives include security/architecture docs, deployment files and notices.
- Contributor and agent instructions describe safe isolated verification, with a
  repository-local skill for behavior/security checks.

## Before changing GitHub visibility

1. Review the final diff and staged contents, including newly added/hidden files.
   Keep ignored local configs, binaries and caches out of the publication commit.
2. Review commit author metadata: the existing local commit includes an author
   email. Decide whether it is suitable for public exposure before changing
   visibility. History was inspected but has not been rewritten.
3. Push the reviewed changes to the private repository and obtain passing hosted
   CI on that revision. Local verification does not establish hosted results.
4. Enable private vulnerability reporting and verify the form in SECURITY.md.
   Set branch protection/rulesets to require review and the applicable CI jobs;
   repository settings have not been inspected or changed by this audit.

## Before distributing a release

Follow [releasing.md](releasing.md): use a supported patched compiler, review
vulnerability results/notices, compare repeat packages and smoke-test artifacts.
Require a passing race check on a supported runner. Cross-builds do not prove
execution on both architectures. Live systemd/logind/polkit, real host sensor
coverage, Telegram delivery and long-running resource budgets need separate
deployment acceptance. Development verification must not exercise live actions.

## Privacy audit scope

The audit inspected publishable working-tree source, tests, examples, docs,
instructions, deployment files and hidden configuration. Available Git history
contains one reachable commit, `439ab5f`; its tracked contents and author metadata
were considered. Earlier untracked-tree audits are superseded by this scope.

No apparent live credentials, operator inventory, private hostnames or personal
paths were found in publication files. Tests deliberately use fake tokens/IDs and
loopback fixtures. The confirmed public repository path is intentional attribution.
Commit metadata carries the author email noted above. Ignored `config.yaml` and
`bin/` are private/local artifacts and were excluded from content output and
publication; new verification artifacts stay under ignored `.cache/`.

This is a scoped review, not proof that every possible secret was detected. No
remote-only branches/tags, GitHub settings, issues, artifacts or unreachable Git
objects were audited. Repeat the staged-content/history check immediately before
visibility changes. Nothing was published and repository visibility was not changed.
