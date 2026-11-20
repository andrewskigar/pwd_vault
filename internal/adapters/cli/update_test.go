package cli_test

import (
	"strings"
	"testing"
	"time"
)

const updatedGithub = "Updated \"github\"\n"

// githubShown is `get github --show` for the user story 10 entry, with the
// lines a test expects to differ substituted in.
func githubShown(replace map[string]string) string {
	lines := []string{
		"Name:     github",
		"Kind:     site",
		"Username: alice",
		"Password: " + entryPassword,
		"URL:      https://github.com",
		"Notes:    2FA on",
		"Created:  2026-11-20T10:00:00Z",
		"Updated:  2026-11-20T10:00:00Z",
		"Expires:  2027-01-31",
	}
	for i, l := range lines {
		label, _, _ := strings.Cut(l, ":")
		if v, ok := replace[label]; ok {
			lines[i] = v
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func showGithub(t *testing.T, path string) string {
	t.Helper()
	get := newVaultHarness(t, masterPassword)
	if code := get.run(t, "--vault", path, "get", "github", "--show"); code != 0 {
		t.Fatalf("get exit = %d (stderr %q)", code, get.stderr.String())
	}
	return get.stdout.String()
}

func TestUpdateChangesGivenFieldsAndBumpsUpdatedTime(t *testing.T) {
	path := vaultWithGithub(t)
	h := newVaultHarness(t, masterPassword) // an entry password prompt fails the test
	h.deps.Clock = func() time.Time { return time.Date(2026, 11, 21, 9, 30, 0, 0, time.UTC) }

	code := h.run(t, "--vault", path, "update", "github", "--username", "bob", "--expires", "")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if got := h.stdout.String(); got != updatedGithub {
		t.Fatalf("stdout = %q, want %q", got, updatedGithub)
	}
	if h.stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", h.stderr.String())
	}
	want := githubShown(map[string]string{
		"Username": "Username: bob",
		"Updated":  "Updated:  2026-11-21T09:30:00Z",
		"Expires":  "Expires:  ",
	})
	if got := showGithub(t, path); got != want {
		t.Fatalf("get --show =\n%s\nwant\n%s", got, want)
	}
}

func TestUpdateChangePasswordPrompted(t *testing.T) {
	path := vaultWithGithub(t)
	h := newVaultHarness(t, masterPassword, "new-entry-pw", "new-entry-pw")

	code := h.run(t, "--vault", path, "update", "github", "--change-password")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if got := h.stdout.String(); got != updatedGithub {
		t.Fatalf("stdout = %q, want %q", got, updatedGithub)
	}
	wantPrompts := "Master password: |Entry password: |Repeat entry password: "
	if got := strings.Join(h.prompter.prompts, "|"); got != wantPrompts {
		t.Fatalf("prompts = %q, want %q", got, wantPrompts)
	}
	want := githubShown(map[string]string{"Password": "Password: new-entry-pw"})
	if got := showGithub(t, path); got != want {
		t.Fatalf("get --show =\n%s\nwant\n%s", got, want)
	}
}

func TestUpdateChangePasswordFromStdinSecondLine(t *testing.T) {
	path := vaultWithGithub(t)
	h := newVaultHarness(t).withStdin(masterPassword + "\nnewpw\n")

	code := h.run(t, "--password-stdin", "--vault", path, "update", "github", "--change-password")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	want := githubShown(map[string]string{"Password": "Password: newpw"})
	if got := showGithub(t, path); got != want {
		t.Fatalf("get --show =\n%s\nwant\n%s", got, want)
	}
}

func TestUpdateGenerateStoresNewPasswordWithoutPrintingIt(t *testing.T) {
	path := vaultWithGithub(t)
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "update", "github", "--generate", "--length", "30")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	wantOut := updatedGithub + "(generated password; view it with: save_pwd get github --show)\n"
	if got := h.stdout.String(); got != wantOut {
		t.Fatalf("stdout = %q, want %q", got, wantOut)
	}
	if h.stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", h.stderr.String())
	}
	shown := showGithub(t, path)
	var pw string
	for _, line := range strings.Split(shown, "\n") {
		if v, ok := strings.CutPrefix(line, "Password: "); ok {
			pw = v
		}
	}
	if len(pw) != 30 {
		t.Fatalf("stored password %q has length %d, want 30", pw, len(pw))
	}
	assertOnlyFrom(t, pw, lowerSet+upperSet+digitSet+symbolSet)
	if want := githubShown(map[string]string{"Password": "Password: " + pw}); shown != want {
		t.Fatalf("get --show =\n%s\nwant only the password changed:\n%s", shown, want)
	}
	if strings.Contains(h.stdout.String(), pw) {
		t.Fatal("update printed the generated password")
	}
}

func TestUpdateUsageErrorsBeforeAnyPrompt(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no change flags", args: nil, want: "no changes requested"},
		{name: "change-password with generate", args: []string{"--change-password", "--generate"}, want: "--change-password"},
		{name: "length without generate", args: []string{"--length", "30"}, want: "--length requires --generate"},
		{name: "unknown kind", args: []string{"--kind", "bogus"}, want: "kind"},
		{name: "empty kind", args: []string{"--kind", ""}, want: "kind"},
		{name: "impossible expiry", args: []string{"--expires", "2027-13-01"}, want: "YYYY-MM-DD"},
		{name: "password flag", args: []string{"--password", "hunter2"}, want: "unknown flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := vaultWithGithub(t)
			before := readFile(t, path)
			h := newVaultHarness(t) // any prompt fails the test

			code := h.run(t, append([]string{"--vault", path, "update", "github"}, tt.args...)...)

			if code != 2 {
				t.Fatalf("exit = %d, want 2 (stderr %q)", code, h.stderr.String())
			}
			stderr := h.stderr.String()
			if !strings.Contains(stderr, tt.want) || !strings.Contains(stderr, helpHint) {
				t.Fatalf("stderr = %q, want %q and the usage hint", stderr, tt.want)
			}
			assertUnchanged(t, path, before)
		})
	}
}

func TestUpdateMissingEntryLeavesVaultUnchanged(t *testing.T) {
	path := vaultWithGithub(t)
	before := readFile(t, path)
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "update", "nope", "--username", "x")

	assertFailure(t, h, code, `entry "nope" not found`)
	assertUnchanged(t, path, before)
}

func TestUpdateMatchesNameCaseInsensitivelyAndKeepsStoredCase(t *testing.T) {
	path := vaultWithGithub(t)
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "update", "GITHUB", "--url", "https://github.com/bob")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if got := h.stdout.String(); got != updatedGithub {
		t.Fatalf("stdout = %q, want %q", got, updatedGithub)
	}
	want := githubShown(map[string]string{"URL": "URL:      https://github.com/bob"})
	if got := showGithub(t, path); got != want {
		t.Fatalf("get --show =\n%s\nwant\n%s", got, want)
	}
}
