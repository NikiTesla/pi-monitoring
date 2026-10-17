# Preparing a release

The current product includes a foreground runtime for HTTP checks, Linux host
metrics and Telegram alerts/commands. Label releases as an early runtime with
in-memory state; the full monitoring MVP remains future work. Packages do not
install or start a service. Do not publish until a project license is
selected and the remaining readiness gates are met.

## Review and verification

1. Independently review source before executing project verification, as required
   by AGENTS.md. Resolve findings first.
2. Run formatting checks, `go test ./...`, `go vet ./...` and `go test -race ./...`.
   The GitHub CI workflow checks the minimum compiler and current stable Go, and
   cross-builds Linux ARM64 and AMD64. CI results must actually pass on the hosting
   platform; adding the workflow does not establish a passing race result.
3. Use a supported, patched Go toolchain for published builds. The minimum Go
   version in go.mod is a source compatibility floor, not a security-support claim.
   Review dependency vulnerabilities and licenses for that release.
4. Audit all publication files, hidden files and available history including commit
   metadata. Confirm local builds, configs, logs and credentials are excluded. Use
   `git status --short --ignored` and inspect the exact staged file list and contents
   before creating the release commit. Ignore rules alone do not establish privacy.

Before choosing a public tag, set the final repository/module path if needed,
enable private vulnerability reporting on the hosting platform, and document its
actual reporting URL. Do not invent a contact address or publish security reports
as public issues. A security contact is still pending.

## Build packages

From the repository root, on Linux with GNU tar, gzip, sha256sum and Go:

```sh
sh scripts/package-release.sh v0.1.0 ./dist
```

The output directory must not exist. Packaging also requires a project LICENSE;
it fails before building or creating output when that file is missing. Output is
two Linux archives and SHA256SUMS. Each archive contains a fresh binary, version,
compiler metadata, README, usage/design/status docs, HTTP example and licenses.
Packaging uploads nothing. Review artifacts before uploading them manually.

Builds disable cgo, workspace files, per-user Go environment configuration and VCS
metadata. They use `-trimpath`, a fixed build ID and the supplied version. Archives
normalize ordering, timestamps, ownership and modes, and gzip omits timestamps.
For identical output, use identical source, dependencies, Go toolchain and build
environment. Record the exact toolchain version with the release.

Build twice into two fresh directories using the same version and compare the
archives byte for byte. Verify SHA256SUMS, inspect archive members and smoke-test
version, init, validate and an isolated local HTTP check. Smoke-test ARM64 on an
ARM64 host before claiming it has run there; cross-compiling is not execution.
Never install/start a service or contact Telegram for release verification.

Ship the dependency and runtime notices with every binary. Do not upload old
ignored binaries. Checksums alone do not authenticate the publisher; use the
chosen hosting platform's release identity and document any signing scheme only
once it actually exists.
