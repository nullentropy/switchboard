#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
bin=$(mktemp -d)
trap 'rm -rf "$bin"' EXIT
go build -o "$bin/switchboard" .
case $(uname -s) in
Darwin)
	go run github.com/nullentropy/caution/cmd/appbundle -bin "$bin/switchboard" -name Switchboard -icon icon.png -o dist/
	codesign --force -s - dist/Switchboard.app
	echo dist/Switchboard.app
	;;
Linux)
	rm -rf dist/linux
	go run github.com/nullentropy/caution/cmd/linuxapp -bin "$bin/switchboard" -name Switchboard -icon icon.png \
		-comment "Coding agent sessions side by side" -categories "Development;" -o dist/linux
	echo dist/linux
	;;
*)
	echo "scripts/app.sh builds on macOS and Linux; on Windows run scripts/app.ps1" >&2
	exit 1
	;;
esac
