#!/bin/sh
# Exercise the Makefile on a temporary filesystem with no privilege changes.
set -eu
repo=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf -- "$fixture"' EXIT
trap 'exit 1' HUP INT TERM
cd "$fixture"
export fixture
mkdir -p tools scripts bin usr/local/bin usr/bin run/systemd/system etc/systemd/system etc/pi-monitor etc/polkit-1/rules.d
touch usr/bin/systemctl etc/pi-monitor/config.yaml
chmod +x usr/bin/systemctl
sed -e "s@/usr/local/bin/@$fixture/usr/local/bin/@g" \
    -e "s@/run/systemd/system@$fixture/run/systemd/system@g" \
    -e "s@/usr/bin/systemctl@$fixture/usr/bin/systemctl@g" \
    -e "s@/etc/systemd/system@$fixture/etc/systemd/system@g" \
    -e "s@/etc/pi-monitor@$fixture/etc/pi-monitor@g" \
    -e "s@/etc/polkit-1/rules.d@$fixture/etc/polkit-1/rules.d@g" \
    "$repo/Makefile" > Makefile

sed "s@/etc/systemd/system@$fixture/etc/systemd/system@g" "$repo/scripts/check-service-account.sh" > scripts/check-service-account.sh
chmod +x scripts/check-service-account.sh

cat > tools/sudo <<'STUB'
#!/bin/sh
set -eu
case "$1" in
    stat)
        [ "$2" = -c ] && [ "$3" = '%u:%a' ] && [ "$4" = "$fixture/etc/systemd/system/pi-monitor.service" ] || exit 99
        echo '0:644'; exit 0 ;;
    cmp)
        [ "$2" = -s ] && [ "$3" = deploy/pi-monitor.service ] && [ "$4" = "$fixture/etc/systemd/system/pi-monitor.service" ] || exit 99
        exec cmp -s "$3" "$4" ;;
    env)
        if [ "$*" = 'env LC_ALL=C passwd -S pi-monitor' ]; then
            echo "pi-monitor $password_status 2024-01-01 0 99999 7 -1"
            exit 0
        fi ;;
    systemctl)
        case "$2 $3" in
            'cat pi-monitor.service') exit 0 ;;
            'restart pi-monitor.service') touch "$fixture/restarted"; exit 0 ;;
        esac ;;
    test) shift; test "$@"; exit $? ;;
    mktemp)
        [ "$2" = "$fixture/usr/local/bin/.pi-monitor.XXXXXXXX" ]
        exec mktemp "$2" ;;
    install)
        [ "$#" = 9 ] && [ "$2 $3 $4 $5 $6 $7 $8" = "-o root -g root -m 0755 bin/pi-monitor" ] || exit 99
        case "$9" in "$fixture"/usr/local/bin/.pi-monitor.*) exec install -m 0755 bin/pi-monitor "$9" ;; esac ;;
    -u)
        [ "$2" = pi-monitor ] && [ "$4" = validate ] && [ "$5" = --config ] && [ "$6" = "$fixture/etc/pi-monitor/config.yaml" ] || exit 99
        case "$3" in "$fixture"/usr/local/bin/.pi-monitor.*) exec "$3" "$4" "$5" "$6" ;; esac ;;
    mv)
        [ "$2 $3" = '-fT --' ] && [ "$5" = "$fixture/usr/local/bin/pi-monitor" ] || exit 99
        case "$4" in "$fixture"/usr/local/bin/.pi-monitor.*) exec mv -fT -- "$4" "$5" ;; esac ;;
    rm)
        [ "$2 $3" = '-f --' ] || exit 99
        case "$4" in "$fixture"/usr/local/bin/.pi-monitor.*) exec rm -f -- "$4" ;; esac ;;
esac
echo "unexpected sudo operation: $1" >&2
exit 99
STUB
cat > bin/pi-monitor <<'CANDIDATE'
#!/bin/sh
touch "$fixture/validated"
exit "$validation_status"
CANDIDATE
chmod +x tools/sudo bin/pi-monitor
export PATH="$fixture/tools:$PATH"
live=usr/local/bin/pi-monitor
echo original > "$live"
export validation_status=2
if make GO=true restart-service; then
    echo "restart succeeded after validation failed" >&2
    exit 1
fi
test -f validated
test ! -e restarted
test "$(cat "$live")" = original
test -z "$(find usr/local/bin -name '.pi-monitor.*' -print)"

export validation_status=0
make GO=true restart-service
cmp bin/pi-monitor "$live"
test -f restarted
test -z "$(find usr/local/bin -name '.pi-monitor.*' -print)"

cat > tools/id <<'STUB'
#!/bin/sh
[ "$*" = '-u pi-monitor' ] || exit 99
echo 997
STUB
cat > tools/getent <<'STUB'
#!/bin/sh
case "$*" in
    passwd) printf 'pi-monitor:x:997:997::/nonexistent:%s\n' "$account_shell" ;;
    group)
        echo 'pi-monitor:x:997:'
        if [ "$shared_group" = yes ]; then echo 'other:x:997:operator'; fi ;;
    *) exit 99 ;;
esac
STUB
chmod +x tools/id tools/getent
export account_shell=/usr/sbin/nologin password_status=L shared_group=no
if sh scripts/check-service-account.sh > account.log 2>&1; then
    echo "existing identity accepted without a matching installation or explicit adoption" >&2
    exit 1
fi
grep -q 'No matching root-owned' account.log
sh scripts/check-service-account.sh --adopt

mkdir deploy
cp "$repo/deploy/pi-monitor.service" deploy/
cp deploy/pi-monitor.service etc/systemd/system/
sh scripts/check-service-account.sh
password_status=P
if sh scripts/check-service-account.sh --adopt > account.log 2>&1; then
    echo "accepted an account with password login enabled" >&2
    exit 1
fi
grep -q 'Refusing to reuse pi-monitor' account.log
password_status=L
shared_group=yes
if sh scripts/check-service-account.sh --adopt > account.log 2>&1; then
    echo "accepted a group alias sharing the service GID" >&2
    exit 1
fi
grep -q 'Refusing to reuse pi-monitor' account.log
shared_group=no
account_shell=/bin/sh
for target in install-service install-reboot-policy; do
    if make GO=true ADOPT_SERVICE_ACCOUNT=1 "$target" > account.log 2>&1; then
        echo "$target accepted an interactive login account" >&2
        exit 1
    fi
    grep -q 'Refusing to reuse pi-monitor' account.log || { cat account.log >&2; exit 1; }
done
