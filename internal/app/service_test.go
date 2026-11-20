package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

// memStore is an in-memory app.VaultStore that counts saves.
type memStore struct {
	vault domain.Vault
	saves int
}

func (s *memStore) Create(_ context.Context, _ app.MasterPassword, v domain.Vault) error {
	s.vault = v
	return nil
}

func (s *memStore) Load(context.Context, app.MasterPassword) (domain.Vault, error) {
	return s.vault, nil
}

func (s *memStore) Save(_ context.Context, _ app.MasterPassword, v domain.Vault) error {
	s.saves++
	s.vault = v
	return nil
}

func mustName(t *testing.T, s string) domain.Name {
	t.Helper()
	n, err := domain.NewName(s)
	if err != nil {
		t.Fatalf("NewName(%q): %v", s, err)
	}
	return n
}

func draft(t *testing.T, name string) app.EntryDraft {
	t.Helper()
	pw, err := domain.NewPassword("entry pw")
	if err != nil {
		t.Fatalf("NewPassword: %v", err)
	}
	return app.EntryDraft{Name: mustName(t, name), Kind: domain.KindSite, Password: pw}
}

func TestAddStampsUTCSecondPrecisionTimestamps(t *testing.T) {
	store := &memStore{vault: domain.NewVault()}
	plusTwo := time.FixedZone("UTC+2", 2*60*60)
	clock := func() time.Time { return time.Date(2026, 11, 20, 12, 0, 5, 987654321, plusTwo) }
	svc := app.NewService(store, clock)

	e, err := svc.Add(t.Context(), app.NewMasterPassword([]byte("master")), draft(t, "github"))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	want := time.Date(2026, 11, 20, 10, 0, 5, 0, time.UTC)
	for label, got := range map[string]time.Time{"CreatedAt": e.CreatedAt(), "UpdatedAt": e.UpdatedAt()} {
		if !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("%s = %v, want %v", label, got, want)
		}
	}
	if store.saves != 1 {
		t.Fatalf("saves = %d, want 1", store.saves)
	}
}

func TestDuplicateAddAndGetNeverSave(t *testing.T) {
	store := &memStore{vault: domain.NewVault()}
	svc := app.NewService(store, func() time.Time { return time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC) })
	mp := app.NewMasterPassword([]byte("master"))
	if _, err := svc.Add(t.Context(), mp, draft(t, "github")); err != nil {
		t.Fatalf("Add: %v", err)
	}

	_, dupErr := svc.Add(t.Context(), mp, draft(t, "GitHub"))
	got, getErr := svc.Get(t.Context(), mp, mustName(t, "GITHUB"))
	_, missingErr := svc.Get(t.Context(), mp, mustName(t, "nope"))

	if !errors.Is(dupErr, domain.ErrDuplicate) {
		t.Errorf("duplicate Add error = %v, want domain.ErrDuplicate", dupErr)
	}
	if getErr != nil || got.Name().String() != "github" {
		t.Errorf("Get = %q, %v, want the stored github entry", got.Name(), getErr)
	}
	if !errors.Is(missingErr, domain.ErrNotFound) {
		t.Errorf("missing Get error = %v, want domain.ErrNotFound", missingErr)
	}
	if store.saves != 1 {
		t.Fatalf("saves = %d, want only the first Add to save", store.saves)
	}
}

func TestListFiltersByKindSortsByNameAndNeverSaves(t *testing.T) {
	store := &memStore{vault: domain.NewVault()}
	svc := app.NewService(store, func() time.Time { return time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC) })
	mp := app.NewMasterPassword([]byte("master"))
	for _, d := range []app.EntryDraft{withKind(draft(t, "zeta"), domain.KindNote), draft(t, "Alpha"), withKind(draft(t, "beta"), domain.KindCard), draft(t, "Gamma")} {
		if _, err := svc.Add(t.Context(), mp, d); err != nil {
			t.Fatalf("Add %s: %v", d.Name, err)
		}
	}
	tests := []struct {
		name   string
		filter app.ListFilter
		want   string
	}{
		{name: "all kinds", filter: app.ListFilter{}, want: "Alpha,beta,Gamma,zeta"},
		{name: "site", filter: app.ListFilter{Kind: domain.KindSite}, want: "Alpha,Gamma"},
		{name: "card", filter: app.ListFilter{Kind: domain.KindCard}, want: "beta"},
		{name: "no matches", filter: app.ListFilter{Kind: domain.KindOther}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := svc.List(t.Context(), mp, tt.filter)
			if err != nil {
				t.Fatalf("List: %v", err)
			}

			var names []string
			for _, e := range entries {
				names = append(names, e.Name().String())
			}
			if got := strings.Join(names, ","); got != tt.want {
				t.Fatalf("names = %q, want %q", got, tt.want)
			}
		})
	}
	if store.saves != 4 {
		t.Fatalf("saves = %d, want only the four Adds to save", store.saves)
	}
}

func withKind(d app.EntryDraft, k domain.Kind) app.EntryDraft {
	d.Kind = k
	return d
}
