package app_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

func TestOpenLoadsOnceAndFindNeverTouchesTheStore(t *testing.T) {
	svc, _, store := seededForUpdate(t)

	sess, err := svc.Open(t.Context(), app.NewMasterPassword([]byte("master")))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if store.loads != 1 {
		t.Fatalf("loads after Open = %d, want 1", store.loads)
	}

	found, findErr := sess.Find(mustName(t, "GITHUB"))
	_, missingErr := sess.Find(mustName(t, "nope"))

	if findErr != nil || found.Name().String() != "GitHub" || found.Username() != "alice" {
		t.Errorf("Find(GITHUB) = %q (username %q), %v, want the stored GitHub entry", found.Name(), found.Username(), findErr)
	}
	if !errors.Is(missingErr, domain.ErrNotFound) {
		t.Errorf("Find(nope) error = %v, want domain.ErrNotFound", missingErr)
	}
	if store.loads != 1 || store.saves != 0 {
		t.Fatalf("loads, saves after Find = %d, %d, want 1, 0", store.loads, store.saves)
	}
}

func TestOpenFailureIsTheWrappedLoadError(t *testing.T) {
	for _, loadErr := range []error{app.ErrUnlock, app.ErrVaultNotFound} {
		t.Run(loadErr.Error(), func(t *testing.T) {
			store := &memStore{loadErr: loadErr}
			svc := app.NewService(store, fixedClock)

			sess, err := svc.Open(t.Context(), app.NewMasterPassword([]byte("master")))

			if !errors.Is(err, loadErr) {
				t.Fatalf("Open error = %v, want %v", err, loadErr)
			}
			if want := "load vault: " + loadErr.Error(); err.Error() != want {
				t.Fatalf("Open error text = %q, want %q", err.Error(), want)
			}
			if sess != nil {
				t.Fatal("Open returned a session along with the error")
			}
			if store.loads != 1 || store.saves != 0 {
				t.Fatalf("loads, saves = %d, %d, want 1, 0", store.loads, store.saves)
			}
		})
	}
}

func TestSessionAddSavesOnceWithoutReloading(t *testing.T) {
	svc, _, store := seededForUpdate(t)
	mp := app.NewMasterPassword([]byte("master"))
	sess, err := svc.Open(t.Context(), mp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	e, err := sess.Add(t.Context(), draft(t, "gitlab"))

	if err != nil {
		t.Fatalf("Session.Add: %v", err)
	}
	if e.Name().String() != "gitlab" || !e.CreatedAt().Equal(e.UpdatedAt()) || e.CreatedAt().IsZero() {
		t.Errorf("added entry %q has CreatedAt %v, UpdatedAt %v, want gitlab stamped now", e.Name(), e.CreatedAt(), e.UpdatedAt())
	}
	if store.loads != 1 || store.saves != 1 {
		t.Fatalf("loads, saves = %d, %d, want 1, 1", store.loads, store.saves)
	}
	if _, err := svc.Get(t.Context(), mp, mustName(t, "GitLab")); err != nil {
		t.Fatalf("Get gitlab after Session.Add: %v", err)
	}
}

func TestFailedSessionAddNeverSaves(t *testing.T) {
	invalid := draft(t, "gitlab")
	invalid.URL = "https://example.com/" + strings.Repeat("a", 2048)
	tests := []struct {
		name  string
		draft app.EntryDraft
		want  error
	}{
		{name: "duplicate name ignoring case", draft: draft(t, "GITHUB"), want: domain.ErrDuplicate},
		{name: "URL over 2048 bytes", draft: invalid, want: domain.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, store := seededForUpdate(t)
			sess, err := svc.Open(t.Context(), app.NewMasterPassword([]byte("master")))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}

			_, err = sess.Add(t.Context(), tt.draft)

			if !errors.Is(err, tt.want) {
				t.Fatalf("Session.Add error = %v, want %v", err, tt.want)
			}
			if store.loads != 1 || store.saves != 0 {
				t.Fatalf("loads, saves = %d, %d, want 1, 0", store.loads, store.saves)
			}
			if got := store.vault.Len(); got != 1 {
				t.Fatalf("vault holds %d entries, want only GitHub", got)
			}
		})
	}
}

func TestSessionDeleteSavesOnceWithoutReloading(t *testing.T) {
	svc, _, store := seededForUpdate(t)
	mp := app.NewMasterPassword([]byte("master"))
	sess, err := svc.Open(t.Context(), mp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := sess.Delete(t.Context(), mustName(t, "github")); err != nil {
		t.Fatalf("Session.Delete: %v", err)
	}

	if store.loads != 1 || store.saves != 1 {
		t.Fatalf("loads, saves = %d, %d, want 1, 1", store.loads, store.saves)
	}
	if _, err := svc.Get(t.Context(), mp, mustName(t, "GitHub")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get after Session.Delete error = %v, want domain.ErrNotFound", err)
	}
}

func TestSessionDeleteMissingEntryNeverSaves(t *testing.T) {
	svc, _, store := seededForUpdate(t)
	sess, err := svc.Open(t.Context(), app.NewMasterPassword([]byte("master")))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	err = sess.Delete(t.Context(), mustName(t, "nope"))

	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Session.Delete error = %v, want domain.ErrNotFound", err)
	}
	if store.loads != 1 || store.saves != 0 {
		t.Fatalf("loads, saves = %d, %d, want 1, 0", store.loads, store.saves)
	}
	if got := store.vault.Len(); got != 1 {
		t.Fatalf("vault holds %d entries, want GitHub kept", got)
	}
}

func TestServiceAddAndDeleteEachLoadOnceAndSaveOnce(t *testing.T) {
	svc, _, store := seededForUpdate(t)
	mp := app.NewMasterPassword([]byte("master"))

	if _, err := svc.Add(t.Context(), mp, draft(t, "gitlab")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if store.loads != 1 || store.saves != 1 {
		t.Fatalf("Add: loads, saves = %d, %d, want 1, 1", store.loads, store.saves)
	}
	store.loads, store.saves = 0, 0

	if err := svc.Delete(t.Context(), mp, mustName(t, "gitlab")); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.loads != 1 || store.saves != 1 {
		t.Fatalf("Delete: loads, saves = %d, %d, want 1, 1", store.loads, store.saves)
	}
}

func fixedClock() time.Time { return time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC) }
