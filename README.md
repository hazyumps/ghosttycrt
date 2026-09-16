# ghosttycrt

A terminal session manager for network gear and servers — SSH, serial console,
session tree, and logging in one TUI, hosted on Ghostty.

Replaces SecureCRT for the parts that actually get used daily, and stays out of
the terminal-emulator business because Ghostty already does that better.

**Status:** M1 complete, plus `workspace` mode — **this is already a SecureCRT
replacement for SSH.** `enter` creates or reattaches a tmux session and hands
you the terminal; the session outlives `gcrt` and Ghostty. With
`display_mode = "workspace"` the tree stays pinned on the left and every session
you open tiles beside it, several live at once — click a pane to focus it,
`Ctrl-b t` to get back to the tree, `Ctrl-b d` to detach the lot. Serial, CRUD,
and logging are next ([`spec/09-milestones.md`](spec/09-milestones.md)).

## Build and run

```
go build ./cmd/gcrt          # or: go run ./cmd/gcrt tui
go run ./cmd/gcrt check      # validate config + sessions, no TUI
go test ./...
```

`gcrt` reads `~/.config/ghosttycrt/{config.toml,sessions.toml}`. Point it
elsewhere while trying it out with `-config-dir examples`.

## The shape of it

```
Ghostty  →  gcrt (TUI session manager)  →  tmux (session host)  →  ssh / picocom
```

- **Ghostty** renders. Native tabs/splits on macOS; any window on Linux.
- **gcrt** is the session tree, the launcher, and the log switch.
- **tmux** is the portable backbone: same on macOS and Linux, survives Ghostty
  quitting or the laptop sleeping, and gives us per-session logging for free.
- Credentials come from **Infisical** or **Vaultwarden** at connect time and are
  never written to disk.

## Non-goals

SFTP/SCP panels, X/Y/Zmodem, Wyse/SCO emulation, RDP/VNC, a password vault of
our own, AI. See `spec/01-overview.md`.

## Prerequisites

| | |
|---|---|
| Ghostty | 1.3.0+ (1.3.1 verified) |
| tmux | installed (3.7c) |
| Go | 1.26+ (verified go1.26.5 darwin/arm64) |
| picocom | installed — serial only |
| infisical CLI | credential provider (installed at `~/.local/bin/infisical`) |
| rbw | credential provider for Vaultwarden — `brew install rbw` |

## Spec index

| File | Covers |
|---|---|
| [`spec/README.md`](spec/README.md) | index, decisions, open questions |
| [`spec/01-overview.md`](spec/01-overview.md) | problem, goals, non-goals, users |
| [`spec/02-architecture.md`](spec/02-architecture.md) | layers, why tmux, process model, paths |
| [`spec/03-data-model.md`](spec/03-data-model.md) | config, session records, state, imports |
| [`spec/04-transports.md`](spec/04-transports.md) | ssh, serial, telnet, local; attach/detach |
| [`spec/05-credentials.md`](spec/05-credentials.md) | Infisical, Vaultwarden, askpass, macros |
| [`spec/06-logging.md`](spec/06-logging.md) | pipe-pane, timestamps, rotation, viewer |
| [`spec/07-tui.md`](spec/07-tui.md) | views, keybinds, status, theming |
| [`spec/08-acceptance.md`](spec/08-acceptance.md) | acceptance criteria and test list |
| [`spec/09-milestones.md`](spec/09-milestones.md) | M0–M6 build order |
