package encfile

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

// fileFullControl is what Windows stores for GENERIC_ALL on a file or
// directory once it maps the generic right (SDDL "FA").
const fileFullControl windows.ACCESS_MASK = 0x1f01ff

const (
	noInheritance   uint8 = 0
	objectAndFolder uint8 = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
)

type allowACE struct {
	sid   *windows.SID
	mask  windows.ACCESS_MASK
	flags uint8
}

func cheapStore(path string) *Store {
	return New(path, Params{Time: 1, MemoryKiB: 64, Threads: 1})
}

func testMaster() app.MasterPassword {
	return app.NewMasterPassword([]byte("acl password"))
}

// assertOwnerOnly reads path's DACL back from the filesystem and requires it
// to be protected and to hold exactly one full-control allow ACE for the
// current user and one for SYSTEM, each with wantFlags.
func assertOwnerOnly(t *testing.T, path string, wantFlags uint8) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("read security descriptor of %s: %v", path, err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatalf("read control of %s: %v", path, err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("%s: DACL is not protected (%s)", path, sd)
	}
	aces := readACEs(t, sd)
	if len(aces) != 2 {
		t.Fatalf("%s: %d ACEs, want 2 (%s)", path, len(aces), sd)
	}
	for _, want := range []*windows.SID{currentUserSID(t), systemSID(t)} {
		if !hasFullControlACE(aces, want, wantFlags) {
			t.Errorf("%s: no full-control allow ACE with flags %#x for %s (%s)", path, wantFlags, want, sd)
		}
	}
}

func hasFullControlACE(aces []allowACE, sid *windows.SID, flags uint8) bool {
	for _, a := range aces {
		fullControl := a.mask == fileFullControl || a.mask == windows.GENERIC_ALL
		if a.sid.Equals(sid) && fullControl && a.flags == flags {
			return true
		}
	}
	return false
}

// readACEs returns every ACE of sd's DACL; a non-allow ACE fails the test.
func readACEs(t *testing.T, sd *windows.SECURITY_DESCRIPTOR) []allowACE {
	t.Helper()
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatalf("read DACL: %v", err)
	}
	if dacl == nil {
		t.Fatalf("null DACL grants everyone access (%s)", sd)
	}
	aces := make([]allowACE, 0, dacl.AceCount)
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatalf("read ACE %d: %v", i, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatalf("ACE %d has type %d, want allow (%s)", i, ace.Header.AceType, sd)
		}
		// The SID is stored inline, starting at SidStart.
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		aces = append(aces, allowACE{sid: sid, mask: ace.Mask, flags: ace.Header.AceFlags})
	}
	return aces
}

func currentUserSID(t *testing.T) *windows.SID {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("token user: %v", err)
	}
	return user.User.Sid
}

func systemSID(t *testing.T) *windows.SID {
	t.Helper()
	sid, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatalf("SYSTEM SID: %v", err)
	}
	return sid
}

func daclSDDL(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("read security descriptor of %s: %v", path, err)
	}
	return sd.String()
}

func TestCreateGivesVaultOwnerOnlyDACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.db")

	if err := cheapStore(path).Create(t.Context(), testMaster(), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	assertOwnerOnly(t, path, noInheritance)
}

func TestSaveGivesExistingVaultOwnerOnlyDACL(t *testing.T) {
	dir := t.TempDir()
	made := filepath.Join(dir, "made.db")
	if err := cheapStore(made).Create(t.Context(), testMaster(), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	raw, err := os.ReadFile(made)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// A copy written by plain os.WriteFile carries the directory's inherited
	// DACL, like a vault saved before owner-only permissions existed.
	path := filepath.Join(dir, "old.db")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := cheapStore(path).Save(t.Context(), testMaster(), domain.NewVault()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	assertOwnerOnly(t, path, noInheritance)
}

func TestTempFileHasOwnerOnlyDACLBeforeRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.db")
	store := cheapStore(path)
	if err := store.Create(t.Context(), testMaster(), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	inspected := 0
	store.rename = func(oldpath, newpath string) error {
		inspected++
		assertOwnerOnly(t, oldpath, noInheritance)
		return os.Rename(oldpath, newpath)
	}

	if err := store.Save(t.Context(), testMaster(), domain.NewVault()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if inspected != 1 {
		t.Fatalf("rename ran %d times, want 1", inspected)
	}
}

func TestCreatedDirectoryGetsInheritableOwnerOnlyDACL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	path := filepath.Join(dir, "v.db")

	if err := cheapStore(path).Create(t.Context(), testMaster(), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	assertOwnerOnly(t, dir, objectAndFolder)
	assertOwnerOnly(t, path, noInheritance)
}

func TestCreateLeavesExistingDirectoryDACLUnchanged(t *testing.T) {
	dir := t.TempDir()
	before := daclSDDL(t, dir)

	if err := cheapStore(filepath.Join(dir, "v.db")).Create(t.Context(), testMaster(), domain.NewVault()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if after := daclSDDL(t, dir); after != before {
		t.Fatalf("existing directory DACL changed:\nbefore %s\nafter  %s", before, after)
	}
}
