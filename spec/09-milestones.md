# 09 — Milestones

Build the smallest usable slice, exercise it, then add. Each milestone ends in
something you would actually use.

## M0 — skeleton (a day)

- [x] Go module, `cmd/gcrt`, `internal/{config,session,tui,tmux}`
- [x] `config.toml` and `sessions.toml` load + validate (`03-data-model.md`)
- [x] XDG path resolution for config/state/data on macOS and Linux
- [x] Tree view: groups, collapse, move, fuzzy filter
- [x] ASCII tree render with placeholder status glyphs

Done when: `gcrt` shows a hand-written `sessions.toml` as a browsable tree.
**Landed 2026-09-16.**

## M1 — connect (a weekend) ← *first useful version*

- [x] `tmux` detection + friendly install message
- [x] `transport` interface, ssh implementation (`04-transports.md`)
- [x] create-or-attach, `tea.ExecProcess`, detach returns to tree
- [x] status glyphs read real tmux state (`tmux list-sessions`)
- [x] `enter` / `d` (detach) / `q` with confirmation
- [x] capability strip: tmux, ssh

Done when: `enter` on an SSH host drops you in, `Ctrl-b d` brings you back, and
the session is still there after quitting Ghostty. **This is already a
SecureCRT replacement for SSH.**

**Landed 2026-09-16.** `d`'s "kill and remove from the tree" option is deferred
to M2, which is where atomic `sessions.toml` writes live.

## M2 — serial + session CRUD (a few days)

- [ ] picocom backend, screen fallback, framing options
- [ ] missing-device check with `/dev/tty.*` candidates
- [ ] `remain-on-exit` for serial panes
- [ ] form view: new/edit/duplicate/delete/pin
- [ ] atomic `sessions.toml` writes
- [ ] `~/.ssh/config` importer with preview diff
- [ ] legacy `TERM=xterm-256color` toggle for old gear

Done when: a USB console to a switch is two keystrokes, and `~/.ssh/config`
imports without retyping.

## M3 — logging (a few days)

- [ ] `pipe-pane` on create, re-applied on every attach
- [ ] dated files, `0600`, header line, optional `ts`
- [ ] per-session toggle on a live session
- [ ] `less -R +F` viewer (`l`) and log browser (`L`)
- [ ] rotation by size; `gcrt logs prune --older-than`
- [ ] live log-size marker in the tree

Done when: console sessions capture themselves and you never think about it.

## M4 — credentials (a few days)

- [ ] provider interface + argv templates from config
- [ ] Infisical provider (confirm flags against `--help` first) 
- [ ] Vaultwarden provider (`rbw`, unlock-then-retry once)
- [ ] `SSH_ASKPASS` self-helper, `SSH_ASKPASS_REQUIRE=force`
- [ ] `Test` button in the form (proves `Get` without revealing the value)
- [ ] `on_connect` macro (opt-in, default off), `redact` handling
      — **deferred past MVP** (Q3); the field stays in the data model
- [ ] log scrubbing for sessions carrying a secret

Done when: password-auth gear connects with no typed password and nothing secret
touches the disk.

## M5 — display modes + polish (a few days)

- [x] `workspace` mode — the tree in its own tmux pane with sessions tiled
      beside it, several live at once. **Landed 2026-09-16**, ahead of the rest
      of M5, at Patrick's request. See D6 in `README.md`.
- [ ] `ghostty-tab` via AppleScript (macOS), with clean fallback on refusal
- [ ] `ghostty-window`, exact flags verified per platform
- [ ] `theme = "auto"` reading Ghostty's config
- [ ] `?` help, `D` doctor view
- [ ] empty/error states from `07-tui.md`

Done when: the tree can stay open in its own tab while sessions live in
tab-native neighbours on the Mac, and Linux is unaffected.

## M6 — migration + hardening (a few days)

- [x] SecureCRT importer (passwords discarded, refs flagged) — **landed
      2026-09-16** as `gcrt import securecrt`, reading the native per-session
      `.ini` store rather than an XML export. 120 sessions imported.
- [x] `gcrt check` command — landed with M0
- [ ] golden/teatest coverage for the UI; integration tests on a private tmux
      socket (`tmux -L gcrt-test`)
- [ ] README quickstart per platform; the `brew`/`apt` lines
- [ ] Linux acceptance run (`08-acceptance.md` AC-29..AC-32`)
- [ ] decide Q1–Q5 from `spec/README.md` and fold the answers back into the spec

Done when: SecureCRT gets deleted.

## Sequencing notes

- **M1 is the value line.** Everything before it is plumbing; everything after it
  is an upgrade to something already useful. Ship M1, live in it a week, then
  decide whether M2 or M3 matters more.
- M2 and M3 are independent; M4 depends on nothing but should follow M1 so there
  is a working session lifecycle to attach credentials to.
- M5's `ghostty-window` is the only piece with real platform uncertainty, and it
  is the only piece that may be dropped without touching the contract.

## Out of the first build

Recorded so they stop coming up: SFTP panel (`NG1`), Zmodem (`NG2`), Wyse/SCO
(`NG3`), `abduco`/`dtach` backend, Windows, a GUI, file-wallpaper terminals,
plugins.
