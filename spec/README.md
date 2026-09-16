# ghosttycrt — spec index

The contract for what `gcrt` is, before any code exists. Read in order; each
file is self-contained.

## Documents

| | | |
|---|---|---|
| `01-overview.md` | problem, goals, non-goals, users | what and why |
| `02-architecture.md` | layers, process model, paths, dependencies | the decisions that are expensive to reverse |
| `03-data-model.md` | config, sessions, state, imports | what lives on disk |
| `04-transports.md` | ssh, serial, telnet, local | how a session starts and how you get back |
| `05-credentials.md` | Infisical, Vaultwarden, askpass | how secrets reach a session and never the disk |
| `06-logging.md` | pipe-pane, rotation, viewer | session capture |
| `07-tui.md` | views, keybinds, theming | the pretty part |
| `08-acceptance.md` | acceptance criteria, test list | how we know it works |
| `09-milestones.md` | M0–M6 | build order |

## Naming

- repo: `ghosttycrt`
- binary: `gcrt`

## Decisions taken

**D1 — Backbone is tmux.** The only portable session host that gives us
persistence *and* per-pane logging (`pipe-pane`) with one mechanism on macOS and
Linux. Ghostty's AppleScript API is macOS 1.3+ only, so it cannot be the
backbone; it is an accelerator. Cost: a tmux dependency. See `02-architecture.md`.

**D2 — Language is Go.** Single static binary copyable to the Linux box, and
`tea.ExecProcess` in bubbletea is exactly the suspend-TUI / run-child /
resume-on-detach primitive the design needs. Rust/ratatui is the runner-up.

**D3 — Portable display mode is in-place attach.** The TUI suspends, `tmux
attach` takes the terminal as a child process, `Ctrl-b d` detaches and the TUI
resumes. Zero Ghostty-specific code, works everywhere. Native Ghostty tabs are
an optional macOS extra.

**D4 — Credentials are references, never values.** The session file stores
`provider + ref`; the secret is fetched by CLI at connect time and handed
straight to the transport. Nothing secret is ever written to config, logs, or
the session database. See `05-credentials.md`.

**D5 — Infisical first, Vaultwarden second.** Both are already available.
Providers are configurable command templates, so neither vendor is hardcoded and
`rbw` vs `vaultwarden-cli` is a config row, not a code change.

**D6 — A second portable display mode: `workspace`.** Added 2026-09-16 at
Patrick's request, after M1. Instead of suspending the tree and attaching in
place, `workspace` runs the tree as pane 0 of a gcrt-owned tmux session and
joins each connection in as a pane beside it, so several sessions are live on
screen at once. It is still pure tmux — no Ghostty coupling, Linux unaffected —
so it extends D3 rather than replacing it. `inline` remains the default and the
contract; `workspace` is opt-in per `display_mode`, and `q` detaches rather than
quitting because killing the workspace would take every session with it. See
`02-architecture.md`.

## Open questions

**Q1 — tmux acceptable as a hard dependency? — RESOLVED 2026-09-16: yes.**
Installed (3.7c on the Mac, one `apt install tmux` on the Linux box). D1 stands.

**Q2 — Vaultwarden client: `rbw` or
[`haydonryan/vaultwarden-cli`](https://github.com/haydonryan/vaultwarden-cli)?**
Not yet decided, and it does not block M1-M3. `rbw` is agent-based (unlock once,
instant after), Rust, and Vaultwarden-tested. The linked CLI is simpler but
stateless. Recommendation: `rbw`, with the other as a config template.

**Q3 — Auto-login macros for network gear. — RESOLVED 2026-09-16: off, and
post-MVP.** Patrick wants it eventually, but it is not worth MVP cycles. The
`[session.on_connect]` field stays in the data model (`03-data-model.md`) so
files keep parsing; the form editor and the executor are deferred until after
M1 ships. Never on by default — it means the secret transits the tmux server.

**Q4 — License.** Unlicensed so far. Public repos use
`hazyumps <patrick@fernlabs.net>`. Recommend MIT.

**Q5 — Private infra or public repo? — RESOLVED 2026-09-16: public.**
Repo is `github.com/hazyumps/ghosttycrt`; commits are signed as
`hazyumps <patrick@fernlabs.net>`. Specs and docs stay as frank as they are.

## Answered

| | Answer |
|---|---|
| Q1 tmux hard dependency | yes |
| Q3 auto-login macros | off; deferred past MVP |
| Q5 public repo | yes, `hazyumps` identity |
| Q2 vaultwarden client | open — recommend `rbw` |
| Q4 license | open — recommend MIT |
