package encfile_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"save_pwd/internal/adapters/encfile"
	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

// Header offsets from the spec's file format v1 layout.
const (
	offTime    = 12
	offMemory  = 16
	offThreads = 20
)

// savedVault writes a vault holding one entry with the cheap params and
// returns its path and raw bytes.
func savedVault(t *testing.T) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v.db")
	store := newStore(path)
	if err := store.Create(t.Context(), master("first password"), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	v := domain.NewVault()
	if err := v.Add(mustEntry(t, "github", domain.KindSite, "2027-01-31")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.Save(t.Context(), master("first password"), v); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return path, raw
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func wrongPasswordMessage(t *testing.T, path string) string {
	t.Helper()
	_, err := newStore(path).Load(t.Context(), master("other password"))
	if !errors.Is(err, app.ErrUnlock) {
		t.Fatalf("wrong password Load error = %v, want app.ErrUnlock", err)
	}
	return err.Error()
}

func TestTamperingAnyByteIsIndistinguishableFromWrongPassword(t *testing.T) {
	path, original := savedVault(t)
	want := wrongPasswordMessage(t, path)
	store := newStore(path)

	// XOR 0xFF pushes every header param byte out of bounds or to at most
	// 64 MiB of Argon2 memory once (kdf_test.go proves the KDF side);
	// single-bit flips could cost up to 512 MiB.
	for offset := range original {
		tampered := bytes.Clone(original)
		tampered[offset] ^= 0xFF
		writeFile(t, path, tampered)

		_, err := store.Load(t.Context(), master("first password"))

		if !errors.Is(err, app.ErrUnlock) {
			t.Fatalf("offset %d: Load error = %v, want app.ErrUnlock", offset, err)
		}
		if err.Error() != want {
			t.Fatalf("offset %d: message %q differs from wrong-password message %q", offset, err, want)
		}
	}
}

func TestTruncatedVaultIsUnlockError(t *testing.T) {
	path, original := savedVault(t)
	for _, size := range []int{0, 61, 62, len(original) - 1} {
		writeFile(t, path, original[:size])

		_, err := newStore(path).Load(t.Context(), master("first password"))

		if !errors.Is(err, app.ErrUnlock) {
			t.Fatalf("size %d: Load error = %v, want app.ErrUnlock", size, err)
		}
	}
}

func TestOversizeVaultIsRejectedBeforeReading(t *testing.T) {
	path, original := savedVault(t)
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := f.Truncate(64<<20 + 1); err != nil {
		t.Fatalf("grow to 64 MiB + 1: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if len(original) >= 64<<20 {
		t.Fatal("seed vault unexpectedly large")
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = newStore(path).Load(t.Context(), master("first password"))
	runtime.ReadMemStats(&after)

	if !errors.Is(err, app.ErrUnlock) {
		t.Fatalf("Load error = %v, want app.ErrUnlock", err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8<<20 {
		t.Fatalf("Load allocated %d bytes, want the size checked before reading", allocated)
	}
}

func TestSaveReencryptsWithStoreParams(t *testing.T) {
	path, _ := savedVault(t)
	stronger := encfile.Params{Time: 2, MemoryKiB: 128, Threads: 1}
	store := encfile.New(path, stronger)

	v, err := store.Load(t.Context(), master("first password"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := store.Save(t.Context(), master("first password"), v); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := encfile.Params{
		Time:      binary.BigEndian.Uint32(raw[offTime:]),
		MemoryKiB: binary.BigEndian.Uint32(raw[offMemory:]),
		Threads:   raw[offThreads],
	}
	if got != stronger {
		t.Fatalf("header params = %+v, want %+v", got, stronger)
	}
}

func TestSaveWithCancelledContextLeavesVaultUnchanged(t *testing.T) {
	path, original := savedVault(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := newStore(path).Save(ctx, master("first password"), domain.NewVault())

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v, want context.Canceled", err)
	}
	assertFileEquals(t, path, original)
	assertOnlyVaultInDir(t, path)
}

func TestSuccessfulWritesLeaveOnlyTheVaultFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.db")
	store := newStore(path)

	if err := store.Create(t.Context(), master("first password"), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	assertOnlyVaultInDir(t, path)

	if err := store.Save(t.Context(), master("first password"), domain.NewVault()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	assertOnlyVaultInDir(t, path)
}

func assertFileEquals(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("the vault file changed")
	}
}

func assertOnlyVaultInDir(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory holds %v, want only %s", names, filepath.Base(path))
	}
}
