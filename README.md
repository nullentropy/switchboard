# Switchboard

Switchboard is a macOS app that runs your coding agent sessions side by side.
Each session is a tile that shows what its agent is doing and asks you when it
needs permission.

## Requirements

- macOS 13 Ventura or newer for a release build. An app built with
  `scripts/app.sh` runs on the macOS version it was built on or newer.
- Go 1.26 or newer, to build from source. The version comes from `go.mod`.
- Xcode Command Line Tools, for the C compiler, `codesign` and `lipo`.
- The `claude` CLI on your `PATH`, signed in. Switchboard will not start
  without it, and the dispatcher runs on it.
- Optional: the `codex` and `gemini` CLIs, for sessions that use those
  agents. Sign in to each once in a terminal first: `codex login`, or run
  `gemini` once (or set `GEMINI_API_KEY`).

Each agent uses its own login, settings and instruction files.

## Install

```
curl -fsSL https://raw.githubusercontent.com/nullentropy/switchboard/main/install.sh | sh
```

This downloads the latest release into `/Applications`, replacing an existing
`Switchboard.app` and quitting it first if it is running. curl does not mark
the download as quarantined, so macOS opens the app without asking.

Or install from a release zip by hand. Unzip
`Switchboard-<version>-macos-universal.zip` and move `Switchboard.app` to
Applications. The app runs on Apple silicon and Intel Macs.

The app is ad hoc signed, not notarized. macOS quarantines a copy downloaded
in a browser and will refuse to open it. Either right-click the app, choose Open, and
confirm, or clear the quarantine flag:

```
xattr -dr com.apple.quarantine /Applications/Switchboard.app
```

## Run from source

```
go run .                 # open the window
go run . -demo           # pretend sessions, nothing saved
go run . -addr :8790     # serve the UI to a browser instead of a window
```

Other flags: `-claude`, `-codex` and `-gemini` point at other binaries.
`-state` sets where the session list and settings are kept.

## Build the app

```
scripts/app.sh
```

This writes `dist/Switchboard.app`, ad hoc signed, for your Mac's
architecture. Drag it to Applications. When opened from Finder it takes `PATH`
from your login shell, so it finds the agent CLIs where your terminal does. It
logs to `~/Library/Logs/Switchboard.log`.

## Make a release

```
scripts/release.sh          # version from the tag on HEAD
scripts/release.sh 1.2.3    # or pass the version
```

The working tree must be clean. Without an argument, HEAD must have a tag; a
leading `v` is dropped. The script builds arm64 and amd64 binaries for macOS
13 or newer from a clean checkout of HEAD, joins them into one universal app,
sets the version in its `Info.plist`, signs it ad hoc and writes
`release/Switchboard-<version>-macos-universal.zip`. It also writes the same
zip as `release/Switchboard-macos-universal.zip`. Attach that one to every
release too, because `install.sh` downloads it by that name.

macOS 13 is the oldest version Go 1.27 supports.

## Reference

### Keyboard shortcuts

Press `?` or the Shortcuts button for the full list.

| Key | Action |
|---|---|
| `Cmd+K` | Commands: every action and session |
| `Cmd+N` | New session |
| `Cmd+L` | Ask the dispatcher |
| `J` `K` | Move between sessions |
| `Enter` | Open the selected session, or write in the open one |
| `Cmd+1` to `Cmd+9` | Open a session by position |
| `A` | Allow what the selected or first waiting session asks |
| `Shift+A` | Always allow it for that session |
| `D` | Deny it |
| `Cmd+J` | Jump to the next session that needs you |
| `.` | Interrupt the turn |
| `R` | Rename |
| `Cmd+Delete` | Remove |
| `Esc` | Leave the message box, then back to all sessions |
| `Cmd+,` | Settings |

### The dispatcher

The bar at the bottom of the window talks to the dispatcher, a Claude session
whose tools act on the wall. It can:

- report system load and list or read sessions,
- start a session in a folder with claude, codex or gemini,
- send a session a message, interrupt it or stop it,
- remove a stopped session, unless its worktree has uncommitted or unmerged
  work,
- read and save themes.

Give it a goal that needs several sessions and it splits the work and starts
one session per task under a shared goal name. It cannot answer permission
prompts. Those stay with you.

### After a restart

Quitting Switchboard ends every agent process, and each session keeps the
state it had. When the app opens again, a session that had finished its turn
or was waiting on you shows as Ready, and one that had failed shows as Failed.
A session that was in the middle of a turn shows as Interrupted. Only a
session that you or the dispatcher stopped shows as Stopped. A message to any
of them resumes its conversation.

If any session was interrupted, Switchboard lists them when it opens. Resume
all tells each one that Switchboard restarted mid-turn and to carry on where
it left off. Not now leaves them Interrupted.

### Themes

Settings has five themes: Gruvbox (the default), Graphite, Paper, Phosphor and
Amber. Ask the dispatcher for another and it saves a named theme that then
appears in Settings.

### Where things are stored

- Session list and settings: `~/Library/Application Support/switchboard/state.json`.
- Worktrees: `~/Library/Application Support/switchboard/worktrees`. A session
  started in a repository where another session already works gets its own
  worktree on a new `switchboard/<name>` branch.
- Checkpoints: before each message you send, the session's folder is saved
  under `refs/switchboard/checkpoints` in its repository, without touching your
  branch. The button beside a message restores the files to that point.
- Log, when run as the app: `~/Library/Logs/Switchboard.log`.

Closing Switchboard ends every session's process. Conversations stay on disk
and resume on your next message, but a prompt that was waiting for an answer
is lost.
