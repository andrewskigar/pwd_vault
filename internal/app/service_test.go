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

// seededForUpdate returns a service whose vault holds one fully populated
// "GitHub" entry created at 10:00 UTC, the clock it reads (so tests can move
// it on), and the store with its save counter reset.
func seededForUpdate(t *testing.T) (*app.Service, *time.Time, *memStore) {
	t.Helper()
	store := &memStore{vault: domain.NewVault()}
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	svc := app.NewService(store, func() time.Time { return now })
	d := draft(t, "GitHub")
	d.Username = "alice"
	d.URL = "https://github.com"
	d.Notes = "2FA on"
	expires, err := domain.ParseDate("2027-01-31")
	if err != nil {
		t.Fatalf("ParseDate: %v", err)
	}
	d.ExpiresOn = expires
	if _, err := svc.Add(t.Context(), app.NewMasterPassword([]byte("master")), d); err != nil {
		t.Fatalf("Add: %v", err)
	}
	store.saves = 0
	return svc, &now, store
}

func TestUpdateChangesOnlyGivenFieldsKeepsCreatedAndBumpsUpdated(t *testing.T) {
	svc, now, store := seededForUpdate(t)
	*now = time.Date(2026, 11, 21, 9, 30, 15, 500000000, time.FixedZone("UTC+2", 2*60*60))
	bob, empty := "bob", ""
	noExpiry := domain.Date{}

	e, err := svc.Update(t.Context(), app.NewMasterPassword([]byte("master")), mustName(t, "github"),
		app.EntryChanges{Username: &bob, Notes: &empty, ExpiresOn: &noExpiry})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	stored, err := svc.Get(t.Context(), app.NewMasterPassword([]byte("master")), mustName(t, "github"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	for label, got := range map[string]domain.Entry{"returned": e, "stored": stored} {
		checks := []struct{ field, got, want string }{
			{"name", got.Name().String(), "GitHub"},
			{"kind", string(got.Kind()), "site"},
			{"username", got.Username(), "bob"},
			{"password", got.Password().Reveal(), "entry pw"},
			{"url", got.URL(), "https://github.com"},
			{"notes", got.Notes(), ""},
			{"expires", got.ExpiresOn().String(), ""},
			{"created", got.CreatedAt().Format(time.RFC3339), "2026-11-20T10:00:00Z"},
			{"updated", got.UpdatedAt().Format(time.RFC3339), "2026-11-21T07:30:15Z"},
		}
		for _, c := range checks {
			if c.got != c.want {
				t.Errorf("%s %s = %q, want %q", label, c.field, c.got, c.want)
			}
		}
	}
	if store.saves != 1 {
		t.Fatalf("saves = %d, want exactly 1", store.saves)
	}
}

func TestUpdateReplacesPassword(t *testing.T) {
	svc, _, store := seededForUpdate(t)
	pw, err := domain.NewPassword("brand new pw")
	if err != nil {
		t.Fatalf("NewPassword: %v", err)
	}

	e, err := svc.Update(t.Context(), app.NewMasterPassword([]byte("master")), mustName(t, "GITHUB"),
		app.EntryChanges{Password: &pw})

	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := e.Password().Reveal(); got != "brand new pw" {
		t.Fatalf("password = %q, want the new one", got)
	}
	if e.Username() != "alice" || e.Name().String() != "GitHub" {
		t.Fatalf("entry = %q/%q, want the other fields unchanged", e.Name(), e.Username())
	}
	if store.saves != 1 {
		t.Fatalf("saves = %d, want exactly 1", store.saves)
	}
}

func TestFailedUpdatesNeverSave(t *testing.T) {
	bob := "bob"
	longURL := "https://example.com/" + strings.Repeat("a", 2048)
	tests := []struct {
		name    string
		entry   string
		changes app.EntryChanges
		want    error
	}{
		{name: "no changes", entry: "github", changes: app.EntryChanges{}, want: app.ErrNoChanges},
		{name: "entry not found", entry: "nope", changes: app.EntryChanges{Username: &bob}, want: domain.ErrNotFound},
		{name: "URL over 2048 bytes", entry: "github", changes: app.EntryChanges{URL: &longURL}, want: domain.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, store := seededForUpdate(t)
			mp := app.NewMasterPassword([]byte("master"))

			_, err := svc.Update(t.Context(), mp, mustName(t, tt.entry), tt.changes)

			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if store.saves != 0 {
				t.Fatalf("saves = %d, want 0", store.saves)
			}
			stored, err := svc.Get(t.Context(), mp, mustName(t, "github"))
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if stored.URL() != "https://github.com" || stored.Username() != "alice" {
				t.Fatalf("stored entry changed: url %q, username %q", stored.URL(), stored.Username())
			}
		})
	}
}

func TestEntryChangesIsEmpty(t *testing.T) {
	empty := ""
	if !(app.EntryChanges{}).IsEmpty() {
		t.Error("zero EntryChanges is not empty")
	}
	// A pointer to "" is a request to clear the field, so it is a change.
	if (app.EntryChanges{URL: &empty}).IsEmpty() {
		t.Error("EntryChanges clearing the URL reports empty")
	}
}
