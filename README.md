# Switchboard

Switchboard is an app that runs your coding agent sessions side by side.
Each session is a tile that shows what its agent is doing and asks you when it
needs permission.

## Requirements

On every system:

- The `claude` CLI on your `PATH`, signed in. Switchboard will not start
  without it, and the dispatcher runs on it.
- `git` on your `PATH`, for worktrees and checkpoints.
- Optional: the `codex` and `gemini` CLIs, for sessions that use those
  agents. Sign in to each once in a terminal first: `codex login`, or run
  `gemini` once (or set `GEMINI_API_KEY`).

Each agent uses its own login, settings and instruction files.

macOS:

- macOS 13 Ventura or newer for a release build. An app built with
  `scripts/app.sh` runs on the macOS version it was built on or newer.

Linux:

- A 64-bit x86 or ARM system with glibc 2.35 or newer, such as Ubuntu 22.04.
- An X11 or XWayland desktop with OpenGL, and the ALSA library. On Debian
  and Ubuntu these come from `libgl1`, `libx11-6` and `libasound2`.

Windows:

- 64-bit Windows 10 or 11.
- `claude` from the native installer, so that `claude.exe` is on your
  `PATH`. The npm package installs `claude.cmd`, which cannot pass the
  dispatcher its instructions.

To build from source:

- Go 1.26 or newer. The version comes from `go.mod`.
- A C compiler. On macOS, the Xcode Command Line Tools, which also provide
  `codesign` and `lipo`. On Debian and Ubuntu, `gcc`, `pkg-config`,
  `libgl1-mesa-dev`, `libx11-dev`, `libxcursor-dev`, `libxi-dev`,
  `libxinerama-dev`, `libxrandr-dev`, `libxxf86vm-dev` and
  `libasound2-dev`. On Windows, a MinGW-w64 `gcc` on your `PATH`.

## Install

### macOS

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
in a browser and will refuse to open it. Either right-click the app, choose
Open, and confirm, or clear the quarantine flag:

```
xattr -dr com.apple.quarantine /Applications/Switchboard.app
```

### Linux

```
curl -fsSL https://raw.githubusercontent.com/nullentropy/switchboard/main/install.sh | sh
```

This installs `~/.local/bin/switchboard` with a desktop entry and icons under
`~/.local/share`, so Switchboard appears in your app launcher. It quits a
running copy first. Set `SWITCHBOARD_DEST` to install under another prefix.

Or install from a release by hand. Unpack
`Switchboard-<version>-linux-amd64.tar.gz`, or the `arm64` one, into
`~/.local`:

```
tar -xzf Switchboard-<version>-linux-amd64.tar.gz -C ~/.local
```

### Windows

In PowerShell:

```
irm https://raw.githubusercontent.com/nullentropy/switchboard/main/install.ps1 | iex
```

This installs `Switchboard.exe` in `%LOCALAPPDATA%\Programs\Switchboard` and
adds Switchboard to the Start menu. It quits a running copy first. Set
`SWITCHBOARD_DEST` to install in another folder.

Or install from a release by hand. Unzip
`Switchboard-<version>-windows-amd64.zip` and run `Switchboard.exe`. The app
is not signed. Windows SmartScreen warns about a copy downloaded in a
browser. Choose More info, then Run anyway.

## Run from source

```
go run .                 # open the window
go run . -demo           # pretend sessions, nothing saved
go run . -addr :8790     # serve the UI to a browser instead of a window
```

Other flags: `-claude`, `-codex` and `-gemini` point at other binaries.
`-state` sets where the session list and settings are kept.

## Build the app

On macOS:

```
scripts/app.sh
```

This writes `dist/Switchboard.app`, ad hoc signed, for your Mac's
architecture. Drag it to Applications. When opened from Finder it takes `PATH`
from your login shell, so it finds the agent CLIs where your terminal does.

On Linux, the same script writes the binary, desktop entry and icons to
`dist/linux`. Copy them into place with `cp -r dist/linux/. ~/.local/`. When
started from the app launcher, Switchboard takes `PATH` from your login shell.

On Windows, in PowerShell:

```
scripts\app.ps1
```

This writes `dist\Switchboard.exe` with its icon.

## Make a release

Pushing a tag such as `v1.2.3` runs `.github/workflows/release.yml`. It builds
each platform on its own GitHub runner and attaches these files to the
release for that tag, creating the release if it does not exist:

- `Switchboard-<version>-macos-universal.zip` and
  `Switchboard-macos-universal.zip`
- `Switchboard-<version>-linux-amd64.tar.gz` and
  `Switchboard-linux-amd64.tar.gz`
- `Switchboard-<version>-linux-arm64.tar.gz` and
  `Switchboard-linux-arm64.tar.gz`
- `Switchboard-<version>-windows-amd64.zip` and
  `Switchboard-windows-amd64.zip`

`install.sh` and `install.ps1` download the files without a version in their
name.

To build one platform's files by hand:

```
scripts/release.sh          # macOS or Linux, version from the tag on HEAD
scripts/release.sh 1.2.3    # or pass the version
scripts\release.ps1 1.2.3   # Windows
```

The working tree must be clean. Without an argument, HEAD must have a tag; a
leading `v` is dropped. Each script builds from a clean checkout of HEAD and
writes to `release/`.

On macOS the script builds arm64 and amd64 binaries for macOS 13 or newer,
joins them into one universal app, sets the version in its `Info.plist` and
signs it ad hoc. macOS 13 is the oldest version Go 1.27 supports. On Linux it
builds for the machine's architecture and packs the binary, desktop entry and
icons. On Windows it builds `Switchboard.exe` with its icon and manifest.

## Reference

### Keyboard shortcuts

Press `?` or the Shortcuts button for the full list. On Linux and Windows,
press Ctrl where this table says Cmd.

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

Dispatcher model in Settings picks Default or one of the Claude models. It is
kept across restarts and takes effect on the dispatcher's next turn.

### Where things are stored

| | macOS | Linux | Windows |
|---|---|---|---|
| Session list and settings | `~/Library/Application Support/switchboard/state.json` | `~/.config/switchboard/state.json` | `%APPDATA%\switchboard\state.json` |
| Worktrees | `~/Library/Application Support/switchboard/worktrees` | `~/.local/share/switchboard/worktrees` | `%LOCALAPPDATA%\switchboard\worktrees` |
| Log, when run as the app | `~/Library/Logs/Switchboard.log` | `~/.cache/switchboard/switchboard.log` | `%LOCALAPPDATA%\switchboard\switchboard.log` |

On Linux, `XDG_CONFIG_HOME` moves the session list and settings,
`XDG_DATA_HOME` moves worktrees and `XDG_CACHE_HOME` moves the log.

A session started in a repository where another session already works gets
its own worktree on a new `switchboard/<name>` branch.

Before each message you send, the session's folder is saved under
`refs/switchboard/checkpoints` in its repository, without touching your
branch. The button beside a message restores the files to that point.

Closing Switchboard ends every session's process. Conversations stay on disk
and resume on your next message, but a prompt that was waiting for an answer
is lost.
