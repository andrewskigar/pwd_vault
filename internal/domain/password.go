package domain

import "fmt"

const (
	maxPasswordBytes = 1024
	// The mask has a fixed width so formatted output never reveals the length.
	passwordMask = "********"
)

// Password is an entry secret. String and GoString always return a mask, so
// fmt and slog never leak it; only Reveal exposes the value.
type Password struct{ value string }

// NewPassword returns ErrInvalid when s is empty or longer than 1024 bytes.
func NewPassword(s string) (Password, error) {
	if s == "" {
		return Password{}, fmt.Errorf("password must not be empty: %w", ErrInvalid)
	}
	if len(s) > maxPasswordBytes {
		return Password{}, fmt.Errorf("password must be at most %d bytes: %w", maxPasswordBytes, ErrInvalid)
	}
	return Password{value: s}, nil
}

// Reveal returns the secret value.
func (p Password) Reveal() string { return p.value }

// IsZero reports whether p is the zero Password.
func (p Password) IsZero() bool { return p.value == "" }

func (p Password) String() string { return passwordMask }

func (p Password) GoString() string { return "domain.Password(" + passwordMask + ")" }
