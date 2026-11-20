//go:build !windows

package encfile

import (
	"os"
	"path/filepath"
	"testing"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s has mode %v, want %v", path, got, want)
	}
}

func createInNewDir(t *testing.T) (dir, path string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "vault")
	path = filepath.Join(dir, "v.db")
	store := New(path, Params{Time: 1, MemoryKiB: 64, Threads: 1})
	if err := store.Create(t.Context(), app.NewMasterPassword([]byte("perm password")), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return dir, path
}

func TestCreateMakesOwnerOnlyVaultAndDirectory(t *testing.T) {
	dir, path := createInNewDir(t)

	assertMode(t, path, 0o600)
	assertMode(t, dir, 0o700)
}

func TestSaveKeepsVaultOwnerOnly(t *testing.T) {
	_, path := createInNewDir(t)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	store := New(path, Params{Time: 1, MemoryKiB: 64, Threads: 1})

	if err := store.Save(t.Context(), app.NewMasterPassword([]byte("perm password")), domain.NewVault()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	assertMode(t, path, 0o600)
}
