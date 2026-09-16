# 02 — Architecture

## Layers

```
┌──────────────────────────────────────────────────────────────────┐
│ Ghostty 1.3.1+  — emulator, native tabs/splits, theming          │
│                                                                  │
│   ┌────────────────┐        ┌──────────────────────────────┐    │
│   │ gcrt TUI       │        │ attached session             │    │
│   │ (session tree) │        │ `tmux attach -t gscrt/<slug>`│    │
│   └───────┬────────┘        └──────────────▲───────────────┘    │
└───────────┼───────────────────────────────-┼────────────────────┘
            │ creates / drives                │ attaches
            ▼                                 │
   ┌──────────────────────────────────────────────────────┐
   │ tmux server — one session per host, persists         │
   │   gscrt/core-sw-01   → `picocom -b 9600 /dev/...`    │
   │   gscrt/esxi-02      → `ssh root@10.1.3.20`          │
   │        └─ pipe-pane ──▶ ~/.local/share/ghosttycrt/logs│
   └──────────────────────────────────────────────────────┘
            ▲
            │ secret on demand, stdout only
   ┌────────────────────────┐
   │ credential providers   │
   │  infisical  |  rbw     │
   └────────────────────────┘
```

Three layers, each replaceable:

1. **Display** — Ghostty. Never touched except for the optional macOS tab mode.
2. **Host** — tmux. Owns the process, the pseudoterminal, and the log stream.
3. **Control** — `gcrt`. Owns the session tree, the vault lookups, and the keys.

## Why tmux is the backbone

The requirement "portable to Linux Ghostty" kills the obvious design. Ghostty's
AppleScript dictionary — `new tab`, `split`, `input text`, `set_tab_title` — is
**macOS-only and 1.3.0+**. Building on it would produce a Mac app with a Linux
port, which is exactly what D3 in `spec/README.md` forbids.

tmux solves four problems at once with one dependency:

| Need | How tmux provides it |
|---|---|
| Sessions survive Ghostty quitting / sleep | sessions live outside Ghostty, in the tmux server |
| Attach from anywhere, detach at will | `tmux attach` / `Ctrl-b d` |
| Per-session logging | `pipe-pane` — a shell command fed every byte of pane output |
| One mechanism for ssh *and* serial | a session is just a command run in a pane |

Nothing else in the candidate set does all four portably. `abduco`/`dtach` are
lighter but have no logging and no window model; `screen` is present on the Mac
but its scripting and logging are far worse and it is effectively unmaintained.

**Cost:** tmux is currently *not installed* on this Mac (see Q1). It is one
`brew install tmux` here and one `apt install tmux` on the Linux box.

## Why Go

- **One static binary.** `GOOS=linux GOARCH=arm64 go build` and `scp` it to the
  Linux box. No runtime, no virtualenv, no node_modules. This is the whole
  portability story.
- **`tea.ExecProcess`.** Bubbletea's Exec primitive hands the real terminal to a
  child process, then redraws the TUI when it exits — precisely the
  suspend-attach-resume cycle D3 requires, already written and tested.
- **Subprocess ergonomics.** Every transport and every credential provider is a
  subprocess; Go's `os/exec` plus `context` handles argv, environment, and
  timeouts without ceremony.
- **Static typing for the session model** without Rust's build times.

Runner-up: Rust + ratatui. Choose it only if the binary-size or performance
argument becomes real; development speed is the tiebreaker and it favours Go
here.

## Process model

`gcrt` runs as a normal foreground TUI inside one Ghostty pane. It is *not* a
daemon and holds no state that matters after it exits — all durable state is in
the tmux server and the config files.

Connecting to a session:

```
gcrt                          fork/exec
 │                                │
 ├─ resolve session ──────────────┤
 ├─ fetch secret (if any) ──▶ infisical/rbw (child, stdout captured)
 ├─ ensure tmux session exists ──▶ tmux new-session -d -s gscrt/<slug> <argv>
 ├─ apply tmux options ──────────▶ tmux set-option / pipe-pane
 └─ display:
      inline (default)   → tea.ExecProcess("tmux attach -t gscrt/<slug>")
                           TUI suspends; Ctrl-b d returns to the tree
      ghostty-tab (macOS) → osascript: new tab with command = tmux attach …
      ghostty-window      → spawn a new Ghostty window running the attach
```

On `Ctrl-b d` the attach child exits, `tea.ExecProcess`'s callback fires, and the
tree redraws with the session now marked **attached: 0, running: 1**. Detach is
the "back" button, and it is free.

`gcrt` never blocks on a session and never proxies terminal bytes. It reads the
pseudoterminal only through tmux (`capture-pane`) when the log viewer asks.

## Display modes

| Mode | Platform | Mechanism | Notes |
|---|---|---|---|
| `inline` | macOS + Linux | `tea.ExecProcess` → `tmux attach` | default; zero Ghostty coupling |
| `workspace` | macOS + Linux | tree and connections as windows/panes of one tmux session | tabs (default) or tiled splits; several visible at once |
| `ghostty-tab` | macOS only | AppleScript `new tab in front window` | keeps the tree visible in its own tab; requires Automation permission (TCC) |
| `ghostty-window` | macOS + Linux | spawn `ghostty` with the attach as its command | best-effort; verify the exact flag per platform at M5 |

