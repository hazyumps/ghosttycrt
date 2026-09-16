# 08 — Acceptance criteria

Each criterion is testable without reading the implementation. `T-n` is the test
that proves it.

## Session management

- **AC-1** `gcrt` lists every session in `sessions.toml`, grouped by `group`, with
  pinned sessions first within their group. `T-1`
- **AC-2** Filter matches on name, host/device, group, and tags, case-insensitive
  and fuzzy. `T-2`
- **AC-3** Create, edit, duplicate, pin, and delete a session round-trip through
  `sessions.toml` and survive a restart. `T-3`
- **AC-4** `sessions.toml` is written atomically; interrupting a save leaves the
  previous file intact. `T-4`
- **AC-5** A config with a duplicate `slug`, a mismatched transport block, or a
  literal-looking secret in a credentials block fails validation with a message
  naming the offending session. `T-5`
- **AC-6** Importing a `~/.ssh/config` proposes sessions and writes nothing until
  confirmed; wildcard stanzas are reported as skipped. `T-6`
- **AC-7** Importing a SecureCRT XML export maps host/user/port/group and
  discards every password field, listing sessions that now need a credential
  ref. `T-7`

## Connecting

- **AC-8** `enter` on an SSH session creates `gscrt/<slug>` if absent and attaches
  if present; pressing `enter` twice returns to the *same* running session. `T-8`
- **AC-9** `Ctrl-b d` from an attached session returns to the tree with the
  session marked detached-but-running. `T-9`
- **AC-10** A session survives `gcrt` exiting and Ghostty quitting; a new `gcrt`
  shows it as running and can reattach. `T-10`
- **AC-11** Serial sessions launch `picocom` when present and `screen` when not,
  using the session's baud/framing. `T-11`
- **AC-12** A missing serial device fails before any tmux session is created and
  lists candidate `/dev/tty.*` devices. `T-12`
- **AC-13** `gcrt` refuses to start without tmux, printing the platform's install
  command. `T-13`

## Credentials

- **AC-14** A session with `provider = "infisical"` authenticates over SSH using
  the password returned by the configured command template; `sessions.toml` is
  unchanged afterwards. `T-14`
- **AC-15** The same with `provider = "vaultwarden"` and an `rbw` template. `T-15`
- **AC-16** The secret never appears in any argv: `ps` shows no secret while an
  authenticated session is connecting. `T-16`
- **AC-17** The secret never appears on stdout/stderr of `gcrt` or in any file
  under the config, state, or log directories. `T-17`
- **AC-18** A failing provider aborts the connection and shows the provider's
  stderr; no session is created. `T-18`
- **AC-19** Changing only `config.toml` swaps the provider command (e.g.
  `infisical` ↔ `rbw`), with no code change. `T-19`
- **AC-20** The askpass helper declines to answer a non-password prompt. `T-20`
- **AC-21** An `on_connect` macro is off unless explicitly enabled per session,
  and a `redact = true` step leaves no trace of the secret in the log or the
  tmux buffer list. `T-21`

## Logging

- **AC-22** A session with logging enabled writes a dated file under the log
  directory containing the device's output. `T-22`
- **AC-23** Logging is idempotent: reattaching three times yields no duplicated
  lines. `T-23`
- **AC-24** Toggling logging on a live session starts/stops capture without
  disconnecting. `T-24`
- **AC-25** After a tmux server restart, the next attach re-establishes the pipe
  for every session with logging enabled in `sessions.toml`. `T-25`
- **AC-26** Log files are mode `0600` and contain no secret for a
  `redact = true` macro session. `T-26`
- **AC-27** `l` opens today's log and `q` returns to the tree. `T-27`
- **AC-28** Rotation produces `<slug>.<date>.<n>.log` once the cap is exceeded. `T-28`

## Portability

- **AC-29** `GOOS=linux GOARCH=arm64 go build` produces a binary that runs on
  Linux and performs AC-8, AC-9, AC-11, AC-22 unchanged. `T-29`
- **AC-30** No code path outside the `ghostty-tab` display mode invokes
  AppleScript or any macOS-only API; a build with that mode compiled out still
  compiles and passes AC-8. `T-30`
- **AC-31** Copying `~/.config/ghosttycrt/` between macOS and Linux yields the
  same session tree; only `state.toml` differs. `T-31`
- **AC-32** `display_mode = "ghostty-tab"` on Linux falls back to `inline`
  without error. `T-32`

## Interface

- **AC-33** The tree redraws under one frame with 500 sessions. `T-33`
- **AC-34** While a credential is being fetched the UI remains responsive and
  shows progress; it never blocks. `T-34`
- **AC-35** Every destructive key opens a confirmation whose default is the safe
  action. `T-35`
- **AC-36** With `theme = "auto"`, colours match the Ghostty config. `T-36`

## Test list

Strategy: the whole thing is subprocess orchestration, so the tests are mostly
integration tests against real `tmux` and a fake transport, plus a stubbed
provider binary. The TUI gets a `teatest` golden test for the tree and keybinds.

| Test | Kind | Notes |
|---|---|---|
| `T-1`..`T-7` | unit + golden | config load/validate/import on fixture files; no subprocesses |
| `T-8`..`T-10` | integration | real `tmux` in a temp socket (`tmux -L gcrt-test`); a `sleep`-based fake transport stands in for ssh |
| `T-11`, `T-12` | integration | fake `picocom` on `PATH`; `TestMain` skips if `tmux` absent |
| `T-13` | integration | run with a `PATH` containing no tmux |
| `T-14`..`T-19` | integration | stub provider binaries that print a known string; assert on process env/argv via a wrapper `ssh` that dumps its env |
| `T-20` | unit | askpass helper with a non-password prompt exits non-zero |
| `T-21` | integration | macro against a fake console; assert tmux buffer list is empty and log lacks the secret |
| `T-22`..`T-28` | integration | real tmux, real pipe-pane, temp dirs |
| `T-29`, `T-30` | build | `go build` cross, plus a `//go:build !darwin` compile of the core |
| `T-31`, `T-32` | unit | path resolution and display-mode selection |
| `T-33`..`T-36` | golden / teatest | bubbletea frames; benchmark for T-33 |

Integration tests use a private tmux socket (`tmux -L gcrt-test`) so they never
touch the user's real sessions, and tear it down in `TestMain`.
