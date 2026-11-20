package encfile_test

import (
	"errors"
	"syscall"
)

// symlinkNotPermitted reports the error Windows returns when the user may
// not create symlinks (no Developer Mode, not elevated).
func symlinkNotPermitted(err error) bool {
	return errors.Is(err, syscall.ERROR_PRIVILEGE_NOT_HELD)
}
