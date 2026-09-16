# 07 — TUI

Bubbletea + lipgloss (D2). One screen, three regions, no modal wizards for the
common path.

## Layout

```
┌ ghosttycrt ─────────────────────────────────────────────────────────────┐
│ filter: /core█                                                           │
├──────────────────┬──────────────────────────────────────────────────────┤
│ ▼ network        │  console-core-sw-01                                  │
│   ▼ switches     │  ────────────────────────────────────────────────    │
│   ● core-sw-01   │  transport   serial                                   │
│     dist-sw-04   │  device      /dev/tty.usbserial-1420 @ 9600 8N1      │
│   ▼ console      │  group       network/console                          │
│   ○ console-…    │  credential  infisical:CORE-SW-01-PASS               │
│ ▼ servers        │  logging     on  → 1.2 MB today                       │
│   · esxi-02      │  last used   2026-09-16 13:04 (42 times)              │
│   ● k3s-01       │                                                       │
├──────────────────┴──────────────────────────────────────────────────────┤
│ enter:attach  n:new  e:edit  d:kill  l:log  L:logs  r:refresh  ?:help    │
│ Ctrl-b d returns to the tree                            tmux ✓  vault ✓   │
└──────────────────────────────────────────────────────────────────────────┘
```

- **Left:** the session tree. Groups collapse/expand, pinned entries float to
  the top of their group, `●/○/·/✗` status from `06-logging.md`.
- **Right:** details for the highlighted session — everything you would otherwise
  open an editor to check, including which credential reference it uses (the
  reference, never the value).
- **Bottom:** context keys and a capability strip showing whether `tmux`,
  `picocom`, and the configured vault are reachable right now, so a broken
  dependency is visible before you press enter.

Narrow terminals collapse to a single column: tree, with details shown inline
under the highlighted row.

## Views

| View | Entered by | Purpose |
|---|---|---|
| Tree | default | browse, connect, manage |
| Filter | `/` | fuzzy over name, host, group, tags |
| Form | `n` / `e` | create/edit a session |
| Log | `l` / `L` | suspend to `less -R +F` over the session log |
| Help | `?` | full keybind list |
| Import | `i` | propose `~/.ssh/config` and SecureCRT XML imports, show a diff |
| Doctor | `D` | dependency and config checks: tmux, ssh, picocom, providers, paths |

## Keybinds

| Key | Action |
|---|---|
| `↑ ↓` / `j k` | move |
| `← →` / `h l`* | collapse / expand group |
| `enter` | attach (creating the session if needed) |
| `o` | new Ghostty tab/window instead of in-place (respects `display_mode`) |
| `/` | filter |
| `esc` | clear filter / leave view |
| `n` | new session |
| `e` | edit session |
| `c` | duplicate session |
| `space` | pin / unpin |
| `t` | toggle logging for the session |
| `l` | open today's log |
| `L` | browse all logs for the session |
| `d` | kill / detach / forget (a menu, defaulting to detach) |
| `r` | refresh tmux state |
| `i` | imports |
| `D` | doctor |
| `?` | help |
| `q` | quit (confirm if sessions are running and `confirm_on_quit`) |

\* `h`/`l` conflict with the familiar vim motion; only `←`/`→` are bound to group
collapse until that is resolved. Do not bind both and hope.

Nothing destructive is bound to a single key. `d` opens a menu whose default is
the safe action; deleting a session record asks for confirmation and shows
exactly which file is being written.

## The form

A single form for both new and edit, keyed off `transport`:

- name, group, tags, description, pin
- transport selector, which reveals only that transport's fields
- credential section: provider dropdown + ref text field, with a **Test** key
  that performs a `Get` and reports success/failure **without displaying the
  secret**
- logging toggle
- `on_connect` macro editor, only shown for serial/telnet and defaulted off

Validation is live against `03-data-model.md`'s rules. Save writes the whole
file atomically (temp file + rename) so a crash mid-save cannot corrupt
`sessions.toml`.

## Empty and error states

Designed first, not last:

- **No sessions:** "No sessions yet. `n` to add one, or `i` to import from
  `~/.ssh/config`." — with the import offered, because that is what someone with
  an existing setup wants.
- **tmux missing:** full-screen, not a crash. Names the platform's install
  command.
- **Vault unreachable:** not fatal. Sessions with `provider = none` still work;
  the capability strip shows the vault as down and connecting to a
  credential-backed session explains exactly which command failed.
- **Session exists but its device is gone:** row shows `✗` with the missing
  device path on the details pane.

## Theming

- A theme name of `auto` reads Ghostty's own config
  (`~/Library/Application Support/com.mitchellh.ghostty/config.ghostty` on macOS,
  `~/.config/ghostty/config` on Linux) for `background`, `foreground`, and the
  ANSI palette, so `gcrt` looks like it belongs inside the terminal rather than
  fighting it.
- Named themes are a small set of built-ins, plus `[theme.custom]` in
  `config.toml`.
- No images, no gradients, no animation. The only motion is a spinner while a
  secret is being fetched.

## Responsiveness

Everything except a credential fetch and a tmux call is local. Targets: tree
redraw under one frame for a few hundred sessions; `enter` to a visible session
feels instant (it is two subprocess calls); nothing blocks the event loop —
subprocesses run through `tea.Cmd` so the UI stays live and can show "fetching
credential…" instead of freezing.
