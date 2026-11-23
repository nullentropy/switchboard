#!/bin/sh
set -eu

repo=${SWITCHBOARD_REPO:-nullentropy/switchboard}
asset=Switchboard-macos-universal.zip
dest=${SWITCHBOARD_DEST:-/Applications}
url=${SWITCHBOARD_URL:-}

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

[ "$(uname -s)" = Darwin ] || fail "Switchboard runs on macOS only"

if [ -z "$url" ]; then
	[ -n "$repo" ] || fail "no GitHub repo set; run with SWITCHBOARD_REPO=nullentropy/switchboard"
	case $repo in
	*/*) ;;
	*) fail "SWITCHBOARD_REPO must be nullentropy/switchboard, got: $repo" ;;
	esac
	url="https://github.com/$repo/releases/latest/download/$asset"
fi

[ -d "$dest" ] || fail "$dest does not exist"
[ -w "$dest" ] || fail "cannot write to $dest; run as an admin user or set SWITCHBOARD_DEST"

app="$dest/Switchboard.app"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

echo "Downloading $url"
curl -fSL --progress-bar -o "$work/$asset" "$url" || fail "download failed: $url"
ditto -x -k "$work/$asset" "$work/unzipped" || fail "could not unzip $asset"
[ -d "$work/unzipped/Switchboard.app" ] || fail "$asset has no Switchboard.app"

exe="$app/Contents/MacOS/"
if pgrep -f "$exe" >/dev/null 2>&1; then
	echo "Quitting the running Switchboard"
	osascript -e "tell application \"$app\" to quit" >/dev/null 2>&1 || true
	i=0
	while pgrep -f "$exe" >/dev/null 2>&1 && [ $i -lt 20 ]; do
		sleep 0.5
		i=$((i + 1))
	done
	if pgrep -f "$exe" >/dev/null 2>&1; then
		pkill -TERM -f "$exe" || true
		sleep 2
		pkill -KILL -f "$exe" 2>/dev/null || true
	fi
fi

rm -rf "$app"
mv "$work/unzipped/Switchboard.app" "$app"
echo "Installed Switchboard to $app"
