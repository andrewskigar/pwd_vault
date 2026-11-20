package encfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

func TestFailedRenameKeepsPreviousVaultAndRemovesTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v.db")
	mp := app.NewMasterPassword([]byte("first password"))
	store := New(path, Params{Time: 1, MemoryKiB: 64, Threads: 1})
	if err := store.Create(t.Context(), mp, domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	injected := errors.New("injected rename failure")
	store.rename = func(string, string) error { return injected }

	err = store.Save(t.Context(), mp, domain.NewVault())

	if !errors.Is(err, injected) {
		t.Fatalf("Save error = %v, want the rename error wrapped", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("the vault file changed after a failed rename")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".vault-*.tmp")); len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}
