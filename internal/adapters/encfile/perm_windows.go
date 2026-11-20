package encfile

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// fileAllAccess is FILE_ALL_ACCESS (SDDL "FA"), which x/sys does not define.
// The specific rights are granted instead of GENERIC_ALL because Windows
// splits an inheritable GENERIC_ALL grant into two ACEs per trustee (an
// effective FA one and an inherit-only GA one); specific rights stay one.
const fileAllAccess windows.ACCESS_MASK = windows.STANDARD_RIGHTS_ALL | 0x1ff

// restrictFile replaces path's DACL with a protected one that grants full
// control only to the current user and SYSTEM, so the vault no longer
// inherits access for Administrators or other users from its directory.
func restrictFile(path string) error {
	return applyOwnerOnlyDACL(path, windows.NO_INHERITANCE)
}

// restrictDir does the same for a directory this store created, and lets
// files and folders created inside it inherit the two ACEs.
func restrictDir(path string) error {
	return applyOwnerOnlyDACL(path, windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT)
}

// applyOwnerOnlyDACL uses the path-based call because Go opens files
// without WRITE_DAC. Volumes without ACLs (FAT/exFAT) report
// ERROR_NOT_SUPPORTED; the vault is still written there, unprotected.
func applyOwnerOnlyDACL(path string, inheritance uint32) error {
	acl, err := ownerOnlyACL(inheritance)
	if err != nil {
		return fmt.Errorf("restrict vault permissions: %w", err)
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil)
	if errors.Is(err, windows.ERROR_NOT_SUPPORTED) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("restrict vault permissions: %w", err)
	}
	return nil
}

func ownerOnlyACL(inheritance uint32) (*windows.ACL, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("current user: %w", err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, fmt.Errorf("SYSTEM account: %w", err)
	}
	entries := []windows.EXPLICIT_ACCESS{
		fullControl(user.User.Sid, windows.TRUSTEE_IS_USER, inheritance),
		fullControl(system, windows.TRUSTEE_IS_WELL_KNOWN_GROUP, inheritance),
	}
	return windows.ACLFromEntries(entries, nil)
}

func fullControl(sid *windows.SID, kind windows.TRUSTEE_TYPE, inheritance uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: fileAllAccess,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  kind,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}
