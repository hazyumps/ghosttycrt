# 05 — Credentials

Two requirements shape everything here:

1. **Never store a secret.** The config holds references. The vault holds values.
2. **Never put a secret in argv.** `ps` is world-readable.

So: fetch at connect time, hold in memory, hand over through a pipe or the
environment, and let it die with the process.

## The session record holds a pointer

```toml
[session.credentials]
provider = "infisical"          # none | infisical | vaultwarden
ref      = "CORE-SW-01-PASS"    # a key name or a vault item name
```

`ref` is opaque to `gcrt`. What it means is the provider's business: an Infisical
secret key, or a Vaultwarden/rbw item name. One session may have a different
provider than another; `credentials.default_provider` is only the default for
new sessions.

## Provider interface

```
type Provider interface {
    Name() string
    Get(ctx context.Context, ref string) (secret string, err error)
}
```

Providers are **argv templates from config**, never hardcoded commands:

```toml
[credentials.providers.infisical]
command = ["infisical", "secrets", "get", "{ref}", "--plain", "--env=prod"]

[credentials.providers.vaultwarden]
command = ["rbw", "get", "{ref}"]
unlock_command = ["rbw", "unlock"]
```

`{ref}` is substituted as exactly one argv element. No shell is involved, so a
`ref` containing spaces or quotes cannot break out. Secret = the child's stdout,
trailing newline trimmed. Non-zero exit = failure, and the provider's stderr is
shown verbatim because it is almost always the actionable message ("context
cancelled", "item not found", "vault is locked").

### Infisical

```toml
command = ["infisical", "secrets", "get", "{ref}", "--plain", "--env=prod"]
```

`--plain` is documented and supported: it prints the bare value with no table
decoration, which is exactly what a subprocess consumer wants. Authentication is
the CLI's existing session — `gcrt` does **not** handle Infisical login, tokens,
or machine identities. If `infisical` is not authenticated, that is reported and
the user fixes it in the CLI. This keeps `gcrt` free of credential logic beyond
`Get`.

Add `--projectId` / `--path` to the template for a project other than the
directory-local default. The spellings must be confirmed against
`infisical secrets get --help` at M4 before shipping; they live in config
precisely so getting one wrong is a one-line fix, not a release.

### Vaultwarden

Two clients, both just a command template:

- **`rbw`** (recommended): Rust, agent-based, `rbw unlock` once and subsequent
  `rbw get <name>` calls are instant against the running agent. Works with
  Vaultwarden, which is what is deployed here.
- **[`haydonryan/vaultwarden-cli`](https://github.com/haydonryan/vaultwarden-cli)**:
  simpler and stateless; every call may re-authenticate. Fine, slower.

Either way, `gcrt` shells out and reads stdout. If the vault is locked, `gcrt`
runs `unlock_command` interactively in the terminal (suspending the TUI the same
way an attach does), then retries `Get` once.

## How the secret reaches the session

### SSH (password auth)

OpenSSH already has a supported hook for this: `SSH_ASKPASS`. `gcrt` re-executes
itself as the helper and hands ssh nothing but an environment variable naming
which session to answer for.

```
env:
  SSH_ASKPASS            = <path to gcrt>
  SSH_ASKPASS_REQUIRE    = force
  GCRT_ASKPASS_SESSION   = <session id>

argv:
  ssh … user@host
```

`ssh` invokes `<gcrt> <prompt>`; `gcrt askpass` looks up `GCRT_ASKPASS_SESSION`,
checks that the prompt looks like a password or passphrase prompt, and prints the
secret to stdout. `SSH_ASKPASS_REQUIRE=force` removes the old `DISPLAY`
requirement; it needs OpenSSH 8.4+, which macOS 13+ and any current Linux ship.

Why this and not `sshpass`: `sshpass` puts the secret in a pty it then feeds to
ssh, and is itself a third-party dependency; `SSH_ASKPASS` is OpenSSH's own,
documented mechanism. Why not keys: keys are the right answer where they exist,
and `identity` in the session record covers that — keys are tried first, so
password auth is only reached for gear that cannot do keys.

Guard: the askpass helper verifies the requesting prompt string. If it is not a
password/passphrase prompt (e.g. a host-key confirmation), it exits non-zero
rather than feeding the secret to an unknown prompt.

### Serial and telnet (on-connect macro)

A console has no auth channel, so the only automation is typing. This is **Q3 —
opt-in, default off**, and it is honestly a bit ugly:

```toml
[session.on_connect]
steps = [
  { expect = "login:",    send = "admin" },
  { expect = "Password:", send = "{secret}", redact = true },
  { expect = ">",         send = "terminal length 0" },
]
```

Implementation: send `Enter`, then `tmux capture-pane` in a loop until `expect`
appears (with a timeout and a visible "waiting for login:" status), then
`tmux send-keys` the `send` value. `redact = true` means the value is never
echoed in the UI, never written to the log (see `06-logging.md`), and the tmux
buffer holding it is deleted immediately after paste.

**Known cost:** the secret transits the tmux server, so it exists briefly in a
tmux buffer and, if `send-keys` is used with a literal, in the tmux command
history. Mitigations: prefer `set-buffer` + `paste-buffer` + `delete-buffer`
over `send-keys`; clear the buffer unconditionally in a `defer`; never log the
step. Even so, treat this as appropriate for lab gear only, not for bastion-class
hosts, and say so in `gcrt`'s confirmation the first time a macro is enabled.

If the macro is off, serial and telnet sessions simply attach and you type.

## Rules that must hold

1. No secret is written to any file `gcrt` owns — not config, not state, not
   logs, not a temp file.
2. No secret appears in any argv. Passed via environment or a pipe only.
3. No secret is rendered to the screen, except in the deliberate case of
   `redact = false` macros where the user has opted in.
4. Secret variables are zeroed after use where the language permits, and
   anything containing one is `SecureString`/`[]byte` rather than `string` in
   Go so it does not linger in immutable string memory.
5. Provider stderr may be shown; provider stdout (the secret) never is.
6. `gcrt check` fails a session whose credential block contains something that
   looks like a literal secret.
7. Adding a new provider must not require touching anything but `config.toml`.

## What this deliberately is not

Not a vault, not a cache (no on-disk secret cache, ever), not a session manager
for the vaults, and not responsible for vault authentication. It is a
read-one-secret-on-demand shim, and the vaults stay the source of truth.
