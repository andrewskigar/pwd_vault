package cli_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata")

const (
	entryPassword = "s3cret-entry-pw"
	unlockLine    = "save_pwd: cannot unlock vault: wrong master password or corrupted file\n"
)

// githubAdd is the spec's user story 10 invocation, minus the vault flag.
var githubAdd = []string{
	"add", "github", "--kind", "site", "--username", "alice",
	"--url", "https://github.com", "--notes", "2FA on", "--expires", "2027-01-31",
}

// newVault runs init through the CLI and returns the vault path.
func newVault(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v.db")
	h := newVaultHarness(t, masterPassword, masterPassword)
	if code := h.run(t, "--vault", path, "init"); code != 0 {
		t.Fatalf("init exit = %d (stderr %q)", code, h.stderr.String())
	}
	return path
}

// vaultWithGithub returns a vault holding the user story 10 entry.
func vaultWithGithub(t *testing.T) string {
	t.Helper()
	path := newVault(t)
	h := newVaultHarness(t, masterPassword, entryPassword, entryPassword)
	if code := h.run(t, append([]string{"--vault", path}, githubAdd...)...); code != 0 {
		t.Fatalf("add exit = %d (stderr %q)", code, h.stderr.String())
	}
	return path
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func assertUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	if !bytes.Equal(readFile(t, path), before) {
		t.Fatal("the vault file changed")
	}
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
	}
	// Git may check golden files out with CRLF on Windows.
	want := strings.ReplaceAll(string(readFile(t, path)), "\r\n", "\n")
	if got != want {
		t.Fatalf("output mismatch with %s\ngot:\n%s\nwant:\n%s", path, got, want)
	}
}

func TestAddThenGetShowsMaskedEntry(t *testing.T) {
	path := newVault(t)
	add := newVaultHarness(t, masterPassword, entryPassword, entryPassword)

	code := add.run(t, append([]string{"--vault", path}, githubAdd...)...)

	if code != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr %q)", code, add.stderr.String())
	}
	if got, want := add.stdout.String(), "Added \"github\"\n"; got != want {
		t.Fatalf("add stdout = %q, want %q", got, want)
	}
	wantPrompts := "Master password: |Entry password: |Repeat entry password: "
	if got := strings.Join(add.prompter.prompts, "|"); got != wantPrompts {
		t.Fatalf("prompts = %q, want %q", got, wantPrompts)
	}
	for i, b := range add.prompter.replies {
		if !bytes.Equal(b, make([]byte, len(b))) {
			t.Fatalf("secret %d was not wiped: %q", i, b)
		}
	}

	get := newVaultHarness(t, masterPassword)
	code = get.run(t, "--vault", path, "get", "github")

	if code != 0 {
		t.Fatalf("get exit = %d, want 0 (stderr %q)", code, get.stderr.String())
	}
	assertGolden(t, "get_masked.golden", get.stdout.String())
	if strings.Contains(get.stdout.String(), entryPassword) {
		t.Fatal("get without --show printed the password")
	}
}

func TestGetShowRevealsPasswordCaseInsensitively(t *testing.T) {
	path := vaultWithGithub(t)
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "get", "GITHUB", "--show")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	assertGolden(t, "get_shown.golden", h.stdout.String())
}

func TestAddDuplicateNameLeavesVaultUnchanged(t *testing.T) {
	path := vaultWithGithub(t)
	before := readFile(t, path)
	h := newVaultHarness(t, masterPassword, entryPassword, entryPassword)

	code := h.run(t, "--vault", path, "add", "GitHub")

	assertFailure(t, h, code, `entry "GitHub" already exists`)
	assertUnchanged(t, path, before)
}

func TestGetMissingEntryIsNotFound(t *testing.T) {
	path := newVault(t)
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "get", "nope")

	assertFailure(t, h, code, `entry "nope" not found`)
}

func TestWrongMasterPasswordIsGenericUnlockError(t *testing.T) {
	tests := []struct {
		name    string
		secrets []string
		args    []string
	}{
		{name: "get", secrets: []string{"wrong master pw"}, args: []string{"get", "github"}},
		{name: "add", secrets: []string{"wrong master pw", entryPassword, entryPassword}, args: []string{"add", "gitlab"}},
		{name: "list", secrets: []string{"wrong master pw"}, args: []string{"list"}},
		{name: "update", secrets: []string{"wrong master pw"}, args: []string{"update", "github", "--username", "bob"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := vaultWithGithub(t)
			before := readFile(t, path)
			h := newVaultHarness(t, tt.secrets...)

			code := h.run(t, append([]string{"--vault", path}, tt.args...)...)

			if code != 1 {
				t.Fatalf("exit = %d, want 1 (stderr %q)", code, h.stderr.String())
			}
			if got := h.stderr.String(); got != unlockLine {
				t.Fatalf("stderr = %q, want %q", got, unlockLine)
			}
			assertUnchanged(t, path, before)
		})
	}
}

