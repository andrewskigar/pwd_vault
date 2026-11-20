// Package domain holds the vault's core value types and their invariants.
package domain

import "errors"

// ErrInvalid marks input that violates a domain invariant. It is wrapped with
// detail, so callers check it with errors.Is.
var ErrInvalid = errors.New("invalid input")
