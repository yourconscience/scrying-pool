#!/bin/sh
# Runs `scry collect` once, right now, the way the nightly launchd job runs it,
# then shows its output and exits with its exit code.
#
#   tools/collect.sh                        every game not saved yet (-since 2160h)
#   tools/collect.sh -other 3 ID ID ID      any other collect arguments
#   SCRY_HOST=HOST tools/collect.sh         on another Mac, over ssh
#   SCRY=PATH tools/collect.sh              another binary; see the grant below
#
# It runs as a launchd job because plain `scry collect` breaks in these ways:
#
# - Over ssh, osascript can't control Brave and the keychain refuses the password
#   (`security` exits 36). A job in the user's gui domain has both.
# - macOS grants "control Brave" per binary path. A binary at a new path asks on
#   the screen, hangs for 2 minutes when nobody answers, and stays denied after
#   that. So keep to a path that has the grant, ~/.local/bin/scry by default, and
#   replace the binary in place: the grant stays through rebuilds.
#
# What the run itself does:
#
# - With no logged-in dominion.games tab it opens a new one and logs in with the
#   keychain's password, then closes that tab when it's done. A logged-out or
#   disconnected tab left open doesn't get in the way.
# - It never kicks a session. When you're logged in elsewhere, at a table or in a
#   game, or the server takes no new games, it stops and says so: games stay
#   unsaved and the next run picks them up.
set -eu

if [ -n "${SCRY_HOST:-}" ]; then
	exec ssh "$SCRY_HOST" "SCRY='${SCRY:-}' sh -s -- $*" <"$0"
fi

SCRY=${SCRY:-$HOME/.local/bin/scry}
[ $# -gt 0 ] || set -- -since 2160h

label=com.yourconscience.scry-once
job=gui/$(id -u)/$label
log=$HOME/Library/Logs/scrying-pool/once.log
plist=${TMPDIR:-/tmp}/$label.plist

state() { launchctl print "$job" 2>/dev/null | sed -n 's/^[[:space:]]*state = //p' | head -1; }

case $(state) in
"") ;;
"not running") launchctl bootout "$job" ;;
*)
	echo "a run is already going, its output is in $log" >&2
	exit 1
	;;
esac

mkdir -p "$(dirname "$log")"
: >"$log"
{
	cat <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>$label</string>
<key>RunAtLoad</key><true/>
<key>EnvironmentVariables</key><dict><key>PATH</key><string>/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin</string></dict>
<key>StandardOutPath</key><string>$log</string>
<key>StandardErrorPath</key><string>$log</string>
<key>ProgramArguments</key><array><string>$SCRY</string><string>collect</string>
EOF
	for a in "$@"; do echo "<string>$a</string>"; done
	echo '</array></dict></plist>'
} >"$plist"

launchctl bootstrap "gui/$(id -u)" "$plist"
tail -f "$log" &
tailer=$!
# launchd can report "not running" before it has started the job
sleep 5
while [ "$(state)" != "not running" ]; do sleep 5; done
sleep 1
kill "$tailer"

code=$(launchctl print "$job" | sed -n 's/^[[:space:]]*last exit code = //p' | head -1)
launchctl bootout "$job"
case $code in
"" | *[!0-9]*) exit 1 ;;
esac
exit "$code"
