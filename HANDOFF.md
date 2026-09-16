# HANDOFF — ghosttycrt

**Date:** 2026-09-16
**State:** **M1 complete, committed** (`40a8685` is M0; M1 commit follows).
`enter` connects over SSH and `Ctrl-b d` returns. This is a working SecureCRT
replacement for SSH.

## Resume in one line

Pick **M2 (serial + CRUD)** or **M3 (logging)** — the spec says they are
independent and that living in M1 for a week should decide which. Then run
`gcrt` for real: point it at `~/.config/ghosttycrt/`, don't use the fixtures.

## What landed in M1

| Area | Path |
|---|---|
| `transport` interface + ssh argv, `-p/-i/-J`, extra_args | `internal/transport/` |
| tmux client: private socket `gcrt`, `gscrt/<slug>` names, `Ensure` (create-or-attach), `Attach`, `Detach`, `Kill`, `BySlug`, `InstallHint` | `internal/tmux/` |
| connect flow (`tea.ExecProcess`), status glyphs, `d` menu, quit confirm, modals | `internal/tui/` |
| tmux refusal with the platform install command (AC-13) | `cmd/gcrt/main.go` |

Status glyphs: `●` attached · `○` running · `·` not running · `✗` transport
unavailable (serial/telnet/local until M2).

```
go build ./... && go test ./...        # integration tests use a private socket
go run ./cmd/gcrt tui -config-dir examples
```

`go vet`/`gofmt` clean. Tests: ssh argv (incl. AC-16 "no secret in argv"),
real-tmux ensure/reuse/kill/options/pane-env, tree+filter+glyph rendering,
quit-confirm and kill-confirm against a live session.

## Decisions taken this session

- **`d` offers Detach and Kill only.** "Kill and remove from the tree" needs
  atomic `sessions.toml` writes, which are M2 — it is not shipped empty-handed.
- **Nothing pre-configures the tmux server.** A sessionless tmux server exits
  immediately, so `set-option -g` before the first session cannot work (see
  gotcha 8). M1 sets no TERM at all; M2 adds the per-session toggle.

## Next: M2 or M3 (from `spec/09-milestones.md`)

**M2 — serial + session CRUD.** picocom backend + screen fallback, framing,
missing-device check with `/dev/tty.*` candidates, `remain-on-exit`, the
form (new/edit/duplicate/delete/pin), atomic writes, `~/.ssh/config` importer
with preview diff, and the legacy `TERM=xterm-256color` toggle.

**M3 — logging.** `pipe-pane` on create and on every attach, dated `0600` files
with a header, per-session toggle, `less -R +F` viewer, rotation and pruning,
live log-size marker in the tree.

M3 is the one that makes the tool feel like SecureCRT; M2 is the one that makes
it useful for console work. Both are a few days.

## Open questions

| | Question | Status |
|---|---|---|
| **Q2** | Vaultwarden client `rbw` vs `vaultwarden-cli` | open — recommend `rbw`; blocks nothing before M4 |
| **Q4** | License | open — recommend MIT |

## Gotchas — do not rediscover

1. **Ghostty CLI is not on `PATH`.** Read `display.ghostty.cli_path` from config;
   do not assume `ghostty` resolves.
2. **AppleScript is macOS-only.** Only behind `ghostty-tab`, and it must degrade
   silently to `inline` (AC-30). Not implemented yet.
3. **`TERM` on old network gear.** tmux 3.7c sets panes to
   `default-terminal = tmux-256color` (not `screen` — that changed in tmux 3.4).
   Old switches know neither, so their CLI can come up garbled. **Verified fix
   for M2:** `tmux new-session -e TERM=xterm-256color …` sets the pane env
   (confirmed via `show-environment`). That is per-session and is the mechanism
   the `legacy_term` toggle should use. Do not bother with `set-option -g` —
   gotcha 8.
4. **`SSH_ASKPASS_REQUIRE=force` needs OpenSSH 8.4+** — present on macOS 13+ and
   any current Linux.
5. **Infisical flag spellings were not verified.** `--plain` is documented;
   `--env` / `--projectId` / `--path` are a guess living in a config template.
   Confirm against `infisical secrets get --help` at the start of M4.
6. **tmux integration tests must use a private socket** — `internal/tmux`'s
   `Client` takes the socket as a parameter, and tests use `gcrt-test-<pid>`.
7. **Bubbletea blocks under a pty that does not answer its startup queries**
   (`ESC]11;?`, `ESC[6n`). A `script`-driven smoke run hangs with no output.
   Test `Model.View()` directly; feed `ESC]11;rgb:…BEL` + `ESC[1;1R` on stdin
   only if a real frame is needed, and guard the run with a background `pkill`.
8. **A sessionless tmux server exits immediately.** `start-server` then
   `set-option -g` fails with `error connecting to …`. `list-sessions` on a
   socket with no server **exits 1** and prints `no server running on …` — the
   `tmux` package treats that as empty, not as an error. Keep it that way.

## Environment — verified on this Mac, 2026-09-16

| | |
|---|---|
| Ghostty | 1.3.1 |
| Go | 1.26.5 darwin/arm64 |
| tmux | 3.7c |
| picocom | 2024-07 |
| infisical | `~/.local/bin/infisical` |
| rbw / bw | MISSING — pending Q2 |
| git signing | SSH, `commit.gpgsign=true`, key `~/.ssh/id_signing.pub` |
| git identity | per-repo `hazyumps <patrick@fernlabs.net>` |

No remote configured. `git remote add origin
git@github.com:hazyumps/ghosttycrt.git` before the first push.
