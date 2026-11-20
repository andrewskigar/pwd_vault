# ADR 0002: Vault file format and cryptography

Status: accepted

## Decision
- One file: a 62-byte versioned binary header (magic, format version, KDF id, Argon2id time/memory/threads, 16-byte salt, AEAD id, 24-byte nonce) followed by an XChaCha20-Poly1305 ciphertext of a JSON payload.
- Key derivation: Argon2id from the master password; parameters live in the header so they can be raised later. Decoding bounds them (time 1-16, memory at most 256 MiB (four times the 64 MiB default), threads 1-16) and files over 64 MiB are rejected. An out-of-bounds header fails with `ErrUnlock` before the KDF runs, so a forged header cannot make Load allocate more than 256 MiB.
- The whole header is the AEAD associated data. Any tampering or wrong password yields the same `ErrUnlock` message.
- Every save uses a fresh random salt and nonce, writes a temp file in the same directory (0600), fsyncs, and renames atomically. New vaults use O_EXCL; directories are 0700 (best effort on Windows).
- Symlinked vault paths: each save resolves the vault path once (`filepath.EvalSymlinks`), creates the temp file in the target's directory and renames it onto the target, so the link itself survives and the rename stays atomic within one directory. Why: users keep the vault in a synced or dotfiles folder and link to it; replacing the link with a regular file would silently stop syncing, and Load already follows links, so saves should too. If nothing exists at the path, the save writes the path itself.
  - Dangling link: Load reports `ErrVaultNotFound`; Create keeps its O_EXCL reservation, so it reports `ErrVaultExists` and never follows or replaces the link.
  - Limits: the path is resolved once per save, so retargeting the link mid-save is possible only for an attacker with write access to the vault directory, which is outside the threat model. Hardlinks and Windows junctions are not handled specially.
- Secrets never come from argv or env. `get` masks by default; `list` never prints passwords.
- Password generation uses `crypto/rand` with unbiased sampling and Fisher-Yates shuffling.

## Consequences
Key-file unlock is not in v1 (header has an unlock-method field for it). Decrypted strings cannot be wiped (Go immutability). Vaults written with more than 256 MiB of Argon2 memory no longer open; the CLI never wrote such params (default 64 MiB), and the format version stays 1.
