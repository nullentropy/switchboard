#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
bin=$(mktemp -d)
trap 'rm -rf "$bin"' EXIT
go build -o "$bin/switchboard" .
go run github.com/nullentropy/caution/cmd/appbundle -bin "$bin/switchboard" -name Switchboard -icon icon.png -o dist/
codesign --force -s - dist/Switchboard.app
echo dist/Switchboard.app
