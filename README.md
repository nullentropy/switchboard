# [Switchboard](https://nullentropy.github.io/switchboard/)

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

Linux:

- An X11 or XWayland desktop with OpenGL, and the ALSA library. On Debian
  and Ubuntu these come from `libgl1`, `libx11-6` and `libasound2`.

Windows:

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
`Switchboard.app` and quitting it first if it is running. Set
`SWITCHBOARD_DEST` to install in another folder. curl does not mark the
download as quarantined, so macOS opens the app without asking.

Or install from a release zip by hand. Unzip
`Switchboard-<version>-macos-universal.zip` and move `Switchboard.app` to
Applications.

The app is ad hoc signed, not notarized. macOS quarantines a copy downloaded
in a browser and will refuse to open it. Either try to open it once, then go
to System Settings > Privacy & Security and choose Open Anyway, or clear the
quarantine flag:

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
`-demo-sessions` sets how many pretend sessions `-demo` starts. The default
is 7.

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

On macOS the script builds arm64 and amd64 binaries, joins them into one
universal app, sets the version in its `Info.plist` and signs it ad hoc. On
Linux it
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
| `Cmd+L` | Write in the open session, or ask the dispatcher |
| `J` `K` | Move between sessions |
| `Enter` | Open the selected session, write in the open one, or send the answers to a question |
| `Cmd+1` to `Cmd+9` | Open a session by position |
| `A` or `Cmd+Y` | Allow what the selected or first waiting session asks |
| `Shift+A` or `Cmd+Shift+Y` | Always allow it for that session |
| `D` or `Cmd+Shift+D` | Deny it |
| `Cmd+J` | Jump to the next session that needs you |
| `.` | Interrupt the turn |
| `Cmd+E` | Edit the newest pending message |
| `Cmd+Shift+E` | Interrupt and send pending messages now |
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

Its tools are served over MCP by Switchboard on `127.0.0.1`, on a random
port and path chosen at startup. Each Claude session gets its own random
path on the same port, which offers only `show_widget` and acts only on that
session.

Session processes run at a lower priority than the dispatcher and the
window. Switchboard checks CPU and memory every 15 seconds while a session
is working. It tells the dispatcher when the machine becomes overloaded and
when it recovers. The dispatcher then holds back new sessions and can pause
the busiest one that is building or testing.

### Widgets

Claude sessions and the dispatcher can show widgets in their conversation
with the `show_widget` tool: a table, progress bars, a list of keys and
values, a bar or line chart, a row of buttons, or a short form. The tool
never asks for permission. Switchboard draws every widget itself from a
fixed schema, in plain text with an optional good, warn or bad tone. A widget
cannot open links or files, and it cannot draw outside its card in the
conversation.

The agent updates a widget by showing it again with the same id. Clicking a
button or submitting a form locks the widget and sends the agent a message
with the answer. The message does not appear in the conversation. If the
session is working or waiting on a permission prompt, the answer waits in
the pending list until the turn ends. Clearing it there makes the widget
answerable again. A widget the user answered cannot be changed.

Limits per call: 64 KB of input, 500 rows and 12 columns in a table, 200
characters per cell, 200 points per chart series and two series, 12 form
fields and 6 buttons. A conversation holds up to 50 widgets. Widgets are
rebuilt from Claude's transcript after a restart, and unanswered ones still
work. Codex and Gemini sessions do not get the tool.

When the dispatcher shows a widget, the strip says so and Open shows its
conversation.

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

### Interfaces

Ask the dispatcher to change how the window is laid out or what it shows,
such as a denser board or a rail row with each session's cost, and it saves
a named interface and switches to it. An interface can replace the title
bar, the board, each session's tile, each session's row in the side rail,
and what shows when there are no sessions. Anything it leaves out stays as
it was. The dispatcher command bar, an open conversation, permission
prompts, dialogs, Settings, the command palette and the menus always stay
the same.

An interface reads the same session details the dispatcher sees and can
only use Switchboard's own actions, such as opening a session or answering a
prompt. It cannot read files, run commands or reach the network. If part of
it fails to draw, that part shows the default with a short notice.

Interface in Settings picks Default or a saved interface, and Use default
interface in the command palette switches back. To start once with Default
without changing the saved choice, run `switchboard -default-ui` or hold
Shift while Switchboard starts. If Switchboard quits within ten seconds of
starting with an interface on, twice in a row, it switches to Default and
tells the dispatcher. The interface stays saved.

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
