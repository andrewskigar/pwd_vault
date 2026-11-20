package encfile_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"save_pwd/internal/adapters/encfile"
	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

func cheapParams() encfile.Params { return encfile.Params{Time: 1, MemoryKiB: 64, Threads: 1} }

func master(s string) app.MasterPassword { return app.NewMasterPassword([]byte(s)) }

// newStore returns the adapter through the port, as the application uses it.
func newStore(path string) app.VaultStore { return encfile.New(path, cheapParams()) }

func TestDefaultParams(t *testing.T) {
	want := encfile.Params{Time: 3, MemoryKiB: 65536, Threads: 4}
	if got := encfile.DefaultParams(); got != want {
		t.Fatalf("DefaultParams() = %+v, want %+v", got, want)
	}
}

func TestCreateThenLoadReturnsEmptyVault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "v.db")
	store := newStore(path)

	if err := store.Create(t.Context(), master("first password"), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	v, err := store.Load(t.Context(), master("first password"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", v.Len())
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".vault-*.tmp")); len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

func TestLoadWithWrongPasswordIsUnlockError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.db")
	store := newStore(path)
	if err := store.Create(t.Context(), master("first password"), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err := store.Load(t.Context(), master("other password"))

	if !errors.Is(err, app.ErrUnlock) {
		t.Fatalf("Load error = %v, want app.ErrUnlock", err)
	}
	if strings.Contains(err.Error(), "other password") {
		t.Fatalf("error %q leaks the password", err)
	}
}

func TestLoadRejectsMalformedFiles(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{name: "empty", content: nil},
		{name: "short", content: []byte("SAVE_PWD")},
		{name: "bad magic", content: bytes.Repeat([]byte{0}, 200)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "v.db")
			if err := os.WriteFile(path, tt.content, 0o600); err != nil {
				t.Fatalf("seed: %v", err)
			}

			_, err := newStore(path).Load(t.Context(), master("first password"))

			if !errors.Is(err, app.ErrUnlock) {
				t.Fatalf("Load error = %v, want app.ErrUnlock", err)
			}
		})
	}
}

func TestLoadMissingFileIsVaultNotFound(t *testing.T) {
	_, err := newStore(filepath.Join(t.TempDir(), "missing.db")).Load(t.Context(), master("first password"))

	if !errors.Is(err, app.ErrVaultNotFound) {
		t.Fatalf("Load error = %v, want app.ErrVaultNotFound", err)
	}
}

func TestCreateRefusesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v.db")
	original := []byte("existing content")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := newStore(path).Create(t.Context(), master("first password"), domain.NewVault())

	if !errors.Is(err, app.ErrVaultExists) {
		t.Fatalf("Create error = %v, want app.ErrVaultExists", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("file changed to %q", got)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".vault-*.tmp")); len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

func TestCreateUsesFreshSaltAndNonce(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.db"), filepath.Join(dir, "b.db")
	for _, p := range []string{a, b} {
		if err := newStore(p).Create(t.Context(), master("first password"), domain.NewVault()); err != nil {
			t.Fatalf("Create %s: %v", p, err)
		}
	}
	rawA, errA := os.ReadFile(a)
	rawB, errB := os.ReadFile(b)
	if errA != nil || errB != nil {
		t.Fatalf("read: %v, %v", errA, errB)
	}
	// Salt is bytes 21..37 and nonce 38..62 in the spec's header layout.
	if bytes.Equal(rawA[21:37], rawB[21:37]) {
		t.Error("two vaults share a salt")
	}
	if bytes.Equal(rawA[38:62], rawB[38:62]) {
		t.Error("two vaults share a nonce")
	}
}

func TestCreateHonoursCancelledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.db")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := newStore(path).Create(ctx, master("first password"), domain.NewVault())

	if err == nil {
		t.Fatal("Create with a cancelled context succeeded")
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("stat: %v, want no file after a cancelled Create", statErr)
	}
}

func mustEntry(t *testing.T, name string, kind domain.Kind, expires string) domain.Entry {
	t.Helper()
	n, err := domain.NewName(name)
	if err != nil {
		t.Fatalf("NewName: %v", err)
	}
	pw, err := domain.NewPassword("pw for " + name)
	if err != nil {
		t.Fatalf("NewPassword: %v", err)
	}
	exp, err := domain.ParseDate(expires)
	if err != nil {
		t.Fatalf("ParseDate: %v", err)
	}
	e, err := domain.NewEntry(domain.EntryFields{
		Name:      n,
		Kind:      kind,
		Username:  "user of " + name,
		Password:  pw,
		URL:       "https://" + name + ".example",
		Notes:     "notes for " + name,
		CreatedAt: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 11, 21, 8, 30, 15, 0, time.UTC),
		ExpiresOn: exp,
	})
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	return e
}

func assertSameFields(t *testing.T, got, want domain.EntryFields) {
	t.Helper()
	if got.Name.String() != want.Name.String() || got.Kind != want.Kind ||
		got.Username != want.Username || got.Password.Reveal() != want.Password.Reveal() ||
		got.URL != want.URL || got.Notes != want.Notes ||
		!got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) ||
		got.ExpiresOn.String() != want.ExpiresOn.String() || got.ExpiresOn.IsZero() != want.ExpiresOn.IsZero() {
		t.Fatalf("fields = %#v, want %#v", got, want)
	}
}

func TestSaveThenLoadRoundTripsEveryField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.db")
	store := newStore(path)
	if err := store.Create(t.Context(), master("first password"), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	withExpiry := mustEntry(t, "GitHub", domain.KindSite, "2027-01-31")
	noExpiry := mustEntry(t, "bank", domain.KindCard, "")
	v := domain.NewVault()
	for _, e := range []domain.Entry{withExpiry, noExpiry} {
		if err := v.Add(e); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	if err := store.Save(t.Context(), master("first password"), v); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := store.Load(t.Context(), master("first password"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", loaded.Len())
	}
	for _, want := range []domain.Entry{withExpiry, noExpiry} {
		got, err := loaded.Get(want.Name())
		if err != nil {
			t.Fatalf("Get(%s): %v", want.Name(), err)
		}
		assertSameFields(t, got.Fields(), want.Fields())
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".vault-*.tmp")); len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

func TestConsecutiveSavesUseFreshSaltAndNonce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.db")
	store := newStore(path)
	if err := store.Create(t.Context(), master("first password"), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	v := domain.NewVault()
	if err := v.Add(mustEntry(t, "github", domain.KindSite, "")); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var raws [2][]byte
	for i := range raws {
		if err := store.Save(t.Context(), master("first password"), v); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		raws[i] = raw
	}

	// Salt is bytes 21..37 and nonce 38..62 in the spec's header layout.
	if bytes.Equal(raws[0][21:37], raws[1][21:37]) {
		t.Error("two saves share a salt")
	}
	if bytes.Equal(raws[0][38:62], raws[1][38:62]) {
		t.Error("two saves share a nonce")
	}
}
