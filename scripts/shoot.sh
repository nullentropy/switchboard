#!/bin/sh
set -eu
out=${1:?usage: scripts/shoot.sh OUTDIR [WIDTH HEIGHT]}
w=${2:-1440}
h=${3:-900}
cd "$(dirname "$0")/.."
mkdir -p "$out"
bin=$(mktemp -d)
go build -o "$bin/switchboard" .
go build -o "$bin/term" github.com/nullentropy/caution/cmd/term
port=$((20000 + $$ % 10000))
"$bin/switchboard" -demo -addr "127.0.0.1:$port" >"$out/server.log" 2>&1 &
pid=$!
trap 'kill $pid 2>/dev/null; rm -rf "$bin"' EXIT
sleep 2
url="ws://127.0.0.1:$port/ws"
shot() { name=$1; shift; "$bin/term" -url "$url" -shot "$out/$name.png" -w "$w" -h "$h" "$@" >/dev/null; }
shot wall -settle 4000
shot wall-later -settle 20000
shot new -settle 4500 -clicks "$((w - 90)),30@3200"
shot palette -settle 4500 -clicks "$((w - 305)),30@3200"
shot session-waiting -settle 5000 -clicks "400,215@3200"
shot session-working -settle 5000 -clicks "250,470@3200"
shot goal-filter -settle 5000 -clicks "600,110@3200"
shot session-diff -settle 6000 -clicks "250,470@3200;520,390@4500"
state=$(mktemp -d)
"$bin/switchboard" -state "$state/state.json" -claude /usr/bin/true -addr "127.0.0.1:$((port + 1))" >>"$out/server.log" 2>&1 &
pid2=$!
trap 'kill $pid $pid2 2>/dev/null; rm -rf "$bin" "$state"' EXIT
sleep 2
url="ws://127.0.0.1:$((port + 1))/ws"
shot empty -settle 3000
kill $pid2 2>/dev/null
"$bin/switchboard" -demo -demo-sessions 20 -addr "127.0.0.1:$((port + 2))" >>"$out/server.log" 2>&1 &
pid3=$!
trap 'kill $pid $pid2 $pid3 2>/dev/null; rm -rf "$bin" "$state"' EXIT
sleep 2
url="ws://127.0.0.1:$((port + 2))/ws"
shot many -settle 12000
echo "screenshots in $out"
