// Package encfile stores the vault in one file encrypted with a key derived
// from the master password (Argon2id + XChaCha20-Poly1305).
package encfile

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

// Params is the Argon2id cost used when writing a vault.
type Params struct {
	Time      uint32
	MemoryKiB uint32
	Threads   uint8
}

// DefaultParams is the production Argon2id cost.
func DefaultParams() Params { return Params{Time: 3, MemoryKiB: 64 * 1024, Threads: 4} }

// Store is the encrypted vault file at one path. It satisfies app.VaultStore.
type Store struct {
	path   string
	params Params
	// rename replaces the vault with the finished temp file; tests swap it
	// to inject a failure at the last step of a save.
	rename func(oldpath, newpath string) error
	// derive runs Argon2id; tests swap it to observe which header params
	// reach the KDF, which a wall-clock budget could only infer.
	derive func(mp app.MasterPassword, h header) []byte
}

// New returns a store for path that encrypts with p.
func New(path string, p Params) *Store {
	return &Store{path: path, params: p, rename: os.Rename, derive: deriveKey}
}

// Create reserves the path with O_EXCL, so an existing vault is never
// overwritten even by a concurrent init, then writes the vault atomically.
func (s *Store) Create(ctx context.Context, mp app.MasterPassword, v domain.Vault) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create vault directory: %w", err)
	}
	reserved, err := os.OpenFile(s.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return app.ErrVaultExists
	}
	if err != nil {
		return fmt.Errorf("create vault file: %w", err)
	}
	if err := reserved.Close(); err != nil {
		return s.abandon(fmt.Errorf("create vault file: %w", err))
	}
	data, err := s.seal(mp, toPayload(v))
	if err != nil {
		return s.abandon(err)
	}
	if err := s.writeAtomic(ctx, data); err != nil {
		return s.abandon(err)
	}
	return nil
}

// Save re-encrypts v with a fresh salt and nonce and atomically replaces the
// vault file.
func (s *Store) Save(ctx context.Context, mp app.MasterPassword, v domain.Vault) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := s.seal(mp, toPayload(v))
	if err != nil {
		return err
	}
	return s.writeAtomic(ctx, data)
}

// abandon removes the file Create reserved and returns cause.
func (s *Store) abandon(cause error) error {
	if err := os.Remove(s.path); err != nil {
		return errors.Join(cause, fmt.Errorf("remove incomplete vault: %w", err))
	}
	return cause
}

// Load decrypts the vault. Every problem with the file's content yields the
// same app.ErrUnlock, so a wrong password and tampering look alike.
func (s *Store) Load(ctx context.Context, mp app.MasterPassword) (domain.Vault, error) {
	if err := ctx.Err(); err != nil {
		return domain.Vault{}, err
	}
	raw, err := readLimited(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return domain.Vault{}, app.ErrVaultNotFound
	}
	if err != nil {
		return domain.Vault{}, err
	}
	plain, ok := s.open(mp, raw)
	if !ok {
		return domain.Vault{}, app.ErrUnlock
	}
	defer clear(plain)
	var p payload
	if err := json.Unmarshal(plain, &p); err != nil {
		return domain.Vault{}, app.ErrUnlock
	}
	v, ok := p.toVault()
	if !ok {
		return domain.Vault{}, app.ErrUnlock
	}
	return v, nil
}

// readLimited checks the size before reading, so an oversize file is
// rejected without being loaded into memory.
func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat vault: %w", err)
	}
	if info.Size() > maxFileSize {
		return nil, app.ErrUnlock
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read vault: %w", err)
	}
	return raw, nil
}

func (s *Store) open(mp app.MasterPassword, raw []byte) ([]byte, bool) {
	if len(raw) < minFileSize || len(raw) > maxFileSize {
		return nil, false
	}
	h, ok := decodeHeader(raw)
	if !ok {
		return nil, false
	}
	key := s.derive(mp, h)
	defer clear(key)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, false
	}
	plain, err := aead.Open(nil, h.nonce[:], raw[headerSize:], raw[:headerSize])
	if err != nil {
		return nil, false
	}
	return plain, true
}

// seal always encrypts with the store's params and a fresh salt and nonce.
func (s *Store) seal(mp app.MasterPassword, p payload) ([]byte, error) {
	h := header{params: s.params}
	if _, err := rand.Read(h.salt[:]); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	if _, err := rand.Read(h.nonce[:]); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	plain, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("encode vault: %w", err)
	}
	defer clear(plain)
	key := s.derive(mp, h)
	defer clear(key)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("init cipher: %w", err)
	}
	hdr := h.encode()
	return append(hdr, aead.Seal(nil, h.nonce[:], plain, hdr)...), nil
}

func deriveKey(mp app.MasterPassword, h header) []byte {
	return argon2.IDKey(mp.Bytes(), h.salt[:], h.params.Time, h.params.MemoryKiB, h.params.Threads, keySize)
}

// writeAtomic writes data to a temp file in the vault's directory and renames
// it over the vault, so a failure never leaves a half-written vault.
func (s *Store) writeAtomic(ctx context.Context, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".vault-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write vault: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync vault: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close vault: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("replace vault: %w", err)
	}
	return nil
}
