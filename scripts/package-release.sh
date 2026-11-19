#!/bin/sh
# Run from the repository root on Linux with GNU tar and a supported Go toolchain.
set -eu

if [ "$#" -ne 2 ]; then
    echo 'usage: sh scripts/package-release.sh VERSION NEW_OUTPUT_DIRECTORY' >&2
    exit 2
fi
version=$1
case "$version" in
    v[0-9]*) ;;
    *) echo 'version must start with v and a digit' >&2; exit 2 ;;
esac
case "$version" in
    *[!A-Za-z0-9.+-]*) echo 'invalid version characters' >&2; exit 2 ;;
esac
if [ ! -f LICENSE ]; then
    echo 'Select and add the project LICENSE before packaging releases.' >&2
    exit 1
fi
output=$2
# Refuse existing directories so old or unrelated artifacts cannot enter a release.
mkdir "$output"
output=$(cd "$output" && pwd)
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT HUP INT TERM
export LC_ALL=C TZ=UTC
export GOENV=off GOWORK=off GOFLAGS='' CGO_ENABLED=0 GOOS=linux
export GOAMD64=v1 GOARM64=v8.0

for arch in amd64 arm64; do
    name="pi-monitor_${version}_linux_${arch}"
    package="$staging/$name"
    mkdir -p "$package/examples" "$package/docs" "$package/deploy" "$package/scripts"
    GOARCH=$arch go build -mod=readonly -trimpath -buildvcs=false \
        -ldflags "-buildid= -X main.version=$version" \
        -o "$package/pi-monitor" ./cmd/pi-monitor
    cp LICENSE README.md CONTRIBUTING.md AGENTS.md SECURITY.md THIRD_PARTY_NOTICES.md "$package/"
    cp -R third_party "$package/"
    cp -R skills "$package/"
    cp scripts/check-service-account.sh "$package/scripts/"
    cp examples/http.yaml "$package/examples/"
    cp docs/*.md "$package/docs/"
    cp deploy/pi-monitor.service deploy/50-pi-monitor-reboot.rules "$package/deploy/"
    go version > "$package/BUILD.txt"
    printf 'GOOS=linux\nGOARCH=%s\nCGO_ENABLED=0\n' "$arch" >> "$package/BUILD.txt"
    tar --sort=name --mtime='@0' --owner=0 --group=0 --numeric-owner \
        --mode='u+rwX,go+rX,go-w' -C "$staging" -cf "$staging/package.tar" "$name"
    gzip -n -c "$staging/package.tar" > "$output/$name.tar.gz"
done
(cd "$output" && sha256sum ./*.tar.gz > SHA256SUMS)
echo "Release packages written to $output; nothing was uploaded."
