# HANDOFF — ghosttycrt

**Date:** 2026-09-16 (fourth session)
**State:** **M1 + `workspace` mode + SecureCRT import.** Patrick's 120 real
sessions are imported and in daily use.

## Resume in one line

The tree now holds 120 real sessions; **47 of them had a SecureCRT password and
have no credential ref yet** (`gcrt import securecrt` lists them). M4
(credentials) is what makes those one-keystroke; M2 (serial + CRUD) is what makes
the tree editable without hand-editing TOML; M3 (logging) is what makes it feel
like SecureCRT.

## What landed in the import session

| Area | Path |
|---|---|
| `workspace_layout`: `tabs` (default), `sidebar`, or `split` | `internal/config/`, `internal/tmux/{workspace,sidebar}.go` |
| header menu bar (clickable Help / Filter / Refresh / Quit), scrollable help | `internal/tui/view.go` |
| SecureCRT `.ini` parser, field mapping, deterministic ids, slug dedup | `internal/importers/securecrt/` |
| `gcrt import securecrt [--from DIR] [--write]`, dry run by default | `cmd/gcrt/import.go` |
| TOML emitter for `sessions.toml` (round-trip tested) | `internal/session/write.go` |

```
gcrt import securecrt            # dry run: what would change
gcrt import securecrt --write    # backs the old file up, then writes atomically
```

Reads SecureCRT's native per-session `.ini` store (not an XML export). Folder
path → `group`; `Hostname`/`Username`/`[SSH2] Port` → `ssh`. `__FolderData__.ini`
and `Default.ini` are skipped. Passwords are never imported; sessions with
`Session Password Saved = 1` are listed as needing a ref. Ids are UUIDv5 over the
session's path, so re-importing is stable and never orphans state or logs.

Patrick's config: 120 sessions (119 ssh, 1 local shell), 47 flagged for a
credential ref, 1 with a live login script (`new-core-console-use this one`).
The previous hand-written `trainee-48` was dropped — SecureCRT's `eve-48.ini`
already covers `10.5.5.248`. Backup at
`~/.config/ghosttycrt/sessions.toml.bak-20260916-170036`.

## How `workspace` works (D6)

`display_mode = "workspace"` makes `gcrt` start inside its own tmux session,
`gscrt/workspace` on the private `gcrt` socket. The tree is pane 0; `enter`
creates a connection as a background window, `join-pane`s it in beside the tree
and focuses it. Hiding `break-pane`s it back to a background window — still
running, off screen. Every pane is tagged `@gcrt_slug`, which is how the tree
maps panes back to sessions.

Keys that differ from `inline`:

