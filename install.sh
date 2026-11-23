#!/bin/sh
set -eu

repo=${SWITCHBOARD_REPO:-nullentropy/switchboard}
url=${SWITCHBOARD_URL:-}

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

case $(uname -s) in
Darwin)
	os=macos
	asset=Switchboard-macos-universal.zip
	dest=${SWITCHBOARD_DEST:-/Applications}
	;;
Linux)
	os=linux
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) fail "no Linux build for $(uname -m); build from source instead" ;;
	esac
	asset=Switchboard-linux-$arch.tar.gz
	dest=${SWITCHBOARD_DEST:-$HOME/.local}
	;;
*)
	fail "no build for $(uname -s); on Windows run install.ps1"
	;;
esac

if [ -z "$url" ]; then
	case $repo in
	*/*) ;;
	*) fail "SWITCHBOARD_REPO must be in owner/name form, such as nullentropy/switchboard, got: $repo" ;;
	esac
	url="https://github.com/$repo/releases/latest/download/$asset"
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

download() {
	echo "Downloading $url"
	curl -fSL --progress-bar -o "$work/$asset" "$url" || fail "download failed: $url"
}

install_macos() {
	[ -d "$dest" ] || fail "$dest does not exist"
	[ -w "$dest" ] || fail "cannot write to $dest; run as an admin user or set SWITCHBOARD_DEST"
	app="$dest/Switchboard.app"
	download
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
}

install_linux() {
	mkdir -p "$dest" || fail "cannot create $dest"
	[ -w "$dest" ] || fail "cannot write to $dest; set SWITCHBOARD_DEST"
	download
	mkdir "$work/unpacked"
	tar -xzf "$work/$asset" -C "$work/unpacked" || fail "could not unpack $asset"
	[ -f "$work/unpacked/bin/switchboard" ] || fail "$asset has no bin/switchboard"

	exe="$dest/bin/switchboard"
	if pgrep -f "^$exe( |\$)" >/dev/null 2>&1; then
		echo "Quitting the running Switchboard"
		pkill -TERM -f "^$exe( |\$)" || true
		i=0
		while pgrep -f "^$exe( |\$)" >/dev/null 2>&1 && [ $i -lt 20 ]; do
			sleep 0.5
			i=$((i + 1))
		done
		pkill -KILL -f "^$exe( |\$)" 2>/dev/null || true
	fi

	mkdir -p "$dest/bin" "$dest/share"
	cp "$work/unpacked/bin/switchboard" "$dest/bin/.switchboard.new"
	chmod 755 "$dest/bin/.switchboard.new"
	mv -f "$dest/bin/.switchboard.new" "$exe"
	cp -R "$work/unpacked/share/." "$dest/share/"
	entry="$dest/share/applications/switchboard.desktop"
	sed "s|^Exec=.*|Exec=$exe|" "$entry" >"$work/entry" && cat "$work/entry" >"$entry"
	if command -v update-desktop-database >/dev/null 2>&1; then
		update-desktop-database "$dest/share/applications" >/dev/null 2>&1 || true
	fi
	echo "Installed Switchboard to $exe"
}

case $os in
macos) install_macos ;;
linux) install_linux ;;
esac
