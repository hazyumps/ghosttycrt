# 04 — Transports

A transport turns a session record into a tmux session. That is its only job:
`(Session) -> argv`, plus whatever environment and tmux options the connection
needs.

## Interface

```
type Transport interface {
    Name() string
    Argv(s Session, secret string) ([]string, []string, error)  // argv, extra env
    NeedsSecret() bool
    TmuxOptions(s Session) map[string]string                     // set-option pairs
    Validate(s Session) error
}
```

`Argv` never receives a secret for transports that do not need one. When a
secret is present it is passed via **environment or a pipe, never as an argv
element** — argv is world-readable in `ps` for the lifetime of the process.

## tmux session naming

`tmux` session name = `gscrt/<slug>`. The `gscrt/` prefix keeps our sessions
apart from anything else on the server, and slash is a legal tmux session
character. `gcrt` lists only sessions matching the prefix, so it never touches
sessions it did not create.

Creation is always idempotent and attach-or-create:

```
tmux new-session -d -s gscrt/<slug> -c <start_dir> <argv…>
```

If the session already exists, it is attached to rather than recreated — so
`enter` on a running session is "go back to it", which is the single most-used
behaviour in the app.

## Transports

### ssh

```
ssh [-p <port>] [-i <identity>] [-J <jump>] [-o SendEnv=…] <user>@<host>
```

- `identity` empty → omit `-i`, let the agent and `~/.ssh/config` decide.
- `jump` → `-J`. (Session-level `jump` wins over `~/.ssh/config`'s
  `ProxyJump`; the config file is a suggestion, the session record is the
  truth.)
- `TERM`: set `SetEnv TERM=xterm-256color` is **not** added by default. Ghostty's
  `xterm-ghostty` works on hosts with the terminfo entry; for gear that lacks it,
  set an `env` map on the session (`03-data-model.md`) with
  `TERM=xterm-256color`. Wrong-`TERM` on old network gear is the single most
  common cause of "Ghostty broke my switch CLI". Consider a global
  `transports.ssh.legacy_term = true` toggle at M2.
- `extra_args` from `config.toml` are appended, so `ServerAliveInterval` and
  friends are set in one place.
- Secret handling: only when the session authenticates by password (see
  `05-credentials.md`).

### serial

Backend is `picocom` (preferred) or `screen` (fallback; both are installed on
this Mac as of 2026-09-16).

```
picocom -b <baud> [-d <databits>] [-y <parity>] [-f <flow>] <device>
screen  <device> <baud>[,<databits><parity><stopbits>]
```

- `picocom` is preferred: active, predictable, `Ctrl-a Ctrl-x` to quit cleanly,
  and its exit status is meaningful so `remain-on-exit` shows real failures.
- `screen`: `Ctrl-a k` to kill a wedged console, `Ctrl-a \` to quit. Its
  `-L` logging is not used — logging is tmux's job (`06-logging.md`), so there
  is exactly one mechanism.
- Required tmux options for serial sessions:
  - `remain-on-exit on` — when the adaptor is unplugged or `picocom` exits, the
    pane stays and shows why, instead of vanishing.
  - `status off` optionally, for a full-screen console feel.
- `device` that does not exist → fail fast *before* creating the tmux session,
  with the list of `/dev/tty.*` candidates shown.

### telnet

```
telnet <host> [<port>]
```

Included because some gear never grew SSH. No credential integration (telnet has
no auth channel to hook); login goes through the on-connect macro if configured.
Warn in the UI: telnet is cleartext.

### local

```
$SHELL
```

A local shell as a managed, persistent, logged session. Cheap to support once
tmux handles the rest, and useful for "a shell that survives Ghostty quitting".

## Attach and detach

`inline` (default, portable):

1. `tmux new-session -d …` ensures the session exists.
2. `tea.ExecProcess(exec.Command("tmux", "attach", "-t", name))`.
3. Bubbletea releases the terminal, the attach takes it, you work.
4. `Ctrl-b d` detaches → the child exits → the callback redraws the tree.

`gcrt` sets a one-line reminder in the status bar: **`Ctrl-b d` returns to the
tree.** Nobody remembers the second time.

`ghostty-tab` (macOS): `osascript` creates a Ghostty tab whose `command` is the
attach. The tree stays visible in its original tab. Requires Automation
permission; if denied or if `macos-applescript = false`, fall back to `inline`
with a one-time notification, never an error.

`ghostty-window` (both): spawn a Ghostty window running the attach. On macOS,
`open -na Ghostty.app --args -e <cmd>`; on Linux, invoke the `ghostty` binary
with the equivalent "run this command" flag. **Exact flags to be verified at
M5** — this mode is explicitly best-effort and must not block any milestone.

## Killing a session

`d` in the tree offers, with confirmation:

- **Detach** (default) — leave it running.
- **Kill session** — `tmux kill-session -t gscrt/<slug>`; the process goes too.
- **Kill and remove from tree** — deletes the record and the state entry. Logs
  are kept (they are the record of what happened).

Destructive options are never the default and are never bound to a bare key.

## Failure behaviour

| Failure | Behaviour |
|---|---|
| tmux not installed | refuse to start, print the install command for the detected platform |
| `ssh` not found | transport unavailable; session row shows it, `enter` explains |
| `picocom` and `screen` both missing | serial sessions unavailable with install hint |
| serial device absent | fail before creating the session; list candidate devices |
| credential fetch fails | do not connect; show provider stderr verbatim (it is usually the real answer) |
| tmux server unreachable | reconnect attempt once, then fail with the tmux error |
