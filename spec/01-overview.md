# 01 — Overview

## Problem

SecureCRT does four jobs: terminal emulation, protocol clients, session
management, and session tooling (logging, scripting, file transfer).

The expensive part — a correct terminal emulator — is already solved by Ghostty,
which is native, GPU-accelerated, and better at it than SecureCRT. The protocol
clients are already solved by `ssh`, `picocom`, and `telnet`, which ship
everywhere and are scriptable.

What is missing is the layer nobody gives away for free: **a session manager**.
A tree of hosts, one keystroke to connect, per-session settings, auto-logging,
and credentials pulled from a vault instead of retyped.

`gcrt` builds only that layer.

## Goals

| | |
|---|---|
| **G1** | Manage a tree of sessions (groups, tags, search, favourites, recents). |
| **G2** | Connect to SSH hosts and serial consoles in one or two keystrokes. |
| **G3** | Keep sessions alive across Ghostty restarts, laptop sleep, and network blips. |
| **G4** | Auto-log each session to dated files, one toggle per session. |
| **G5** | Pull passwords from Infisical or Vaultwarden at connect time; never store them. |
| **G6** | Run identically on macOS and Linux Ghostty, with a copyable config. |
| **G7** | Be pleasant to look at and fast enough to feel instant. |

## Non-goals

| | |
|---|---|
| **NG1** | SFTP/SCP file browser. Use `sftp`, `rclone`, or `Finder`. |
| **NG2** | X/Y/Zmodem/Ymodem. If IOS images over console become necessary, that is its own project. |
| **NG3** | Wyse 50/60, SCO ANSI emulation. Ghostty is xterm/VT only; not needed for the target gear. |
| **NG4** | RDP, VNC, Spice. |
| **NG5** | Our own password vault or crypto. Providers only. |
| **NG6** | AI features, telemetry, accounts, sync services. |
| **NG7** | Being a terminal emulator. Ghostty is the emulator. |

## Users

Primarily one: Patrick, at a Mac, managing lab servers over SSH and network gear
over serial console, who wants to stop paying for SecureCRT and stop retyping
passwords. Secondarily the same person on a Linux workstation.

## Constraints

- **No secrets on disk.** Config holds references; see `05-credentials.md`.
- **Portability is a first-class requirement**, not an afterthought. Anything
  macOS-only must be opt-in and must degrade cleanly on Linux.
- **External dependencies are limited to well-known unix tools**: `tmux`, `ssh`,
  `picocom`, `telnet`, and the credential CLIs. No daemons, no servers, no
  database engine.
- **Config is hand-editable and diffable.** A text file you can `git diff`, not
  a binary blob, because it will be synced between machines.

## What "done" looks like

Open Ghostty, run `gcrt`, arrow to `core-sw-01`, press enter, and you are on the
console with a log already open on disk. Detach, pick `esxi-02`, press enter, and
you are on SSH with the password fetched from Infisical. Copy
`~/.config/ghosttycrt/` to the Linux box, `brew install tmux picocom` there, and
nothing else changes.

## Prior art

Worth reading before building, for what to borrow and what to avoid:

- `sshm`, `ssh-tui`, `lazyssh`, `ssh-manager` — TUI SSH pickers. All *connect*
  rather than manage persistent sessions; none drive Ghostty or log.
- `termscp` — a full TUI SFTP client. Proof the "pretty TUI over a protocol"
  shape works, and the size of the thing we are deliberately excluding (NG1).
- `tmuxinator`, `teamocil` — declarative tmux session layouts. Closest in spirit;
  YAML-first, no TUI, no credentials.
- Tabby — the closest all-round SecureCRT replacement, Electron, no Ghostty.
