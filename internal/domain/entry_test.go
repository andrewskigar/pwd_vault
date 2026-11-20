package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"save_pwd/internal/domain"
)

func TestNewName(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantDisplay string
		wantKey     string
		wantErr     bool
	}{
		{name: "keeps display case", in: "GitHub", wantDisplay: "GitHub", wantKey: "github"},
		{name: "trims surrounding space", in: "  bank  ", wantDisplay: "bank", wantKey: "bank"},
		{name: "folds unicode case", in: "ÉCOLE", wantDisplay: "ÉCOLE", wantKey: "école"},
		{name: "128 runes is the maximum", in: strings.Repeat("é", 128), wantDisplay: strings.Repeat("é", 128), wantKey: strings.Repeat("é", 128)},
		{name: "empty", in: "", wantErr: true},
		{name: "only spaces", in: "   ", wantErr: true},
		{name: "129 runes", in: strings.Repeat("a", 129), wantErr: true},
		{name: "control character", in: "a\tb", wantErr: true},
		{name: "DEL character", in: "a\x7fb", wantErr: true},
		{name: "invalid UTF-8", in: "a\xffb", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := domain.NewName(tt.in)

			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalid) {
					t.Fatalf("NewName(%q) error = %v, want ErrInvalid", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewName(%q): %v", tt.in, err)
			}
			if n.String() != tt.wantDisplay || n.Key() != tt.wantKey {
				t.Fatalf("NewName(%q) = %q key %q, want %q key %q", tt.in, n.String(), n.Key(), tt.wantDisplay, tt.wantKey)
			}
		})
	}
}

func TestParseKind(t *testing.T) {
	for _, s := range []string{"site", "card", "note", "other"} {
		if k, err := domain.ParseKind(s); err != nil || string(k) != s {
			t.Errorf("ParseKind(%q) = %q, %v", s, k, err)
		}
	}
	for _, s := range []string{"", "Site", "bogus"} {
		if _, err := domain.ParseKind(s); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("ParseKind(%q) error = %v, want ErrInvalid", s, err)
		}
	}
}

func TestParseDate(t *testing.T) {
	d, err := domain.ParseDate("2027-01-31")
	if err != nil || d.String() != "2027-01-31" || d.IsZero() {
		t.Fatalf("ParseDate(2027-01-31) = %q, %v", d, err)
	}
	empty, err := domain.ParseDate("")
	if err != nil || !empty.IsZero() || empty.String() != "" {
		t.Fatalf("ParseDate(\"\") = %q, %v, want the zero date", empty, err)
	}
	for _, s := range []string{"2027-13-01", "2027-02-30", "2027-1-31", "31/01/2027", "2027-01-31T00:00:00Z"} {
		if _, err := domain.ParseDate(s); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("ParseDate(%q) error = %v, want ErrInvalid", s, err)
		}
	}
}

func validFields(t *testing.T) domain.EntryFields {
	t.Helper()
	n, err := domain.NewName("github")
	if err != nil {
		t.Fatal(err)
	}
	pw, err := domain.NewPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	return domain.EntryFields{Name: n, Kind: domain.KindSite, Password: pw, CreatedAt: at, UpdatedAt: at}
}

func TestNewEntryInvariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(f *domain.EntryFields)
	}{
		{name: "zero name", mutate: func(f *domain.EntryFields) { f.Name = domain.Name{} }},
		{name: "unknown kind", mutate: func(f *domain.EntryFields) { f.Kind = "bogus" }},
		{name: "zero password", mutate: func(f *domain.EntryFields) { f.Password = domain.Password{} }},
		{name: "username over 256 bytes", mutate: func(f *domain.EntryFields) { f.Username = strings.Repeat("u", 257) }},
		{name: "URL over 2048 bytes", mutate: func(f *domain.EntryFields) { f.URL = strings.Repeat("u", 2049) }},
		{name: "notes over 8192 bytes", mutate: func(f *domain.EntryFields) { f.Notes = strings.Repeat("n", 8193) }},
		{name: "zero created", mutate: func(f *domain.EntryFields) { f.CreatedAt = time.Time{} }},
		{name: "zero updated", mutate: func(f *domain.EntryFields) { f.UpdatedAt = time.Time{} }},
		{name: "updated before created", mutate: func(f *domain.EntryFields) { f.UpdatedAt = f.CreatedAt.Add(-time.Second) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validFields(t)
			tt.mutate(&f)

			if _, err := domain.NewEntry(f); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("NewEntry error = %v, want ErrInvalid", err)
			}
		})
	}

	t.Run("limits are inclusive", func(t *testing.T) {
		f := validFields(t)
		f.Username, f.URL, f.Notes = strings.Repeat("u", 256), strings.Repeat("u", 2048), strings.Repeat("n", 8192)
		if _, err := domain.NewEntry(f); err != nil {
			t.Fatalf("NewEntry: %v", err)
		}
	})
}

func TestVaultAddAndGetAreCaseInsensitive(t *testing.T) {
	first := validFields(t)
	e, err := domain.NewEntry(first)
	if err != nil {
		t.Fatal(err)
	}
	v := domain.NewVault()
	if err := v.Add(e); err != nil {
		t.Fatalf("Add: %v", err)
	}
	upper, _ := domain.NewName("GITHUB")
	dup := first
	dup.Name = upper
	dupEntry, err := domain.NewEntry(dup)
	if err != nil {
		t.Fatal(err)
	}

	if err := v.Add(dupEntry); !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("Add(GITHUB) error = %v, want ErrDuplicate", err)
	}
	got, err := v.Get(upper)
	if err != nil || got.Name().String() != "github" {
		t.Fatalf("Get(GITHUB) = %q, %v, want github", got.Name(), err)
	}
	missing, _ := domain.NewName("nope")
	if _, err := v.Get(missing); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get(nope) error = %v, want ErrNotFound", err)
	}
	if v.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", v.Len())
	}
}

func TestZeroVaultAcceptsAdd(t *testing.T) {
	e, err := domain.NewEntry(validFields(t))
	if err != nil {
		t.Fatal(err)
	}
	var v domain.Vault

	if err := v.Add(e); err != nil {
		t.Fatalf("Add to zero Vault: %v", err)
	}
	if v.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", v.Len())
	}
}

func TestVaultEntriesAreSortedByKey(t *testing.T) {
	v := domain.NewVault()
	for _, s := range []string{"bank", "Zeta", "alpha"} {
		f := validFields(t)
		f.Name, _ = domain.NewName(s)
		e, err := domain.NewEntry(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := v.Add(e); err != nil {
			t.Fatal(err)
		}
	}

	var got []string
	for _, e := range v.Entries() {
		got = append(got, e.Name().String())
	}

	if strings.Join(got, ",") != "alpha,bank,Zeta" {
		t.Fatalf("Entries() names = %v, want alpha,bank,Zeta", got)
	}
}
