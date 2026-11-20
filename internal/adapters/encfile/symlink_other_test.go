//go:build !windows

package encfile_test

// symlinkNotPermitted is always false off Windows: creating a symlink in a
// test's temp directory needs no privilege there.
func symlinkNotPermitted(error) bool { return false }
