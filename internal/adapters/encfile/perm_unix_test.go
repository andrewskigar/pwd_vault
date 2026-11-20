//go:build unix

package encfile

import (
	"syscall"
	"testing"
)

// A restrictive umask must not leave the owner without write access: the
// modes are set explicitly, not left to the umask. The umask is process-wide,
// which is safe because encfile tests do not run in parallel.
func TestRestrictiveUmaskStillGivesOwnerOnlyModes(t *testing.T) {
	previous := syscall.Umask(0o277)
	t.Cleanup(func() { syscall.Umask(previous) })

	dir, path := createInNewDir(t)

	assertMode(t, path, 0o600)
	assertMode(t, dir, 0o700)
}
