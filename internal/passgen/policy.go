// Package passgen generates random passwords from a policy, with the
// randomness source injected by the caller.
package passgen

import (
	"fmt"

	"save_pwd/internal/domain"
)

// Classes is a bit set of character classes.
type Classes uint8

const (
	Lower Classes = 1 << iota
	Upper
	Digits
	Symbols
)

// AllClasses enables every character class.
const AllClasses = Lower | Upper | Digits | Symbols

// Ambiguity selects whether look-alike characters may appear.
type Ambiguity uint8

const (
	AllowAmbiguous Ambiguity = iota
	ExcludeAmbiguous
)

const (
	MinLength     = 8
	MaxLength     = 256
	DefaultLength = 20
	// WeakBelow is the length under which a password is accepted but flagged.
	WeakBelow = 12
)

// Policy is a validated set of generation options.
type Policy struct {
	length    int
	classes   Classes
	ambiguity Ambiguity
}

// NewPolicy returns an error wrapping domain.ErrInvalid when the length is
// outside MinLength..MaxLength, no known class is enabled, or the ambiguity
// option is unknown.
func NewPolicy(length int, classes Classes, a Ambiguity) (Policy, error) {
	if length < MinLength || length > MaxLength {
		return Policy{}, fmt.Errorf("length %d must be between %d and %d: %w", length, MinLength, MaxLength, domain.ErrInvalid)
	}
	if classes == 0 || classes&^AllClasses != 0 {
		return Policy{}, fmt.Errorf("at least one known character class is required: %w", domain.ErrInvalid)
	}
	if a != AllowAmbiguous && a != ExcludeAmbiguous {
		return Policy{}, fmt.Errorf("unknown ambiguity option %d: %w", a, domain.ErrInvalid)
	}
	return Policy{length: length, classes: classes, ambiguity: a}, nil
}

// DefaultPolicy is 20 characters from every class, ambiguous characters allowed.
func DefaultPolicy() Policy {
	return Policy{length: DefaultLength, classes: AllClasses, ambiguity: AllowAmbiguous}
}

// Length is the number of characters the policy generates.
func (p Policy) Length() int { return p.length }

// IsWeak reports whether the length is below WeakBelow.
func (p Policy) IsWeak() bool { return p.length < WeakBelow }
