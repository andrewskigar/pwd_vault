package domain

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxNameRunes = 128

// Name identifies an entry. It keeps the case the user typed for display, but
// two names that differ only in case are the same entry.
type Name struct{ display string }

// NewName trims surrounding space and returns ErrInvalid unless the result is
// 1..128 runes of valid UTF-8 without control characters.
func NewName(s string) (Name, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) {
		return Name{}, fmt.Errorf("name must be valid UTF-8: %w", ErrInvalid)
	}
	if n := utf8.RuneCountInString(s); n == 0 || n > maxNameRunes {
		return Name{}, fmt.Errorf("name must be 1 to %d characters: %w", maxNameRunes, ErrInvalid)
	}
	if strings.ContainsFunc(s, unicode.IsControl) {
		return Name{}, fmt.Errorf("name must not contain control characters: %w", ErrInvalid)
	}
	return Name{display: s}, nil
}

func (n Name) String() string { return n.display }

// Key is the case-insensitive identity used for uniqueness and lookup.
func (n Name) Key() string { return strings.ToLower(n.display) }

// IsZero reports whether n is the zero Name.
func (n Name) IsZero() bool { return n.display == "" }
