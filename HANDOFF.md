# HANDOFF — ghosttycrt

**Date:** 2026-09-16 (later session)
**State:** **M0 complete and tested.** Nothing committed yet — repo has no commits.

## Resume in one line

Start **M1** (SSH create-or-attach) in `spec/09-milestones.md`. M1 is the value
line; no question blocks it.

## What landed this session

| Area | Path |
|---|---|
| Entry point, `tui` + `check` subcommands, `-config-dir` etc. | `cmd/gcrt/main.go` |
| XDG paths, config load + defaults + validation | `internal/config/` |
| Session model, load, validation, tree, fuzzy filter | `internal/session/` |
| tmux detection, `list-sessions` parsing, private socket `gcrt` | `internal/tmux/` |
| Bubbletea tree/details/status TUI | `internal/tui/` |
| Hand-written samples | `examples/{config,sessions}.toml` |
| Tests (tree, validation, fuzzy, view rendering) | `internal/{session,tui}/*_test.go` |

```
go build ./... && go test ./...
go run ./cmd/gcrt check -config-dir examples
go run ./cmd/gcrt tui   -config-dir examples
```

`go vet` clean, `gofmt` clean. Verified rendering under a pty: tree, details
pane, capability strip (`tmux ✓ vault infisical picocom ✓ ssh ✓`), and the
serial-device warning all appear.

## Questions — answered 2026-09-16

| | Question | Answer |
|---|---|---|
| **Q1** | tmux as a hard dependency? | **Yes** — installed (3.7c); D1 stands |
| **Q3** | Auto-login macros for serial/telnet? | **Off, and deferred past MVP** — Patrick wants it eventually; the field stays in the data model, the editor/executor do not ship with M1 |
| **Q5** | Private infra or public repo? | **Public** — `github.com/hazyumps/ghosttycrt`, commits signed as `hazyumps <patrick@fernlabs.net>` |
| **Q2** | Vaultwarden client: `rbw` or `vaultwarden-cli`? | still open — recommend `rbw`; blocks nothing before M4 |
| **Q4** | License | still open — recommend MIT |

## Decisions already locked (rationale in `spec/README.md`)

- **D1** tmux is the session-host backbone — only portable thing giving
  persistence *and* `pipe-pane` logging with one mechanism.
- **D2** Go. Single static binary; `tea.ExecProcess` is the suspend-attach-resume
  primitive.
- **D3** Portable display mode is **in-place attach** (`tmux attach` as a child);
  Ghostty tabs are a macOS accelerator only.
- **D4** Credentials are `provider + ref`, never values. No secret on disk, ever.
- **D5** Providers are argv templates in `config.toml`; neither vault hardcoded.

## M1 scope (from `spec/09-milestones.md`)

- [ ] `tmux` detection + friendly install message
- [ ] `transport` interface, ssh implementation (`spec/04-transports.md`)
- [ ] create-or-attach, `tea.ExecProcess`, detach returns to tree
- [ ] status glyphs read real tmux state (the `tmux` package already returns it;
      wire it into `renderRow`)
- [ ] `enter` / `d` (detach) / `q` with confirmation
- [ ] capability strip: tmux, ssh

Done when: `enter` on an SSH host drops you in, `Ctrl-b d` brings you back, and
the session is still there after quitting Ghostty.

## Gotchas — do not rediscover

1. **Ghostty CLI is not on `PATH`.** Read `display.ghostty.cli_path` from config;
   do not assume `ghostty` resolves.
2. **AppleScript is macOS-only.** It may only appear behind the `ghostty-tab`
   display mode, and must degrade silently to `inline` on Linux or when
   `macos-applescript = false` (AC-30).
3. **`TERM=xterm-ghostty` breaks old network gear.** Hosts without Ghostty's
   terminfo entry need `TERM=xterm-256color` per session. `ghostty +ssh`
   (1.4.0) auto-installs terminfo but the wrapper does not survive `screen`,
   `scp`, `git`, or scripts. The most likely "Ghostty broke my switch CLI" bug.
4. **`SSH_ASKPASS_REQUIRE=force` needs OpenSSH 8.4+** — present on macOS 13+ and
   any current Linux; removes the old `DISPLAY` requirement.
5. **Infisical flag spellings were not verified.** `--plain` is documented;
   `--env` / `--projectId` / `--path` are a guess living in a config template so
   being wrong is a one-line fix. Confirm at the start of M4.
6. **tmux integration tests must use a private socket** (`tmux -L gcrt-test`) —
   see T-8..T-13. The `tmux` package already takes the socket as a parameter.
7. **Bubbletea blocks under a pty that does not answer its startup queries**
   (`ESC]11;?` background-colour, `ESC[6n` cursor-position). A `script`-driven
   smoke run emits only those bytes and hangs. Test `Model.View()` directly
   instead; feed `ESC]11;rgb:…BEL` + `ESC[1;1R` on stdin if a real frame is
   needed.

## Environment — verified on this Mac, 2026-09-16

| | |
|---|---|
| Ghostty | **1.3.1** — AppleScript dictionary present |
| Ghostty CLI | **not on `PATH`** — `/Applications/Ghostty.app/Contents/MacOS/ghostty` |
| Go | 1.26.5 darwin/arm64 |
| **tmux** | **3.7c** — installed this session |
| **picocom** | **2024-07** — installed this session |
| infisical | present — `~/.local/bin/infisical` |
| rbw / bw | MISSING — pending Q2 |
| git signing | SSH, `commit.gpgsign=true`, key `~/.ssh/id_signing.pub` |

## Why build this rather than use something off the shelf

- **Tabby** (MIT) is the closest SecureCRT replacement but is Electron, not
  native, and does not drive Ghostty.
- **WezTerm** (MIT) is native with `wezterm serial`, but has no GUI host tree.
- **WindTerm** is only *partially* open source — fails a hard requirement.
- Every TUI SSH picker (`sshm`, `ssh-tui`, `lazyssh`, `ssh-manager`) *connects*
  rather than managing persistent, logged sessions.

The gap: a session tree + serial + auto-logging that runs identically inside
Ghostty on macOS and Linux, with vault-sourced credentials.

## Committed

Signed initial commit on `master` as `hazyumps <patrick@fernlabs.net>`. No remote
configured yet — `git remote add origin git@github.com:hazyumps/ghosttycrt.git`
before the first push.
