package encfile_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

// symlink creates link -> target, skipping the test where the OS does not
// let this user create symlinks.
func symlink(t *testing.T, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	if symlinkNotPermitted(err) {
		t.Skipf("creating symlinks is not permitted here (%v); resolve_test.go covers the logic", err)
	}
	if err != nil {
		t.Fatalf("Symlink: %v", err)
	}
}

func assertIsSymlinkTo(t *testing.T, link, target string) {
	t.Helper()
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("%s has mode %v, want a symlink", link, info.Mode())
	}
	got, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != target {
		t.Fatalf("link points to %q, want %q", got, target)
	}
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".vault-*.tmp")); len(leftovers) != 0 {
		t.Fatalf("temp files left behind in %s: %v", dir, leftovers)
	}
}

func TestSaveThroughSymlinkUpdatesTargetAndKeepsLink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "real.db")
	if err := newStore(target).Create(t.Context(), master("first password"), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	link := filepath.Join(t.TempDir(), "link.db")
	symlink(t, target, link)
	v := domain.NewVault()
	if err := v.Add(mustEntry(t, "github", domain.KindSite, "2027-01-31")); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := newStore(link).Save(t.Context(), master("first password"), v); err != nil {
		t.Fatalf("Save: %v", err)
	}

	assertIsSymlinkTo(t, link, target)
	for _, path := range []string{link, target} {
		got, err := newStore(path).Load(t.Context(), master("first password"))
		if err != nil {
			t.Fatalf("Load %s: %v", path, err)
		}
		if got.Len() != 1 {
			t.Fatalf("Load %s: Len() = %d, want 1", path, got.Len())
		}
	}
	assertNoTempFiles(t, filepath.Dir(link))
	assertNoTempFiles(t, filepath.Dir(target))
}

func TestLoadThroughDanglingSymlinkIsVaultNotFound(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link.db")
	symlink(t, filepath.Join(dir, "missing.db"), link)

	_, err := newStore(link).Load(t.Context(), master("first password"))

	if !errors.Is(err, app.ErrVaultNotFound) {
		t.Fatalf("Load error = %v, want app.ErrVaultNotFound", err)
	}
}

func TestCreateOnDanglingSymlinkIsVaultExistsAndKeepsLink(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.db")
	link := filepath.Join(dir, "link.db")
	symlink(t, missing, link)

	err := newStore(link).Create(t.Context(), master("first password"), domain.NewVault())

	if !errors.Is(err, app.ErrVaultExists) {
		t.Fatalf("Create error = %v, want app.ErrVaultExists", err)
	}
	assertIsSymlinkTo(t, link, missing)
	if _, err := os.Lstat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Lstat target: %v, want the dangling target still missing", err)
	}
	assertNoTempFiles(t, dir)
}
