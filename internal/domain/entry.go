package domain

import (
	"fmt"
	"time"
)

const (
	maxUsernameBytes = 256
	maxURLBytes      = 2048
	maxNotesBytes    = 8192
)

// EntryFields is the immutable DTO used to build and restore entries.
type EntryFields struct {
	Name      Name
	Kind      Kind
	Username  string
	Password  Password
	URL       string
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresOn Date
}

// Entry is one stored credential. It is valid by construction.
type Entry struct{ f EntryFields }

// NewEntry returns ErrInvalid unless f satisfies every entry invariant.
func NewEntry(f EntryFields) (Entry, error) {
	if err := f.validate(); err != nil {
		return Entry{}, err
	}
	return Entry{f: f}, nil
}

func (f EntryFields) validate() error {
	switch {
	case f.Name.IsZero():
		return fmt.Errorf("name is required: %w", ErrInvalid)
	case !f.Kind.valid():
		return fmt.Errorf("kind must be site, card, note or other: %w", ErrInvalid)
	case f.Password.IsZero():
		return fmt.Errorf("password is required: %w", ErrInvalid)
	case len(f.Username) > maxUsernameBytes:
		return fmt.Errorf("username must be at most %d bytes: %w", maxUsernameBytes, ErrInvalid)
	case len(f.URL) > maxURLBytes:
		return fmt.Errorf("URL must be at most %d bytes: %w", maxURLBytes, ErrInvalid)
	case len(f.Notes) > maxNotesBytes:
		return fmt.Errorf("notes must be at most %d bytes: %w", maxNotesBytes, ErrInvalid)
	case f.CreatedAt.IsZero() || f.UpdatedAt.IsZero():
		return fmt.Errorf("created and updated times are required: %w", ErrInvalid)
	case f.UpdatedAt.Before(f.CreatedAt):
		return fmt.Errorf("updated time must not precede created time: %w", ErrInvalid)
	}
	return nil
}

func (e Entry) Name() Name           { return e.f.Name }
func (e Entry) Kind() Kind           { return e.f.Kind }
func (e Entry) Username() string     { return e.f.Username }
func (e Entry) Password() Password   { return e.f.Password }
func (e Entry) URL() string          { return e.f.URL }
func (e Entry) Notes() string        { return e.f.Notes }
func (e Entry) CreatedAt() time.Time { return e.f.CreatedAt }
func (e Entry) UpdatedAt() time.Time { return e.f.UpdatedAt }
func (e Entry) ExpiresOn() Date      { return e.f.ExpiresOn }

// Fields returns a copy of the entry's data for storage and updates.
func (e Entry) Fields() EntryFields { return e.f }
