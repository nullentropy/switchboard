#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
root=$(pwd)

if [ $# -gt 1 ]; then
	echo "usage: scripts/release.sh [version]" >&2
	exit 2
fi

if [ -n "$(git status --porcelain)" ]; then
	echo "working tree is not clean; commit or stash first" >&2
	exit 1
fi

if [ $# -eq 1 ]; then
	version=$1
else
	version=$(git describe --tags --exact-match 2>/dev/null) || {
		echo "HEAD has no tag; tag it or pass a version: scripts/release.sh 1.2.3" >&2
		exit 1
	}
fi
version=${version#v}
case $version in
'' | *[!0-9A-Za-z.-]*)
	echo "bad version: $version" >&2
	exit 1
	;;
esac
bundle_version=$(printf '%s' "$version" | sed 's/[^0-9.].*//; s/\.$//')
[ -n "$bundle_version" ] || bundle_version=0
min_macos=13.0

out="$root/release"
zip="$out/Switchboard-$version-macos-universal.zip"
work=$(mktemp -d)
cleanup() {
	git -C "$root" worktree remove --force "$work/src" 2>/dev/null || true
	rm -rf "$work"
}
trap cleanup EXIT

git worktree add --detach --quiet "$work/src" HEAD

for arch in arm64 amd64; do
	(cd "$work/src" && CGO_ENABLED=1 GOOS=darwin GOARCH=$arch \
		MACOSX_DEPLOYMENT_TARGET=$min_macos \
		CGO_CFLAGS="-O2 -g -mmacosx-version-min=$min_macos" \
		CGO_LDFLAGS="-mmacosx-version-min=$min_macos" \
		go build -trimpath -o "$work/switchboard-$arch" .)
	minos=$(vtool -show-build "$work/switchboard-$arch" | awk '$1 == "minos" { print $2 }')
	if [ "$minos" != "$min_macos" ]; then
		echo "$arch binary declares macOS $minos, want $min_macos" >&2
		exit 1
	fi
done
lipo -create -output "$work/switchboard" "$work/switchboard-arm64" "$work/switchboard-amd64"

(cd "$work/src" && go run github.com/nullentropy/caution/cmd/appbundle -bin "$work/switchboard" -name Switchboard -icon icon.png -o "$work/stage")
app="$work/stage/Switchboard.app"
plutil -replace CFBundleShortVersionString -string "$version" "$app/Contents/Info.plist"
plutil -replace CFBundleVersion -string "$bundle_version" "$app/Contents/Info.plist"
plutil -replace LSMinimumSystemVersion -string "$min_macos" "$app/Contents/Info.plist"
codesign --force -s - "$app"
codesign --verify --strict "$app"

latest="$out/Switchboard-macos-universal.zip"
mkdir -p "$out"
rm -f "$zip" "$latest"
ditto -c -k --sequesterRsrc --keepParent "$app" "$zip"
cp "$zip" "$latest"
echo "$zip"
echo "$latest"
