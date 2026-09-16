# 03 — Data model

Three files, all TOML, all hand-editable. Config and sessions are syncable;
state is not.

## `config.toml` — application settings

```toml
[general]
default_transport   = "ssh"
display_mode        = "inline"        # inline | ghostty-tab | ghostty-window
confirm_on_quit     = true
secret_scrub_logs   = true

[general.theme]
name                = "auto"          # auto = follow Ghostty, or a named theme

[transports.ssh]
binary              = "ssh"
extra_args          = ["-o", "ServerAliveInterval=30"]

[transports.serial]
backend             = "picocom"       # picocom | screen
default_baud        = 9600

[transports.telnet]
binary              = "telnet"

[logging]
enabled_by_default  = false
directory           = ""              # empty = XDG data dir
timestamp           = true
rotate_mb           = 50

[credentials]
default_provider    = "infisical"

[credentials.providers.infisical]
# argv template; {ref} is replaced with the secret reference from the session
command = ["infisical", "secrets", "get", "{ref}", "--plain", "--env=prod"]

[credentials.providers.vaultwarden]
# rbw is the recommended client (agent-based, Vaultwarden-compatible).
# Alternative: ["vaultwarden-cli", "get", "{ref}"]
command = ["rbw", "get", "{ref}"]
unlock_command = ["rbw", "unlock"]

[import.ssh_config]
enabled             = true
path                = "~/.ssh/config"

[display.ghostty]
cli_path            = "/Applications/Ghostty.app/Contents/MacOS/ghostty"
```

Notes:

- **Providers are argv arrays, not shell strings.** No word-splitting bugs, no
  quoting hazards, and `{ref}` is the only substitution.
- The Infisical flag set (`--env`, `--projectId`, `--path`) belongs here, not in
  code, so `gcrt` never guesses at another project's layout. Verify the exact
  spellings against `infisical secrets get --help` at M4.
- `display.ghostty.cli_path` is only consulted for `ghostty-window`; `ghostty`
  is not on `PATH` by default on macOS.

## `sessions.toml` — the session tree

One `[[session]]` table per host. `id` is a stable UUID generated at creation so
renames never orphan state or logs; `slug` is derived from the display name and
used for the tmux session name.

```toml
[[session]]
id            = "b3f1c2a4-…"        # stable, immutable
name          = "core-sw-01"
slug          = "core-sw-01"        # [a-z0-9-], unique; tmux name = gscrt/<slug>
transport     = "ssh"              # ssh | serial | telnet | local
group         = "network/switches"  # "/"-delimited, renders as a tree
tags          = ["cisco", "prod"]
pinned        = true
description   = "Cisco C9300, rack A"

  [session.ssh]
  host        = "10.1.3.11"
  port        = 22
  user        = "admin"
  jump        = "bastion"           # optional ProxyJump target
  identity    = "~/.ssh/id_ed25519" # optional; empty = agent/default

  [session.logging]
  enabled     = true                # overrides logging.enabled_by_default

  [session.credentials]
  provider    = "infisical"         # none | infisical | vaultwarden
  ref         = "CORE-SW-01-PASS"   # a pointer, never a value

[[session]]
id            = "c9e7…"
name          = "console-core-sw-01"
slug          = "console-core-sw-01"
transport     = "serial"
group         = "network/console"

  [session.serial]
  device      = "/dev/tty.usbserial-1420"
  baud        = 9600
  databits    = 8
  parity      = "none"
  stopbits    = 1
  flow        = "none"

  [session.logging]
  enabled     = true

  [session.on_connect]              # optional; see 05-credentials.md (Q3)
  steps       = [
    { expect = "login:",  send = "admin" },
    { expect = "Password:", send = "{secret}", redact = true },
    { expect = ">",       send = "terminal length 0" },
  ]
```

Field rules:

- `transport` selects which `[session.<transport>]` block is valid. A session
  with mismatched fields is a config error, reported at load, not at connect.
- `slug` must be unique and match `^[a-z0-9][a-z0-9-]*$`. Duplicates are a load
  error with both offending names shown.
- `group` renders as a `/`-separated tree. Empty group = root.
- Credentials are **always** `provider` + `ref`. There is no field anywhere in
  this file that holds a secret. A literal password in `sessions.toml` is a bug.
- `device` for serial is the full `/dev` path; a `device_hint` (substring match,
  resolved at connect) may be added if the adaptor's path churns — decide at M2.

## `state.toml` — runtime state, not synced

```toml
[recent]
order = ["b3f1c2a4-…", "c9e7…"]      # most recent first, capped at 50

[sessions."b3f1c2a4-…"]
last_used   = "2026-09-16T13:20:00-04:00"
use_count   = 42
```

Machine-local by definition; excluded from the sync set and safe to delete.
Losing it costs recents and counts, nothing else.

## Imports

Two importers, both non-destructive (they propose, you confirm, then they write):

**`~/.ssh/config`** — parse `Host` stanzas into `transport = "ssh"` sessions
with `host`, `port`, `user`, `identity`, `jump` (from `ProxyJump`). Wildcard
hosts (`Host *`, `Host 10.1.3.*`) are skipped and reported. Groups come from
nested `Include` paths or a configured mapping; default is one group named
`ssh-config`.

**SecureCRT** — SecureCRT keeps one `.ini` per session under
`~/Library/Application Support/VanDyke/SecureCRT/Config/Sessions`, mirroring the
folder tree as directories, so the folder path *is* the group. This is the real
on-disk store and is what `gcrt import securecrt` reads; the XML export format is
not needed. Lines are `S:"Key"=text`, `D:"Key"=hex` or `B:"Key"=hex`, with hex
continuation lines for binary blobs — the parser must skip continuations or it
will lose the key that follows.

Mapped fields, confirmed against a real 121-session config (2026-09-16):

| SecureCRT | gcrt |
|---|---|
| `S:"Hostname"` | `ssh.host` (or `serial`/`telnet` equivalent) |
| `S:"Username"` | `ssh.user` |
| `D:"[SSH2] Port"` | `ssh.port` — a **hex** dword, `00000016` is 22 |
| `S:"Protocol Name"` | `transport` (`SSH2`, `Serial`, `Telnet`, `Local Shell`) |
| folder path | `group` |
| file name minus `.ini` | `name` |
| `S:"Firewall Name"` | `ssh.jump`, when it is not `None` |

`__FolderData__.ini` and `Default.ini` are folder metadata and the template, not
sessions, and are skipped.

Password fields are **never** imported — SecureCRT stores them encrypted, but
the value would not survive the trip to SSH anyway. A session whose
`D:"Session Password Saved"` is 1 is reported so a credential reference can be
added by hand. `Use Login Script` sessions are reported too: their login macro
is not imported, and `on_connect` is deferred past MVP (Q3).

Session ids are a UUIDv5 hash of the session's path relative to the Sessions
directory, so re-importing the same config yields the same ids and never orphans
state or logs.

Both importers write to a staging file and print a diff before committing.

## Validation

`gcrt check` (and load-time validation) enforces:

1. Unique `slug` and unique `id`.
2. `transport` matches the present `[session.<transport>]` block.
3. Serial `baud` in {300,1200,2400,4800,9600,19200,38400,57600,115200,230400}.
4. Credential `provider` is one of the configured providers.
5. Serial `device` exists at connect time (warning if missing at load time,
   since USB adaptors come and go).
6. No field in any `[session.credentials]` block looks like a literal secret
   (heuristic: long high-entropy strings with no `provider` set → error).
