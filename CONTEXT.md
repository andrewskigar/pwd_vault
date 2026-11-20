# save_pwd domain context

save_pwd is a command-line password vault. Terms used in code and docs:

- **Vault**: the single encrypted file holding all entries. Located by `--vault`, then `SAVE_PWD_VAULT`, then `os.UserConfigDir()/save_pwd/vault.db`.
- **Master password**: the one secret that unlocks the vault. Read from a hidden prompt or the first stdin line with `--password-stdin`; never from argv or env.
- **Entry**: a named credential (name, kind, username, password, URL, notes, created/updated timestamps, optional expiry date).
- **Name**: entry identity. Case-preserving for display, case-insensitive for uniqueness and lookup (`Name.Key()`).
- **Kind**: one of `site`, `card`, `note`, `other`.
- **Core**: `internal/domain`, `internal/app`, `internal/passgen`. Imports only the standard library (no `os`, `net`, `math/rand`, `unsafe`) and other core packages.
- **Adapters**: `internal/adapters/cli` (cobra, driving) and `internal/adapters/encfile` (encrypted file, driven, implements `app.VaultStore`). `cmd/save_pwd/main.go` is the composition root.

Architecture, format and toolchain decisions: see `docs/adr/` (security checks: `docs/adr/0003-toolchain-and-security-checks.md`).
