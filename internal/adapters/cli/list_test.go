package cli_test

import (
	"os"
	"strings"
	"testing"
)

// listEntries are the ticket's three entries, added out of name order and
// each with its own password so a leak names the entry it came from.
var listEntries = []struct {
	args     []string
	password string
}{
	{args: []string{"add", "zeta", "--kind", "note"}, password: "pw-zeta-Q7v!"},
	{args: []string{"add", "Alpha", "--kind", "site", "--username", "alice", "--expires", "2027-01-31"}, password: "pw-alpha-K2m#"},
	{args: []string{"add", "beta", "--kind", "card", "--username", "bob"}, password: "pw-beta-X9t$"},
}

// vaultForListing returns a vault holding listEntries.
func vaultForListing(t *testing.T) string {
	t.Helper()
	path := newVault(t)
	for _, e := range listEntries {
		h := newVaultHarness(t).withStdin(masterPassword + "\n" + e.password + "\n")
		args := append([]string{"--password-stdin", "--vault", path}, e.args...)
		if code := h.run(t, args...); code != 0 {
			t.Fatalf("%v exit = %d (stderr %q)", e.args, code, h.stderr.String())
		}
	}
	return path
}

func TestListShowsTableSortedByNameWithoutPasswords(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		golden string
	}{
		{name: "all kinds", args: []string{"list"}, golden: "list_all.golden"},
		{name: "card only", args: []string{"list", "--kind", "card"}, golden: "list_card.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := vaultForListing(t)
			h := newVaultHarness(t, masterPassword)

			code := h.run(t, append([]string{"--vault", path}, tt.args...)...)

			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
			}
			assertGolden(t, tt.golden, h.stdout.String())
			if got := h.stderr.String(); got != "" {
				t.Fatalf("stderr = %q, want empty", got)
			}
			for _, e := range listEntries {
				for stream, out := range map[string]string{"stdout": h.stdout.String(), "stderr": h.stderr.String()} {
					if strings.Contains(out, e.password) {
						t.Errorf("%s contains the password of %v", stream, e.args)
					}
				}
			}
		})
	}
}

func TestListWithNothingToShowReportsNoEntriesOnStderr(t *testing.T) {
	tests := []struct {
		name  string
		vault func(t *testing.T) string
		args  []string
	}{
		{name: "empty vault", vault: newVault, args: []string{"list"}},
		{name: "empty vault filtered", vault: newVault, args: []string{"list", "--kind", "site"}},
		{name: "filter without matches", vault: vaultForListing, args: []string{"list", "--kind", "other"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.vault(t)
			h := newVaultHarness(t, masterPassword)

			code := h.run(t, append([]string{"--vault", path}, tt.args...)...)

			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
			}
			if got := h.stdout.String(); got != "" {
				t.Fatalf("stdout = %q, want empty", got)
			}
			if got, want := h.stderr.String(), "No entries.\n"; got != want {
				t.Fatalf("stderr = %q, want %q", got, want)
			}
		})
	}
}

func TestListDoesNotModifyVaultFile(t *testing.T) {
	path := vaultForListing(t)
	before := readFile(t, path)
	infoBefore, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "list")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	assertUnchanged(t, path, before)
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("modification time changed from %v to %v", infoBefore.ModTime(), infoAfter.ModTime())
	}
}