func TestEntryCommandsAgainstMissingVault(t *testing.T) {
	tests := []struct {
		name    string
		secrets []string
		args    []string
	}{
		{name: "get", secrets: []string{masterPassword}, args: []string{"get", "github"}},
		{name: "add", secrets: []string{masterPassword, entryPassword, entryPassword}, args: []string{"add", "github"}},
		{name: "list", secrets: []string{masterPassword}, args: []string{"list"}},
		{name: "update", secrets: []string{masterPassword}, args: []string{"update", "github", "--username", "bob"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "missing.db")
			h := newVaultHarness(t, tt.secrets...)

			code := h.run(t, append([]string{"--vault", path}, tt.args...)...)

			assertFailure(t, h, code, `no vault at `+path+`; run "save_pwd init" first`)
			assertNoFile(t, path)
		})
	}
}

func TestInvalidEntryArgumentsAreUsageErrorsBeforeAnyPrompt(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "add empty name", args: []string{"add", ""}},
		{name: "add blank name", args: []string{"add", "   "}},
		{name: "add name over 128 runes", args: []string{"add", strings.Repeat("é", 129)}},
		{name: "add name with control character", args: []string{"add", "git\x07hub"}},
		{name: "add unknown kind", args: []string{"add", "github", "--kind", "bogus"}},
		{name: "add impossible expiry", args: []string{"add", "github", "--expires", "2027-13-01"}},
		{name: "add password flag", args: []string{"add", "github", "--password", "hunter2"}},
		{name: "add without name", args: []string{"add"}},
		{name: "get empty name", args: []string{"get", ""}},
		{name: "get name with control character", args: []string{"get", "git\nhub"}},
		{name: "list unknown kind", args: []string{"list", "--kind", "bogus"}},
		{name: "list extra argument", args: []string{"list", "extra-arg"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := newVault(t)
			before := readFile(t, path)
			h := newVaultHarness(t) // any prompt fails the test

			code := h.run(t, append([]string{"--vault", path}, tt.args...)...)

			if code != 2 {
				t.Fatalf("exit = %d, want 2 (stderr %q)", code, h.stderr.String())
			}
			if !strings.Contains(h.stderr.String(), helpHint) {
				t.Fatalf("stderr = %q, want the usage hint", h.stderr.String())
			}
			if strings.Contains(h.stderr.String(), "hunter2") {
				t.Fatalf("stderr %q echoes the secret flag value", h.stderr.String())
			}
			assertUnchanged(t, path, before)
		})
	}
}

func TestAddWithPasswordStdinReadsEntryPasswordFromSecondLine(t *testing.T) {
	path := newVault(t)
	h := newVaultHarness(t).withStdin(masterPassword + "\nentrypw\n")

	code := h.run(t, "--password-stdin", "--vault", path, "add", "github")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	get := newVaultHarness(t).withStdin(masterPassword + "\n")
	if code := get.run(t, "--password-stdin", "--vault", path, "get", "github", "--show"); code != 0 {
		t.Fatalf("get exit = %d (stderr %q)", code, get.stderr.String())
	}
	if !strings.Contains(get.stdout.String(), "Password: entrypw\n") {
		t.Fatalf("get stdout = %q, want the stdin entry password", get.stdout.String())
	}
}

func TestAddFailuresBeforeSavingLeaveVaultUnchanged(t *testing.T) {
	tests := []struct {
		name    string
		secrets []string
		stdin   *string
		want    string
	}{
		{name: "missing stdin entry password", stdin: ptr(masterPassword + "\n"), want: "no password provided on stdin"},
		{name: "mismatched entry confirmation", secrets: []string{masterPassword, entryPassword, entryPassword + "x"}, want: "entry passwords do not match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := newVault(t)
			before := readFile(t, path)
			h := newVaultHarness(t, tt.secrets...)
			args := []string{"--vault", path, "add", "github"}
			if tt.stdin != nil {
				h = h.withStdin(*tt.stdin)
				args = append([]string{"--password-stdin"}, args...)
			}

			code := h.run(t, args...)

			assertFailure(t, h, code, tt.want)
			assertUnchanged(t, path, before)
		})
	}
}

func TestAddedEntryIsNotPlaintextInVaultFile(t *testing.T) {
	path := vaultWithGithub(t)

	raw := readFile(t, path)

	for _, s := range []string{"github", "alice", "https://github.com", "2FA on", entryPassword} {
		if bytes.Contains(raw, []byte(s)) {
			t.Errorf("vault file contains %q in plaintext", s)
		}
	}
}
