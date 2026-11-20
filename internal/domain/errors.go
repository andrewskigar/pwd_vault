// Package domain holds the vault's core value types and their invariants.
package domain

import "errors"

var (
	// ErrInvalid marks input that violates a domain invariant. It is wrapped
	// with detail, so callers check it with errors.Is.
	ErrInvalid   = errors.New("invalid input")
	ErrNotFound  = errors.New("entry not found")
	ErrDuplicate = errors.New("entry already exists")
)
