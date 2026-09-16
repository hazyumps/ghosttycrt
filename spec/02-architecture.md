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
| `workspace` | macOS + Linux | tree and sessions as panes of one tmux session | several sessions visible at once, tree pinned left |
| `ghostty-tab` | macOS only | AppleScript `new tab in front window` | keeps the tree visible in its own tab; requires Automation permission (TCC) |
| `ghostty-window` | macOS + Linux | spawn `ghostty` with the attach as its command | best-effort; verify the exact flag per platform at M5 |

`inline` is the contract. The others are conveniences and allowed to be
absent — if AppleScript is disabled (`macos-applescript = false`) or the
platform is Linux, `gcrt` silently falls back to `inline`. No feature may
require a display mode other than `inline`.

### `workspace`

`gcrt` starts itself inside a tmux session of its own (`gscrt/workspace`) on the
private `gcrt` socket, then runs its TUI in pane 0. `enter` ensures a pane for
the session — created as a background window, then `join-pane`d in — and focuses
it, so the tree stays visible on the left and sessions tile to its right
(`main-vertical`, tree pinned at `workspace_tree_width`). Hiding a session
`break-pane`s it back into a background window: still running, off screen. Every
pane is tagged with `@gcrt_slug`, which is how the tree maps panes back to
sessions.

Consequences worth knowing:

- `Ctrl-b d` detaches the whole workspace; `q` does the same, because quitting
  would kill the panes that *are* the sessions. Shutting the workspace down is a
  confirmed action in the `d` menu.
- A crashed connection leaves a dead pane showing its exit status
  (`remain-on-exit`), rather than vanishing.
- `mouse on` gives click-to-focus panes and draggable borders, and costs
  drag-to-select (use Shift-drag).
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
