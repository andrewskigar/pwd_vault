package cli

import (
	"errors"
	"fmt"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

// vaultFailure turns vault-level use-case errors into the spec's user-facing
// messages; other errors pass through unchanged.
func vaultFailure(err error, path string) error {
	switch {
	case errors.Is(err, app.ErrUnlock):
		// Bare sentinel: the message must not reveal which step failed.
		return app.ErrUnlock
	case errors.Is(err, app.ErrVaultNotFound):
		return fmt.Errorf("no vault at %s; run \"save_pwd init\" first", path)
	case errors.Is(err, app.ErrVaultExists):
		return fmt.Errorf("vault already exists at %s", path)
	}
	return err
}

// entryFailure is vaultFailure plus the per-entry errors, named as the user
// typed them.
func entryFailure(err error, path string, name domain.Name) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return fmt.Errorf("entry %q not found", name.String())
	case errors.Is(err, domain.ErrDuplicate):
		return fmt.Errorf("entry %q already exists", name.String())
	}
	return vaultFailure(err, path)
}
