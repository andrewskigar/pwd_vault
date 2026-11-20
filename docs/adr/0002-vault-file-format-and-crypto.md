# ADR 0002: Vault file format and cryptography

Status: accepted

## Decision
- One file: a 62-byte versioned binary header (magic, format version, KDF id, Argon2id time/memory/threads, 16-byte salt, AEAD id, 24-byte nonce) followed by an XChaCha20-Poly1305 ciphertext of a JSON payload.
- Key derivation: Argon2id from the master password; parameters live in the header so they can be raised later. Decoding bounds them (time 1-16, memory at most 1 GiB, threads 1-16) and files over 64 MiB are rejected.
- The whole header is the AEAD associated data. Any tampering or wrong password yields the same `ErrUnlock` message.
- Every save uses a fresh random salt and nonce, writes a temp file in the same directory (0600), fsyncs, and renames atomically. New vaults use O_EXCL; directories are 0700 (best effort on Windows).
- Secrets never come from argv or env. `get` masks by default; `list` never prints passwords.
- Password generation uses `crypto/rand` with unbiased sampling and Fisher-Yates shuffling.

## Consequences
Key-file unlock is not in v1 (header has an unlock-method field for it). Decrypted strings cannot be wiped (Go immutability).
