package app

import (
	"context"
	"fmt"

	"save_pwd/internal/domain"
)

// Session is one unlocked vault serving a single command: Open loads and
// decrypts it once, and each mutation saves at most once. A failed Save leaves
// the in-memory vault already changed, so discard the session after any error.
type Session struct {
	store VaultStore
	now   Clock
	// mp is borrowed, not copied: the caller still owns and wipes it.
	mp    MasterPassword
	vault domain.Vault
}

// Open loads the vault once: ErrVaultNotFound or ErrUnlock, wrapped
// "load vault: %w".
func (s *Service) Open(ctx context.Context, mp MasterPassword) (*Session, error) {
	v, err := s.load(ctx, mp)
	if err != nil {
		return nil, err
	}
	return &Session{store: s.store, now: s.now, mp: mp, vault: v}, nil
}

// Find returns the entry named n, ignoring case, or domain.ErrNotFound. It
// never touches the store.
func (s *Session) Find(n domain.Name) (domain.Entry, error) {
	e, err := s.vault.Get(n)
	if err != nil {
		return domain.Entry{}, fmt.Errorf("get entry: %w", err)
	}
	return e, nil
}

// Add builds the entry, stamped now, adds it and saves once. domain.ErrInvalid
// and domain.ErrDuplicate (the name is taken, ignoring case) return without
// saving.
func (s *Session) Add(ctx context.Context, d EntryDraft) (domain.Entry, error) {
	now := s.now.timestamp()
	e, err := domain.NewEntry(domain.EntryFields{
		Name:      d.Name,
		Kind:      d.Kind,
		Username:  d.Username,
		Password:  d.Password,
		URL:       d.URL,
		Notes:     d.Notes,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresOn: d.ExpiresOn,
	})
	if err != nil {
		return domain.Entry{}, fmt.Errorf("new entry: %w", err)
	}
	if err := s.vault.Add(e); err != nil {
		return domain.Entry{}, fmt.Errorf("add entry: %w", err)
	}
	if err := s.save(ctx); err != nil {
		return domain.Entry{}, err
	}
	return e, nil
}

// Update applies c to the entry named n, ignoring case, and saves once. It
// keeps the creation time and stored display name and stamps the update time.
// ErrNoChanges, domain.ErrNotFound and domain.ErrInvalid return without saving.
func (s *Session) Update(ctx context.Context, n domain.Name, c EntryChanges) (domain.Entry, error) {
	if c.IsEmpty() {
		return domain.Entry{}, ErrNoChanges
	}
	current, err := s.Find(n)
	if err != nil {
		return domain.Entry{}, err
	}
	f := c.applyTo(current.Fields())
	f.UpdatedAt = s.now.timestamp()
	e, err := domain.NewEntry(f)
	if err != nil {
		return domain.Entry{}, fmt.Errorf("update entry: %w", err)
	}
	if err := s.vault.Replace(e); err != nil {
		return domain.Entry{}, fmt.Errorf("replace entry: %w", err)
	}
	if err := s.save(ctx); err != nil {
		return domain.Entry{}, err
	}
	return e, nil
}

// Delete removes the entry named n, ignoring case, and saves once; a missing
// entry is domain.ErrNotFound and nothing is saved.
func (s *Session) Delete(ctx context.Context, n domain.Name) error {
	if err := s.vault.Remove(n); err != nil {
		return fmt.Errorf("remove entry: %w", err)
	}
	return s.save(ctx)
}

func (s *Session) save(ctx context.Context) error {
	if err := s.store.Save(ctx, s.mp, s.vault); err != nil {
		return fmt.Errorf("save vault: %w", err)
	}
	return nil
}
