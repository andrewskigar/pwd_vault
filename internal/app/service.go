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
// name is taken (ignoring case).
func (s *Service) Add(ctx context.Context, mp MasterPassword, d EntryDraft) (domain.Entry, error) {
	now := s.timestamp()
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
	v, err := s.load(ctx, mp)
	if err != nil {
		return domain.Entry{}, err
	}
	if err := v.Add(e); err != nil {
		return domain.Entry{}, fmt.Errorf("add entry: %w", err)
	}
	if err := s.store.Save(ctx, mp, v); err != nil {
		return domain.Entry{}, fmt.Errorf("save vault: %w", err)
	}
	return e, nil
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

func (s *Service) load(ctx context.Context, mp MasterPassword) (domain.Vault, error) {
	v, err := s.store.Load(ctx, mp)
	if err != nil {
		return domain.Vault{}, fmt.Errorf("load vault: %w", err)
	}
	return v, nil
}

// timestamp is the stored form of "now": UTC in whole seconds, so stored and
// printed times agree.
func (s *Service) timestamp() time.Time {
	return s.now().UTC().Truncate(time.Second)
}
