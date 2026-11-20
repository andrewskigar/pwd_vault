package app

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"save_pwd/internal/domain"
)

const minMasterRunes = 8

// VaultStore is the driven port for one vault location.
type VaultStore interface {
	// Create writes a new vault and never overwrites: ErrVaultExists.
	Create(ctx context.Context, mp MasterPassword, v domain.Vault) error
	// Load decrypts the vault: ErrVaultNotFound or ErrUnlock.
	Load(ctx context.Context, mp MasterPassword) (domain.Vault, error)
	// Save atomically replaces the stored vault.
	Save(ctx context.Context, mp MasterPassword, v domain.Vault) error
}

// Clock returns the current time.
type Clock func() time.Time

// EntryDraft is the input for Add; the timestamps come from the Clock.
type EntryDraft struct {
	Name      domain.Name
	Kind      domain.Kind
	Username  string
	Password  domain.Password
	URL       string
	Notes     string
	ExpiresOn domain.Date
}

// EntryChanges is the input for Update. A nil field leaves the entry's value
// unchanged; a pointer to "" or to the zero Date clears it.
type EntryChanges struct {
	Kind      *domain.Kind
	Username  *string
	Password  *domain.Password
	URL       *string
	Notes     *string
	ExpiresOn *domain.Date
}

// IsEmpty reports whether c requests no change at all.
func (c EntryChanges) IsEmpty() bool {
	return c == EntryChanges{}
}

// applyTo returns f with every requested change made.
func (c EntryChanges) applyTo(f domain.EntryFields) domain.EntryFields {
	if c.Kind != nil {
		f.Kind = *c.Kind
	}
	if c.Username != nil {
		f.Username = *c.Username
	}
	if c.Password != nil {
		f.Password = *c.Password
	}
	if c.URL != nil {
		f.URL = *c.URL
	}
	if c.Notes != nil {
		f.Notes = *c.Notes
	}
	if c.ExpiresOn != nil {
		f.ExpiresOn = *c.ExpiresOn
	}
	return f
}

// ListFilter selects the entries List returns; the zero Kind means all kinds.
type ListFilter struct {
	Kind domain.Kind
}

// Service runs the vault use cases against one store.
type Service struct {
	store VaultStore
	now   Clock
}

// NewService wires the use cases to their store and clock.
func NewService(store VaultStore, now Clock) *Service {
	return &Service{store: store, now: now}
}

// Init creates an empty vault protected by mp.
func (s *Service) Init(ctx context.Context, mp MasterPassword) error {
	if utf8.RuneCount(mp.Bytes()) < minMasterRunes {
		return ErrWeakMaster
	}
	if err := s.store.Create(ctx, mp, domain.NewVault()); err != nil {
		return fmt.Errorf("create vault: %w", err)
	}
	return nil
}

// Add stores a new entry created now, or returns domain.ErrDuplicate when the
// name is taken (ignoring case). It is Open followed by Session.Add, so the
// vault is loaded before the draft is validated.
func (s *Service) Add(ctx context.Context, mp MasterPassword, d EntryDraft) (domain.Entry, error) {
	sess, err := s.Open(ctx, mp)
	if err != nil {
		return domain.Entry{}, err
	}
	return sess.Add(ctx, d)
}

// Get returns the entry named n, ignoring case, or domain.ErrNotFound.
func (s *Service) Get(ctx context.Context, mp MasterPassword, n domain.Name) (domain.Entry, error) {
	v, err := s.load(ctx, mp)
	if err != nil {
		return domain.Entry{}, err
	}
	e, err := v.Get(n)
	if err != nil {
		return domain.Entry{}, fmt.Errorf("get entry: %w", err)
	}
	return e, nil
}

// List returns the entries matching f, sorted by name ignoring case. It only
// reads the vault, so it never saves.
func (s *Service) List(ctx context.Context, mp MasterPassword, f ListFilter) ([]domain.Entry, error) {
	v, err := s.load(ctx, mp)
	if err != nil {
		return nil, err
	}
	var matches []domain.Entry
	for _, e := range v.Entries() {
		if f.matches(e) {
			matches = append(matches, e)
		}
	}
	return matches, nil
}

// Update applies c to the entry named n, ignoring case. It keeps the creation
// time and stored display name, stamps the update time, and saves only when the
// changed entry is valid: ErrNoChanges, domain.ErrNotFound, domain.ErrInvalid.
func (s *Service) Update(ctx context.Context, mp MasterPassword, n domain.Name, c EntryChanges) (domain.Entry, error) {
	if c.IsEmpty() {
		return domain.Entry{}, ErrNoChanges
	}
	v, err := s.load(ctx, mp)
	if err != nil {
		return domain.Entry{}, err
	}
	current, err := v.Get(n)
	if err != nil {
		return domain.Entry{}, fmt.Errorf("get entry: %w", err)
	}
	f := c.applyTo(current.Fields())
	f.UpdatedAt = s.now.timestamp()
	e, err := domain.NewEntry(f)
	if err != nil {
		return domain.Entry{}, fmt.Errorf("update entry: %w", err)
	}
	if err := v.Replace(e); err != nil {
		return domain.Entry{}, fmt.Errorf("replace entry: %w", err)
	}
	if err := s.store.Save(ctx, mp, v); err != nil {
		return domain.Entry{}, fmt.Errorf("save vault: %w", err)
	}
	return e, nil
}

// Delete removes the entry named n, ignoring case, and saves; a missing entry
// is domain.ErrNotFound and nothing is saved.
func (s *Service) Delete(ctx context.Context, mp MasterPassword, n domain.Name) error {
	sess, err := s.Open(ctx, mp)
	if err != nil {
		return err
	}
	return sess.Delete(ctx, n)
}

func (f ListFilter) matches(e domain.Entry) bool {
	return f.Kind == "" || e.Kind() == f.Kind
}

func (s *Service) load(ctx context.Context, mp MasterPassword) (domain.Vault, error) {
	v, err := s.store.Load(ctx, mp)
	if err != nil {
		return domain.Vault{}, fmt.Errorf("load vault: %w", err)
	}
	return v, nil
}

// timestamp is the stored form of "now": UTC in whole seconds, so stored and
// printed times agree.
func (c Clock) timestamp() time.Time {
	return c().UTC().Truncate(time.Second)
}
