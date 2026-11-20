package encfile

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

// These tests swap the resolve hook to stand in for a symlink, so the
// resolve-to-target logic is covered even where os.Symlink needs privileges
// (Windows without Developer Mode); symlink_test.go covers real links.

type renameCall struct{ oldpath, newpath string }

// resolvedStore creates a vault at a target path, then returns a store whose
// own path is a separate placeholder file that resolve maps to the target.
func resolvedStore(t *testing.T) (store *Store, linkPath, target string, renames *[]renameCall) {
	t.Helper()
	target = filepath.Join(t.TempDir(), "real.db")
	mp := app.NewMasterPassword([]byte("first password"))
	if err := New(target, Params{Time: 1, MemoryKiB: 64, Threads: 1}).Create(t.Context(), mp, domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	linkPath = filepath.Join(t.TempDir(), "link.db")
	if err := os.WriteFile(linkPath, []byte("stand-in for a symlink"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	store = New(linkPath, Params{Time: 1, MemoryKiB: 64, Threads: 1})
	store.resolve = func(p string) (string, error) {
		if p != linkPath {
			t.Errorf("resolve(%q), want %q", p, linkPath)
		}
		return target, nil
	}
	calls := []renameCall{}
	store.rename = func(oldpath, newpath string) error {
		calls = append(calls, renameCall{oldpath, newpath})
		return os.Rename(oldpath, newpath)
	}
	return store, linkPath, target, &calls
}

func oneEntryVault(t *testing.T) domain.Vault {
	t.Helper()
	name, err := domain.NewName("github")
	if err != nil {
		t.Fatalf("NewName: %v", err)
	}
	pw, err := domain.NewPassword("s3cret")
	if err != nil {
		t.Fatalf("NewPassword: %v", err)
	}
	stamp := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	e, err := domain.NewEntry(domain.EntryFields{
		Name: name, Kind: domain.KindSite, Password: pw, CreatedAt: stamp, UpdatedAt: stamp,
	})
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	v := domain.NewVault()
	if err := v.Add(e); err != nil {
		t.Fatalf("Add: %v", err)
	}
	return v
}

func TestSaveWritesTheResolvedTargetInItsOwnDirectory(t *testing.T) {
	store, linkPath, target, renames := resolvedStore(t)
	mp := app.NewMasterPassword([]byte("first password"))

	if err := store.Save(t.Context(), mp, oneEntryVault(t)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if len(*renames) != 1 {
		t.Fatalf("renames = %+v, want exactly one", *renames)
	}
	got := (*renames)[0]
	if got.newpath != target || filepath.Dir(got.oldpath) != filepath.Dir(target) {
		t.Fatalf("rename(%q, %q), want a temp file in %s renamed onto %s", got.oldpath, got.newpath, filepath.Dir(target), target)
	}
	link, err := os.ReadFile(linkPath)
	if err != nil || string(link) != "stand-in for a symlink" {
		t.Fatalf("link path now holds %q (err %v), want it untouched", link, err)
	}
	v, err := New(target, Params{Time: 1, MemoryKiB: 64, Threads: 1}).Load(t.Context(), mp)
	if err != nil {
		t.Fatalf("Load target: %v", err)
	}
	if v.Len() != 1 {
		t.Fatalf("target Len() = %d, want 1", v.Len())
	}
	for _, dir := range []string{filepath.Dir(linkPath), filepath.Dir(target)} {
		if leftovers, _ := filepath.Glob(filepath.Join(dir, ".vault-*.tmp")); len(leftovers) != 0 {
			t.Fatalf("temp files left behind in %s: %v", dir, leftovers)
		}
	}
}

func TestSaveFallsBackToThePathWhenNothingResolves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.db")
	store := New(path, Params{Time: 1, MemoryKiB: 64, Threads: 1})
	store.resolve = func(p string) (string, error) {
		return "", &fs.PathError{Op: "lstat", Path: p, Err: fs.ErrNotExist}
	}
	mp := app.NewMasterPassword([]byte("first password"))

	if err := store.Save(t.Context(), mp, oneEntryVault(t)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	v, err := New(path, Params{Time: 1, MemoryKiB: 64, Threads: 1}).Load(t.Context(), mp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", v.Len())
	}
}

func TestSaveFailsWhenThePathCannotBeResolved(t *testing.T) {
	store, linkPath, target, renames := resolvedStore(t)
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	injected := errors.New("injected resolve failure")
	store.resolve = func(string) (string, error) { return "", injected }

	err = store.Save(t.Context(), app.NewMasterPassword([]byte("first password")), oneEntryVault(t))

	if !errors.Is(err, injected) || !strings.Contains(err.Error(), "resolve vault path") {
		t.Fatalf("Save error = %v, want %q wrapping the resolve error", err, "resolve vault path")
	}
	if len(*renames) != 0 {
		t.Fatalf("renames = %+v, want none", *renames)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("the target changed after a failed resolve")
	}
	for _, dir := range []string{filepath.Dir(linkPath), filepath.Dir(target)} {
		if leftovers, _ := filepath.Glob(filepath.Join(dir, ".vault-*.tmp")); len(leftovers) != 0 {
			t.Fatalf("temp files left behind in %s: %v", dir, leftovers)
		}
	}
}
