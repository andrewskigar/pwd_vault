# save_pwd

save_pwd is a command-line password vault. It keeps all your credentials in one
encrypted file, unlocked by a single master password. Each entry has a name, a
kind (`site`, `card`, `note` or `other`), a username, a password, a URL, notes,
created and updated timestamps and an optional expiry date. Names keep the case
you type but are unique and looked up case-insensitively, so `GitHub` and
`github` are the same entry.

Domain vocabulary is defined in [CONTEXT.md](CONTEXT.md).

## Contents

- [Security model](#security-model)
- [Install](#install)
- [Vault location](#vault-location)
- [Quick start](#quick-start)
- [Command reference](#command-reference)
- [Scripting](#scripting)
- [Exit codes and errors](#exit-codes-and-errors)
- [Limitations](#limitations)
- [Development](#development)
- [License](#license)

## Security model

The full design is in
[ADR-0002: vault file format and cryptography](docs/adr/0002-vault-file-format-and-crypto.md).
In short:

- **Encryption**: the key is derived from the master password with Argon2id
  (64 MiB by default) and the entries are encrypted with XChaCha20-Poly1305.
- **Authenticated header**: the file starts with a 62-byte versioned header
  (KDF and AEAD ids, Argon2id parameters, salt, nonce). The whole header is the
  AEAD associated data, so tampering with it fails decryption.
- **Bounded header**: Argon2id parameters are checked before the KDF runs
  (time 1-16, memory at most 256 MiB, threads 1-16), and files over 64 MiB are
  rejected, so a forged file cannot make save_pwd allocate unbounded memory.
- **Fresh salt and nonce on every save.**
- **Atomic saves**: each save writes a temp file in the same directory, fsyncs
  it and renames it over the vault. A new vault is created exclusively
  (`O_EXCL`), so `init` never overwrites an existing file.
- **Owner-only permissions**, applied before any data is written:
  - Windows: a protected DACL (no inherited entries) granting full control to
    the current user and SYSTEM only.
  - Elsewhere: mode `0600` for the file and `0700` for the vault directory.
  - Only the vault's own directory, and only if save_pwd created it, gets
    owner-only rights; existing directories are left untouched.
- **Symlinks**: if the vault path is a symlink, saves follow it and replace the
  target, so the link survives (useful for vaults kept in a synced folder).
- **No secrets on the command line**: the master password and entry passwords
  never come from arguments or environment variables. They are read from a
  hidden terminal prompt or from stdin with `--password-stdin`.
- **Masked output**: `get` masks the password unless you pass `--show`; `list`
  never shows passwords.
- **Wiping**: secret byte buffers (master password, typed and generated entry
  passwords, derived key, decrypted plaintext) are zeroed as soon as they are no
  longer needed. See [Limitations](#limitations) for what Go cannot wipe.
- **Uniform unlock error**: a wrong master password and a corrupted or tampered
  file give the same message.

## Install

From a clone of this repository:

```sh
go install ./cmd/save_pwd
```

or build a binary in the current directory:

```sh
go build -o save_pwd ./cmd/save_pwd
```

`go.mod` declares `go 1.27` with `toolchain go1.27.4`; a recent Go downloads
that toolchain automatically. The version string defaults to `dev` and can be
set at build time:

```sh
go build -ldflags "-X main.version=1.0.0" -o save_pwd ./cmd/save_pwd
save_pwd --version
# save_pwd version 1.0.0
```

## Vault location

The vault path is chosen in this order:

1. `--vault PATH`
2. the `SAVE_PWD_VAULT` environment variable (an empty value counts as unset)
3. the default, `save_pwd/vault.db` inside the user config directory
   (`os.UserConfigDir()`):

| OS      | Default vault path                                                      |
|---------|-------------------------------------------------------------------------|
| Windows | `%AppData%\save_pwd\vault.db`                                           |
| macOS   | `~/Library/Application Support/save_pwd/vault.db`                       |
| Linux   | `$XDG_CONFIG_HOME/save_pwd/vault.db`, or `~/.config/save_pwd/vault.db`  |

`generate` does not use a vault, so it ignores all of these.

## Quick start

Interactively (needs a terminal; prompts go to stderr and input is hidden):

```console
$ save_pwd init
Master password:
Repeat master password:
Created vault at /home/alice/.config/save_pwd/vault.db
$ save_pwd add github --username alice --url https://github.com
Master password:
Entry password:
Repeat entry password:
Added "github"
$ save_pwd get github
Master password:
Name:     github
...
```

The master password must be at least 8 characters.

## Command reference

Every vault command accepts the global flags:

| Flag               | Meaning                                                              |
|--------------------|----------------------------------------------------------------------|
| `--vault string`   | path of the vault file                                               |
| `--password-stdin` | read the master password from the first line of stdin instead of prompting |
| `-h`, `--help`     | help for the command                                                 |
| `-v`, `--version`  | print the version (root command only): `save_pwd version dev`        |

The examples below use `--password-stdin` with the obviously fake master
password `correct-horse-demo` so that the output shown is exactly what the
binary prints. Interactively, drop `--password-stdin` and the `printf`.

### init

Create a new encrypted vault. No command-specific flags.

```console
$ printf '%s\n' correct-horse-demo | save_pwd init --password-stdin
Created vault at /tmp/demo/vault.db
$ printf '%s\n' correct-horse-demo | save_pwd init --password-stdin
save_pwd: vault already exists at /tmp/demo/vault.db
```

Interactively, the master password is asked for twice. A master password
shorter than 8 characters fails with
`save_pwd: master password must be at least 8 characters`.

### add

`save_pwd add <name> [flags]`: add an entry. Its password is prompted (twice)
or generated, never taken from flags.

| Flag                | Default | Meaning                                         |
|---------------------|---------|-------------------------------------------------|
| `--kind string`     | `site`  | entry kind: `site`, `card`, `note` or `other`   |
| `--username string` |         | user name or login                              |
| `--url string`      |         | address of the resource                         |
| `--notes string`    |         | free-form notes                                 |
| `--expires string`  |         | expiry date, `YYYY-MM-DD`                       |
| `--generate`        | off     | generate a random entry password instead of prompting |
| `--length int`      | `20`    | password length (requires `--generate`)         |
| `--no-symbols`      | off     | leave out symbol characters (requires `--generate`) |
| `--no-ambiguous`    | off     | leave out look-alike characters `O0o1lI` (requires `--generate`) |

```console
$ printf '%s\n%s\n' correct-horse-demo example-entry-pw | save_pwd add github --username alice --url https://github.com --expires 2027-01-31 --password-stdin
Added "github"
$ printf '%s\n' correct-horse-demo | save_pwd add bank --generate --length 24 --kind card --password-stdin
Added "bank"
(generated password; view it with: save_pwd get bank --show)
```

A generated password is never printed by `add`; use the hint to view it. Adding
a name that already exists (in any case) fails with
`save_pwd: entry "Bank" already exists`.

### get

`save_pwd get <name> [flags]`: print an entry, with the password masked.
Timestamps are RFC 3339 in UTC.

| Flag     | Meaning                           |
|----------|-----------------------------------|
| `--show` | print the password in clear text  |

```console
$ printf '%s\n' correct-horse-demo | save_pwd get github --password-stdin
Name:     github
Kind:     site
Username: alice
Password: ********
URL:      https://github.com
Notes:    
Created:  2026-11-20T20:34:28Z
Updated:  2026-11-20T20:34:28Z
Expires:  2027-01-31
```

With `--show`, the `Password:` line holds the real password
(here `example-entry-pw`).

### list

`save_pwd list [flags]`: list entries as a table sorted by name. Passwords are
never shown; dates are `YYYY-MM-DD`.

| Flag            | Meaning                                                   |
|-----------------|-----------------------------------------------------------|
| `--kind string` | only list entries of this kind: `site`, `card`, `note` or `other` |

```console
$ printf '%s\n' correct-horse-demo | save_pwd list --password-stdin
NAME    KIND  USERNAME  UPDATED     EXPIRES
bank    card            2026-11-20
github  site  alice     2026-11-20  2027-01-31
$ printf '%s\n' correct-horse-demo | save_pwd list --kind note --password-stdin
No entries.
```

`No entries.` goes to stderr (exit 0), so a pipe reading stdout sees nothing.

### update

`save_pwd update <name> [flags]`: change an entry. Only the flags you give are
changed; an explicitly empty value (for example `--notes ""`) clears that
field.

| Flag                | Default | Meaning                                         |
|---------------------|---------|-------------------------------------------------|
| `--kind string`     |         | new entry kind: `site`, `card`, `note` or `other` |
| `--username string` |         | new user name; empty clears it                  |
| `--url string`      |         | new address; empty clears it                    |
| `--notes string`    |         | new notes; empty clears them                    |
| `--expires string`  |         | new expiry date, `YYYY-MM-DD`; empty clears it  |
| `--change-password` | off     | prompt for a new entry password                 |
| `--generate`        | off     | generate a random entry password instead of prompting |
| `--length int`      | `20`    | password length (requires `--generate`)         |
| `--no-symbols`      | off     | leave out symbol characters (requires `--generate`) |
| `--no-ambiguous`    | off     | leave out look-alike characters `O0o1lI` (requires `--generate`) |

`--change-password` and `--generate` are mutually exclusive. save_pwd unlocks
the vault and finds the entry first, so a wrong master password, a missing vault
or an unknown name fails before you are asked for (or it generates) the new
password.

```console
$ printf '%s\n' correct-horse-demo | save_pwd update github --username bob --password-stdin
Updated "github"
$ printf '%s\n%s\n' correct-horse-demo new-entry-pw | save_pwd update github --change-password --password-stdin
Updated "github"
$ printf '%s\n' correct-horse-demo | save_pwd update github --generate --password-stdin
Updated "github"
(generated password; view it with: save_pwd get github --show)
$ printf '%s\n' correct-horse-demo | save_pwd update github --password-stdin
save_pwd: no changes requested; give a field flag, --change-password or --generate
Run 'save_pwd --help' for usage.
```

### delete

`save_pwd delete <name> [flags]`: delete an entry after a `y/N` confirmation.
The vault is unlocked and the entry found before the question is asked.

| Flag    | Meaning                                  |
|---------|------------------------------------------|
| `--yes` | delete without asking for confirmation   |

```console
$ printf '%s\ny\n' correct-horse-demo | save_pwd delete bank --password-stdin
Delete entry "bank"? [y/N] Deleted "bank"
$ printf '%s\n' correct-horse-demo | save_pwd delete github --yes --password-stdin
Deleted "github"
```

The question is written to stderr. Only `y` or `yes` (any case) confirms;
anything else, including no answer, fails with `save_pwd: deletion aborted`
(exit 1).

### generate

`save_pwd generate [flags]`: print a random password. Needs no vault and no
master password.

| Flag             | Default | Meaning                                   |
|------------------|---------|-------------------------------------------|
| `--length int`   | `20`    | password length, 8 to 256                 |
| `--no-symbols`   | off     | leave out symbol characters               |
| `--no-ambiguous` | off     | leave out look-alike characters `O0o1lI`  |

```console
$ save_pwd generate --length 16
<16 random characters>
$ save_pwd generate --length 6
save_pwd: length 6 must be between 8 and 256: invalid input
Run 'save_pwd --help' for usage.
```

Lengths 8 to 11 are accepted with a warning on stderr, for example
`save_pwd: warning: a 10-character password is weak; use at least 12`. The
same rules apply to `--length` on `add` and `update`.

## Scripting

With `--password-stdin`, save_pwd reads secrets from stdin, one per line, in
this order:

| Command                          | Line 1          | Line 2                        |
|----------------------------------|-----------------|-------------------------------|
| `init`                           | master password | (none; not confirmed)         |
| `add` (without `--generate`)     | master password | entry password                |
| `add --generate`                 | master password | (none)                        |
| `update --change-password`       | master password | new entry password            |
| `update` (other flags)           | master password | (none)                        |
| `delete` (without `--yes`)       | master password | confirmation answer, e.g. `y` |
| `get`, `list`, `delete --yes`    | master password | (none)                        |

- A trailing `\r` is stripped, so CRLF input works.
- An empty or missing secret line is an error:
  `save_pwd: no password provided on stdin`.
- Prompts and notices go to stderr, so stdout carries only results.
- Without `--password-stdin`, secrets are read from a hidden terminal prompt.
  If stdin is not a terminal the command fails with
  `save_pwd: no terminal available; use --password-stdin`. (For `delete`
  without `--password-stdin`, the confirmation answer is read from stdin.)

Example:

```sh
export SAVE_PWD_VAULT=/tmp/demo/vault.db
printf '%s\n' "$MASTER" | save_pwd init --password-stdin
printf '%s\n%s\n' "$MASTER" "$ENTRY_PW" | save_pwd add github --username alice --password-stdin
printf '%s\n' "$MASTER" | save_pwd list --password-stdin
```

Keep real secrets out of shell history: load `$MASTER` and `$ENTRY_PW` from a
protected file or a secret manager rather than typing them literally.

## Exit codes and errors

| Code | Meaning |
|------|---------|
| `0`  | success |
| `1`  | runtime failure: wrong master password, missing vault, entry not found or already exists, aborted delete, I/O error |
| `2`  | usage error: unknown command or flag, wrong number of arguments, invalid name, kind, date or length, conflicting or missing flags |

Errors are printed to stderr as `save_pwd: <message>`; usage errors add
`Run 'save_pwd --help' for usage.` Common messages:

```text
save_pwd: cannot unlock vault: wrong master password or corrupted file
save_pwd: no vault at /tmp/demo/vault.db; run "save_pwd init" first
save_pwd: entry "nope" not found
```

The unlock message is the same for a wrong password and a damaged or tampered
file, by design.

## Limitations

These are accepted trade-offs, documented in
[ADR-0002](docs/adr/0002-vault-file-format-and-crypto.md):

- Decrypted entry fields and entry passwords (`domain.Password`) are immutable
  Go strings. They cannot be wiped and may linger in the heap, and from there
  in swap or crash dumps, until the memory is reused.
- `encoding/json` intermediate copies, the `bufio` buffer holding
  `--password-stdin` lines, `math/big` values used during generation and library
  internals (Argon2 working memory, the AEAD key schedule) are not cleared.
- No `mlock` or swap protection.
- `get --show` prints the secret to stdout by design.
- No key-file unlock in v1 (the header reserves a field for it).
- On volumes without ACL support (FAT/exFAT) on Windows, the vault is written
  without an ACL.
- Hardlinks and Windows junctions are not handled specially. The symlink target
  is resolved once per save, so retargeting it mid-save requires write access to
  the vault directory, which is outside the threat model.
- The `view it with` hint printed by `add --generate` and `update --generate`
  is safe to paste in POSIX shells for any name; in PowerShell for any name
  except quoted names containing `'` or a typographic single quote (these are
  rejected by `get`, nothing runs); and in cmd.exe only for bare or
  double-quoted names.
- Vaults written with more than 256 MiB of Argon2 memory do not open (save_pwd
  never writes such vaults).

## Development

```sh
go test ./...
go vet ./...
golangci-lint run
govulncheck ./...
```

- Lint uses golangci-lint v2 with [.golangci.yml](.golangci.yml): errcheck,
  govet, ineffassign, staticcheck and unused, with gofmt as the formatter.
- If `govulncheck ./...` cannot load packages after a toolchain bump, use the
  binary-mode fallback from
  [ADR-0003: toolchain and security checks](docs/adr/0003-toolchain-and-security-checks.md):
  `go build -o save_pwd.exe ./cmd/save_pwd` then
  `govulncheck -mode=binary save_pwd.exe`. Do not commit the binary.
- Contributors need golangci-lint v2 and govulncheck built with go1.27.4 or
  later on `PATH`.

### Architecture

The code follows a hexagonal layout
([ADR-0001](docs/adr/0001-hexagonal-layout.md)):

| Package                     | Role |
|-----------------------------|------|
| `internal/domain`           | core: entries, names, kinds, dates, passwords |
| `internal/app`              | core: use cases and the `VaultStore` and `Clock` ports |
| `internal/passgen`          | core: password generation from an injected randomness source |
| `internal/adapters/cli`     | driving adapter: the cobra command tree and exit codes |
| `internal/adapters/encfile` | driven adapter: the encrypted vault file (implements `app.VaultStore`) |
| `cmd/save_pwd`              | composition root wiring real resources into the CLI |

`internal/architecture_test.go` enforces the import rules: it fails if a core
package imports an adapter, a third-party module, `os`, `net`, `math/rand` or
`unsafe`. Terms are defined in [CONTEXT.md](CONTEXT.md).

## License

There is no license yet.