`inline` is the contract. The others are conveniences and allowed to be
absent — if AppleScript is disabled (`macos-applescript = false`) or the
platform is Linux, `gcrt` silently falls back to `inline`. No feature may
require a display mode other than `inline`.

### `workspace`

`gcrt` starts itself inside a tmux session of its own (`gscrt/workspace`) on the
private `gcrt` socket, then runs its TUI in a window named `tree`. Every
connection is a pane tagged with `@gcrt_slug`, which is how the tree maps panes
back to sessions. `workspace_layout` decides where that pane lives:

| Layout | How a connection appears |
|---|---|
| `tabs` | its own tmux window — a tab, with the tree in a tab of its own |
| `sidebar` | a window on a *second* server, whose status line is drawn at the top of a content pane beside the pinned tree |
| `split` | `join-pane`d into the tree window, tiled to the right of the tree |

In `split`, `main-vertical` pins the tree to `workspace_tree_width` and the
connections divide the remaining space; hiding one `break-pane`s it back into a
background window so the process keeps running. In the two tab layouts nothing
is hidden — a window *is* the tab — so `d` offers *Switch to it* and *Kill*
rather than Show/Hide. In `tabs` the outer status line is the tab bar; in
`sidebar` the inner one is, with `window-status-format` putting a `•` on a tab
whose output is waiting, and the tree window has `monitor-activity` off so its
own redraws never flag it.

#### Why `sidebar` runs two servers

A tmux tab is a window, and a window's status line spans the whole terminal.
There is therefore no way to draw a tab bar inside one *region* of a single
window: pane borders name one pane each and are not clickable per tab. The
content pane runs a second server (`gcrt-tabs`) whose status line is positioned
at the top, which makes it the tab bar for exactly that region.

The familiar nested-tmux hazard does not apply. The outer client consumes the
prefix key before any pane sees it, so the inner server has no reachable prefix
and is driven entirely by clicks on its tab bar and by gcrt's own commands. The
costs are real but contained: two servers to start, tear down, and reattach, and
a second place connection state lives.

The content pane runs an attach loop rather than a bare `tmux attach`, because
the tab server's session is created lazily on the first connection and dies with
the last one; the loop reports *no connections yet* and retries.

`Ctrl-b t` returns to the tree in every layout — as `select-window` in `tabs`,
and as `select-pane` in `sidebar` and `split`, where the tree shares the window.

Other consequences worth knowing:

- `Ctrl-b d` detaches the whole workspace; `q` does the same, because quitting
  would kill the panes that *are* the sessions. Shutting the workspace down is a
  confirmed action in the `d` menu, and tears down both servers.
- A crashed connection leaves a dead pane showing its exit status
  (`remain-on-exit`) rather than vanishing. Entering it again restarts it: a
  dead pane cannot be revived, so it is replaced.
- `mouse on` gives click-to-focus panes, clickable tabs, and draggable borders;
  it costs drag-to-select (use Shift-drag). Clicking a tab in the `sidebar`
  costs one click the first time — the first click only moves focus into the
  content pane.
- A detached workspace's panes persist, including the tree process, so
  re-running `gcrt` reattaches rather than rebuilding.

The macOS tab form, verified against the 1.3 AppleScript dictionary:

```applescript
tell application "Ghostty"
  set cfg to new surface configuration
  set command of cfg to "tmux attach -t gscrt/core-sw-01"
  set t to new tab in front window with configuration cfg
  perform action "set_tab_title:core-sw-01" on focused terminal of t
end tell
```

Note the CLI is not on `PATH` by default — it lives at
`/Applications/Ghostty.app/Contents/MacOS/ghostty`.

## On-disk layout

XDG paths on both platforms, because they are portable and diffable. macOS
happens to also support them; we prefer them over
`~/Library/Application Support` deliberately.

| Purpose | Path | Sync? |
|---|---|---|
| Main config | `~/.config/ghosttycrt/config.toml` | yes |
| Sessions | `~/.config/ghosttycrt/sessions.toml` | yes |
| Runtime state (recents, pins, last used) | `~/.local/state/ghosttycrt/state.toml` | no |
| Logs | `~/.local/share/ghosttycrt/logs/` | no |
| Cache (vault lookups, discovery) | `~/.cache/ghosttycrt/` | no |

`XDG_*` environment variables are honoured if set. Copying
`~/.config/ghosttycrt/` is the entire migration to another machine.

## Security posture

- Secrets exist only in the memory of `gcrt` and the child process, and in
  transit over a pipe. Not in `sessions.toml`, not in `state.toml`, not in logs.
- `gcrt` never writes a secret to a file it owns. If a transport needs a secret
  on stdin or argv, it is passed through a pipe, never an argv element.
- Log scrubbing (`06-logging.md`) is on by default for sessions whose transport
  carries a secret.
- The session tree contains hostnames and usernames. That is sensitive enough to
  not be public; see Q5.

## Dependencies

**Runtime, required:** tmux, ssh (OpenSSH 8.4+ for `SSH_ASKPASS_REQUIRE=force`).
**Runtime, per transport:** picocom (serial), telnet (telnet).
**Runtime, per provider:** infisical, rbw.
**Build:** Go 1.26+.
**Optional:** Ghostty CLI path (macOS tab mode), `moreutils` for `ts` (log
timestamping).
