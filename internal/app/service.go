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
}

// Clock returns the current time.
type Clock func() time.Time

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