| | |
|---|---|
| `enter` | go to the connection's tab (`tabs`, `sidebar`) or open it beside the tree (`split`) |
| `d` | Switch to it / Kill, plus *Shut down workspace* (`tabs`, `sidebar`); Show / Hide / Kill in `split` |
| `q` | **detach** — the workspace keeps running |
| `Ctrl-b t` | back to the tree from a session pane |
| `Ctrl-b d` | detach the whole workspace (tmux's own) |
| mouse | click a tab in the status line to switch; in `split`, click a pane to focus and drag a border to resize |

Patrick's `~/.config/ghosttycrt/config.toml` is set to `workspace`, tree width 34.

## Known rough edges (deliberate, not bugs)

1. **The tree pane is full width until the first session opens.** tmux only
   applies `main-pane-width` from two panes up, so the window starts wide and
   snaps to 34 once something is joined. Cosmetic; the TUI handles the resize.
2. **Focus lands on the last-focused pane on reattach.** Normal tmux behaviour.
   If that is a session, `Ctrl-b t` returns to the tree.
3. **`mouse on` costs drag-to-select.** Use Shift-drag (or Option-drag).
4. **A dead pane is left in place** (`remain-on-exit`) showing its exit status,
   so a failed connection is visible rather than silent. Kill it from the `d`
   menu.
5. **`q` never quits in workspace mode.** Shutting down is the destructive
   action in the `d` menu, behind a confirmation.

## Next: M2 or M3 (from `spec/09-milestones.md`)

**M4 — credentials** is the one with a concrete list waiting: 47 imported
sessions had a SecureCRT password and have no credential ref. Until M4 they
connect on the agent/identity path only.

**M2 — serial + session CRUD.** picocom backend + screen fallback, framing,
missing-device check with `/dev/tty.*` candidates, `remain-on-exit` for serial,
the form (new/edit/duplicate/delete/pin), atomic writes, `~/.ssh/config` importer
with preview diff, and the legacy `TERM=xterm-256color` toggle — which should use
`new-session -e TERM=xterm-256color`, verified to work.

**M3 — logging.** `pipe-pane` on create and on every attach, dated `0600` files
with a header, per-session toggle, `less -R +F` viewer, rotation and pruning,
live log-size marker in the tree.

M3 is what makes it feel like SecureCRT; M2 is what makes console work possible.

## Open questions

| | Question | Status |
|---|---|---|
| **Q2** | Vaultwarden client `rbw` vs `vaultwarden-cli` | open — recommend `rbw`; blocks nothing before M4 |
| **Q4** | License | open — recommend MIT |

## Gotchas — do not rediscover

1. **Ghostty CLI is not on `PATH`.** Read `display.ghostty.cli_path`; do not
   assume `ghostty` resolves.
2. **`TERM` on old network gear.** tmux 3.7c sets panes to
   `default-terminal = tmux-256color`. Old switches know neither that nor
   `screen`, so their CLI can come up garbled. **Verified fix for M2:**
   `new-session -e TERM=xterm-256color` sets the pane env (confirmed with
   `show-environment`). Never `set-option -g` — gotcha 8.
3. **`SSH_ASKPASS_REQUIRE=force` needs OpenSSH 8.4+** — fine on macOS 13+ and
   current Linux.
4. **Infisical flag spellings were not verified.** Confirm
   `infisical secrets get --help` at the start of M4.
5. **tmux integration tests must use a private socket** — `Client` takes the
   socket as a parameter; tests use `gcrt-test-<pid>` / `gcrt-ws-<pid>`.
6. **Bubbletea blocks under a pty that does not answer its startup queries**
   (`ESC]11;?`, `ESC[6n`). Test `Model.View()` directly; feed
   `ESC]11;rgb:…BEL` + `ESC[1;1R` on stdin for a real frame, and guard any pty
   run with a background `pkill`. Also: `script` must be given a usable `TERM`
   or tmux refuses with *"open terminal failed: terminal does not support
   clear"*.
7. **A sessionless tmux server exits immediately**, so `start-server` followed
   by `set-option -g` fails. `list-sessions`/`list-panes` on a socket with no
   server **exit 1** — treat as empty, not as an error.
8. **Do not `TrimSpace` `list-panes` output before splitting it.** A live pane's
   trailing `pane_dead_status` is empty, so the last line ends in a tab and
   trimming silently drops that pane. This shipped as a bug; `parsePanes` has a
   regression test.
9. **argv survives tmux intact** — `new-session`/`new-window` use `execvp`, not
   `sh -c`. Verified with spaces, `|`, `$` and quotes. No shell-quoting layer
   is needed, and adding one would break it.
10. **`join-pane` has no `-P`.** Get the pane id from `new-window -P -F
    '#{pane_id}'` (which does support it), or from `list-panes`.
11. **`@gcrt_slug` pane options survive `join-pane` and `break-pane`.** Use them
    for identity; `pane_title` is overwritten by whatever the pane runs.
12. **`select-pane` does not cross windows.** In `tabs` layout the tree lives in
    another window, so `prefix t` must be `select-window -t gscrt/workspace:tree`;
    `select-pane -t %0` silently stays put. (`select-pane` still works in `split`,
    where the tree shares the window.)
13. **A dead pane cannot be revived.** `remain-on-exit` keeps it visible with its
    status, but entering it again must `kill-pane` and rebuild — `respawn-pane`
    would need the argv again, and reusing the pane leaves the dead flag set.
14. **The `sidebar` layout runs a second tmux server** (`gcrt-tabs`) because a tab
    is a window and a window's status line spans the whole terminal — there is no
    way to draw a tab bar inside one region of a single window. Its session is
    created lazily by the first connection and dies with the last, so the content
    pane runs an attach *loop* rather than a bare `attach`. Nested tmux is safe
    here: the outer client eats the prefix before a pane sees it, so the inner
    server has no reachable prefix. Two servers must be torn down together.
15. **Bubbletea batches keystrokes.** Several keys arriving in one read (a paste,
    or fast typing) become **one** `KeyMsg` whose `String()` is `"jj"`, matching
    no single-key case — so both are dropped, and pasting into the filter did
    nothing. `Update` now detects a multi-rune `KeyMsg` and replays each rune.
    Any test that sends batched input hits this too.
16. **A composed line wider than the pane wraps and wrecks the frame.** The
    footer's key list alone is longer than 80 columns. `fitLine` drops the
    capability strip, then the scroll position, then clips. Same class of bug as
    the oversized modal: `Place` and `Width` never shrink what you hand them.
17. **`lipgloss.Width` is a minimum, not a maximum.** A 21-character key in a
    20-column field pushes the following text out of the box. Clip before
    padding.

## Environment — verified on this Mac, 2026-09-16

| | |
|---|---|
| Ghostty | 1.3.1 · Go 1.26.5 · tmux 3.7c · picocom 2024-07 |
| infisical | `~/.local/bin/infisical` |
| rbw / bw | MISSING — pending Q2 |
| git signing | SSH, `commit.gpgsign=true`, key `~/.ssh/id_signing.pub` |
| git identity | per-repo `hazyumps <patrick@fernlabs.net>` |

`gcrt` is installed at `~/go/bin/gcrt`, symlinked to `/opt/homebrew/bin/gcrt`.
No git remote yet — `git remote add origin
git@github.com:hazyumps/ghosttycrt.git` before the first push.
