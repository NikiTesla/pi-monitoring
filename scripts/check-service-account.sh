#!/bin/sh
# Run from the source or package root, before assigning service credentials or privileges.
set -eu

case "$#:${1-}" in
    0:|1:--adopt) ;;
    *) echo 'Usage: sh scripts/check-service-account.sh [--adopt]' >&2; exit 2 ;;
esac

reject_identity() {
    echo 'Refusing to reuse pi-monitor. Verify it is a dedicated, password-locked nonlogin account with a private pi-monitor primary group. Resolve any user/group collision before retrying.' >&2
    exit 1
}

groups=$(getent group) || reject_identity
group_id=$(printf '%s\n' "$groups" | awk -F: '
    {
        gidCount[$3]++
        if ($1 == "pi-monitor") {
            matches++
            gid = $3
            privateGroup = NF == 4 && gid ~ /^[0-9]+$/ && gid > 0 && ($4 == "" || $4 == "pi-monitor")
        }
    }
    END {
        if (matches != 1 || !privateGroup || gidCount[gid] != 1) exit 1
        print gid
    }
') || reject_identity

accounts=$(getent passwd) || reject_identity
printf '%s\n' "$accounts" | awk -F: -v groupID="$group_id" '
    {
        uidCount[$3]++
        if ($4 == groupID) primaryMembers++
        if ($1 == "pi-monitor") {
            matches++
            uid = $3
            dedicated = NF == 7 && $3 ~ /^[0-9]+$/ && $3 > 0 && $4 == groupID && $7 ~ /^\/(usr\/)?(sbin\/nologin|bin\/false)$/
        }
    }
    END { exit !(matches == 1 && dedicated && uidCount[uid] == 1 && primaryMembers == 1) }
' || reject_identity

status=$(sudo env LC_ALL=C passwd -S pi-monitor) || reject_identity
case "$status" in
    'pi-monitor L '*|'pi-monitor LK '*) ;;
    *) reject_identity ;;
esac

if [ "${1-}" != '--adopt' ]; then
    unit=/etc/systemd/system/pi-monitor.service
    if ! { sudo test -f "$unit" && sudo test ! -L "$unit" &&
        [ "$(sudo stat -c '%u:%a' "$unit")" = '0:644' ] &&
        sudo cmp -s deploy/pi-monitor.service "$unit"; }; then
        echo 'No matching root-owned pi-monitor unit identifies this account. Verify who uses the account and group. To adopt a dedicated identity, retry make with ADOPT_SERVICE_ACCOUNT=1 or pass --adopt to the manual account check.' >&2
        exit 1
    fi
fi
