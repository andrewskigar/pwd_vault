//go:build !windows

package encfile

import (
	"fmt"
	"os"
)

// restrictFile makes path readable and writable by its owner only. The mode
// is set explicitly so the umask can neither loosen nor over-tighten it.
func restrictFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restrict vault permissions: %w", err)
	}
	return nil
}

// restrictDir limits a directory this store created to its owner.
func restrictDir(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("restrict vault permissions: %w", err)
	}
	return nil
}
