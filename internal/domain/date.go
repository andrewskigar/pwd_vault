package domain

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Date is a civil date such as an expiry day. The zero Date means "no date".
type Date struct{ t time.Time }

// ParseDate parses YYYY-MM-DD; the empty string is the zero Date.
func ParseDate(s string) (Date, error) {
	if s == "" {
		return Date{}, nil
	}
	t, err := time.Parse(dateLayout, s)
	// The round trip rejects forms time.Parse tolerates but users should not
	// rely on, so a stored date always prints back exactly as entered.
	if err != nil || t.Format(dateLayout) != s {
		return Date{}, fmt.Errorf("date must be a real day written YYYY-MM-DD: %w", ErrInvalid)
	}
	return Date{t: t}, nil
}

// String returns YYYY-MM-DD, or "" for the zero Date.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.t.Format(dateLayout)
}

// IsZero reports whether d is "no date".
func (d Date) IsZero() bool { return d.t.IsZero() }
