package encfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

// tamperCostBudgetKiB caps Time × MemoryKiB for any params a tampered
// cheap-param header may hand to Argon2: XOR 0xFF on the memory bytes
// reaches at most 65344 KiB for one pass, about 64 MiB once.
const tamperCostBudgetKiB = 64 * 1024

// kdfSpy records the params of every key derivation and returns a fixed key
// instead of running Argon2, so a header that slips through fails fast.
type kdfSpy struct{ calls []Params }

func (k *kdfSpy) derive(_ app.MasterPassword, h header) []byte {
	k.calls = append(k.calls, h.params)
	return make([]byte, keySize)
}

// spiedVault saves an empty vault with the cheap params and returns a
// store whose KDF is observed by the returned spy.
func spiedVault(t *testing.T) (*Store, *kdfSpy, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v.db")
	store := New(path, Params{Time: 1, MemoryKiB: 64, Threads: 1})
	mp := app.NewMasterPassword([]byte("first password"))
	if err := store.Create(t.Context(), mp, domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	spy := &kdfSpy{}
	store.derive = spy.derive
	return store, spy, raw
}

func loadForged(t *testing.T, store *Store, forged []byte) error {
	t.Helper()
	if err := os.WriteFile(store.path, forged, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := store.Load(t.Context(), app.NewMasterPassword([]byte("first password")))
	return err
}

func TestSpyObservesTheLoadKDF(t *testing.T) {
	store, spy, original := spiedVault(t)

	_ = loadForged(t, store, original)

	want := []Params{{Time: 1, MemoryKiB: 64, Threads: 1}}
	if len(spy.calls) != 1 || spy.calls[0] != want[0] {
		t.Fatalf("KDF calls = %+v, want %+v", spy.calls, want)
	}
}

func TestTamperedBytesNeverReachArgon2WithCostlyParams(t *testing.T) {
	store, spy, original := spiedVault(t)

	for offset := range original {
		tampered := bytes.Clone(original)
		tampered[offset] ^= 0xFF
		spy.calls = nil

		err := loadForged(t, store, tampered)

		if !errors.Is(err, app.ErrUnlock) {
			t.Fatalf("offset %d: Load error = %v, want app.ErrUnlock", offset, err)
		}
		for _, p := range spy.calls {
			if !p.withinBounds() || uint64(p.Time)*uint64(p.MemoryKiB) > tamperCostBudgetKiB {
				t.Fatalf("offset %d: KDF ran with %+v, want in-bounds params costing at most %d KiB", offset, p, tamperCostBudgetKiB)
			}
		}
	}
}

func TestOutOfBoundsHeaderIsRejectedWithoutRunningKDF(t *testing.T) {
	setU32 := func(off int, v uint32) func([]byte) {
		return func(b []byte) { binary.BigEndian.PutUint32(b[off:], v) }
	}
	setByte := func(off int, v byte) func([]byte) {
		return func(b []byte) { b[off] = v }
	}
	tests := []struct {
		name   string
		mutate func([]byte)
	}{
		{name: "time 0", mutate: setU32(offTime, 0)},
		{name: "time 17", mutate: setU32(offTime, 17)},
		{name: "memory 7 KiB", mutate: setU32(offMemory, 7)},
		{name: "memory 1048577 KiB", mutate: setU32(offMemory, 1048577)},
		{name: "threads 0", mutate: setByte(offThreads, 0)},
		{name: "threads 17", mutate: setByte(offThreads, 17)},
		{name: "unknown version", mutate: func(b []byte) { binary.BigEndian.PutUint16(b[offVersion:], 2) }},
		{name: "unknown unlock id", mutate: setByte(offUnlock, 2)},
		{name: "unknown kdf id", mutate: setByte(offKDF, 2)},
		{name: "unknown aead id", mutate: setByte(offAEAD, 2)},
	}
	store, spy, original := spiedVault(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forged := bytes.Clone(original)
			// The largest in-bounds cost, so every case differs from an
			// accepted header only by the field under test.
			binary.BigEndian.PutUint32(forged[offTime:], maxTime)
			binary.BigEndian.PutUint32(forged[offMemory:], maxMemoryKiB)
			tt.mutate(forged)
			spy.calls = nil

			err := loadForged(t, store, forged)

			if !errors.Is(err, app.ErrUnlock) {
				t.Fatalf("Load error = %v, want app.ErrUnlock", err)
			}
			if len(spy.calls) != 0 {
				t.Fatalf("KDF ran with %+v, want the header rejected first", spy.calls)
			}
		})
	}
}
