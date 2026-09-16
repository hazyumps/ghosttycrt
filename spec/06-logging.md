# 06 — Logging

Logging is the feature that most justifies the tmux backbone: `pipe-pane` gives
every byte of a pane's output to a shell command, per session, on both macOS and
Linux, with no changes to the transport.

## Mechanism

At session creation, if logging is enabled:

```
tmux pipe-pane -t gscrt/<slug> -o \
  'cat >> "<logdir>/<slug>.<YYYY-mm-dd>.log"'
```

With `moreutils`' `ts` available and `logging.timestamp = true`:

```
tmux pipe-pane -t gscrt/<slug> -o \
  'ts "[%Y-%m-%d %H:%M:%S] " >> "<logdir>/<slug>.<YYYY-mm-dd>.log"'
```

`-o` means "only if no pipe is already open", so this is idempotent across
reconnects and reattaches: a session that is already logging is not
double-piped and does not get duplicate lines.

Turning the toggle off is `tmux pipe-pane -t gscrt/<slug>` with no command,
which closes the pipe. Toggling works on a live session without disconnecting.

## File layout

```
~/.local/share/ghosttycrt/logs/
  core-sw-01.2026-09-16.log
  esxi-02.2026-09-16.log
```

- One file per session per day, so a year of console logs is browsable by date
  and nothing needs a log-rotation daemon.
- `slug` in the name, so the file is identifiable without the session tree.
- Header line written at file creation:
  `# ghosttycrt <slug> <transport> <host|device> opened <ISO8601>` — hostnames
  and device paths only, never a user or secret.
- Logs are **not** in the synced config set and are machine-local.
- Permissions `0600`: console logs contain configuration and occasionally
  secrets typed by hand.

## What is and is not captured

| | |
|---|---|
| Captured | everything the remote device prints to the pane — the important direction for console work |
| Not captured | what you type, directly |
| Exception | with `tmux`'s default `echo`, locally-echoed input is present; on a serial console with local echo off, typed passwords are **not** in the log, which is the safer default and worth preserving |

This is the honest limitation versus SecureCRT, which can log input too. It is
also the reason log scrubbing is rarely needed on serial, and the reason
`on_connect` macro steps marked `redact = true` are safe: a console does not
echo a password, and the macro does not echo it either.

Scrubbing (`secret_scrub_logs = true`, default) is a filter for the cases where a
secret *could* land in the stream — an SSH session where the remote echoes, a
paste into a login prompt. The filter holds the current session's secret in
memory and rewrites any occurrence to `[REDACTED]` before the line reaches the
file. It is a belt-and-braces measure, not the primary control; the primary
control is that we never write a secret ourselves.

## Rotation

- `logging.rotate_mb` (default 50) rolls the day's file to
  `<slug>.<date>.<n>.log` when it exceeds the cap. Console logs are text and
  grow slowly; this exists to stop a `debug all` on a switch from filling a disk.
- `gcrt logs prune --older-than 90d` deletes old logs. Never automatic — log
  deletion is destructive and per the rules must be explicit.
- A warning appears in the TUI when total log size exceeds a configurable
  threshold.

## Viewer

`l` on a session opens a pager over that session's current log:

- Suspends the TUI and hands the terminal to `less -R +F` (follow mode), the
  same suspend/resume pattern as an attach, so `q` returns to the tree.
- `less +F` gives follow-tail for free, with `Ctrl-C` to stop following and
  `F` to resume — i.e. exactly the tail-a-console behaviour wanted, without
  writing a pager.
- If the session is live and logging, the viewer tails; if not, it opens the most
  recent log read-only.
- `L` opens the log *directory* in `$PAGER`/`ls` form, for picking an older day.

## Live log status in the tree

A session row shows logging state at a glance:

| Marker | Meaning |
|---|---|
| `●` | live, logging, pipe open |
| `○` | live, not logging |
| `·` | detached, session exists |
| `✗` | session dead / `remain-on-exit` holding a failure |

and the byte count of today's log beside it, so "is this being captured" is
answerable without opening anything.

## Failure behaviour

| Failure | Behaviour |
|---|---|
| log directory unwritable | session still connects; logging marker degrades to `○` and the reason is shown on first toggle |
| `ts` absent and `timestamp = true` | fall back to untimestamped, note it once in the status bar; do not fail |
| pipe dies (disk full) | session unaffected; marker goes to `○`; a warning is raised on next tree redraw |
| tmux server restarted | pipes are gone; `gcrt` re-applies `pipe-pane` for every session with logging enabled on next attach |

That last row is why `pipe-pane` is re-applied on every attach rather than only
at creation: the tmux server is not durable across a reboot, but the *intent* to
log is in `sessions.toml`, so the pipe is restored to match intent.
