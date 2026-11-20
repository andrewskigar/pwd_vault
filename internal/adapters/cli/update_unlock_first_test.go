package cli_test

import (
	"io"
	"path/filepath"
	"testing"
)

// TestUpdateNewPasswordFailsAfterOnlyTheMasterPassword proves update unlocks
// the vault and finds the entry before it asks for or generates the new entry
// password: each failure costs only the master password, no entry prompt, no
// second stdin line and no randomness.
func TestUpdateNewPasswordFailsAfterOnlyTheMasterPassword(t *testing.T) {
	tests := []struct {
		name    string
		vault   string // "github", or "missing" for no vault file
		secrets []string
		stdin   string
		args    []string
		prompts []string
		random  io.Reader
		stderr  string // ignored for a missing vault: the message names the temp path
	}{
		{
			name: "change-password, wrong master", vault: "github",
			secrets: []string{"wrong master pw"},
			args:    []string{"update", "github", "--change-password"},
			prompts: []string{"Master password: "},
			stderr:  unlockLine,
		},
		{
			name: "change-password --password-stdin, wrong master", vault: "github",
			// Only the master line: reading the entry password first would
			// fail with "no password provided on stdin" instead.
			stdin:  "wrong\n",
			args:   []string{"--password-stdin", "update", "github", "--change-password"},
			stderr: unlockLine,
		},
		{
			name: "change-password, missing entry", vault: "github",
			secrets: []string{masterPassword},
			args:    []string{"update", "nope", "--change-password"},
			prompts: []string{"Master password: "},
			stderr:  "save_pwd: entry \"nope\" not found\n",
		},
		{
			name: "change-password --password-stdin, missing entry", vault: "github",
			stdin:  masterPassword + "\n",
			args:   []string{"--password-stdin", "update", "nope", "--change-password"},
			stderr: "save_pwd: entry \"nope\" not found\n",
		},
		{
			name: "change-password, missing vault", vault: "missing",
			secrets: []string{masterPassword},
			args:    []string{"update", "github", "--change-password"},
			prompts: []string{"Master password: "},
		},
		{
			name: "change-password --password-stdin, missing vault", vault: "missing",
			stdin: masterPassword + "\n",
			args:  []string{"--password-stdin", "update", "github", "--change-password"},
		},
		{
			name: "generate, wrong master", vault: "github",
			secrets: []string{"wrong master pw"},
			args:    []string{"update", "github", "--generate"},
			prompts: []string{"Master password: "},
			random:  failingReader{},
			stderr:  unlockLine,
		},
		{
			name: "generate, missing entry", vault: "github",
			secrets: []string{masterPassword},
			args:    []string{"update", "nope", "--generate"},
			prompts: []string{"Master password: "},
			random:  failingReader{},
			stderr:  "save_pwd: entry \"nope\" not found\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "missing.db")
			var before []byte
			if tt.vault == "github" {
				path = vaultWithGithub(t)
				before = readFile(t, path)
			}
			h := newVaultHarness(t, tt.secrets...).withStdin(tt.stdin)
			if tt.random != nil {
				h.deps.Random = tt.random
			}
			calls := h.countStoreCalls()

			code := h.run(t, append([]string{"--vault", path}, tt.args...)...)

			if code != 1 {
				t.Fatalf("exit = %d, want 1 (stderr %q)", code, h.stderr.String())
			}
			want := tt.stderr
			if tt.vault == "missing" {
				want = "save_pwd: no vault at " + path + "; run \"save_pwd init\" first\n"
			}
			if got := h.stderr.String(); got != want {
				t.Fatalf("stderr = %q, want only %q", got, want)
			}
			if h.stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty (no Updated line, no hint)", h.stdout.String())
			}
			assertPrompts(t, h, tt.prompts...)
			assertStoreCalls(t, calls, 1, 0)
			if tt.vault == "github" {
				assertUnchanged(t, path, before)
			} else {
				assertNoFile(t, path)
			}
			assertPromptedSecretsWiped(t, h)
		})
	}
}

func TestUpdateNewPasswordLoadsOnceAndSavesOnce(t *testing.T) {
	tests := []struct {
		name    string
		secrets []string
		stdin   string
		args    []string
		prompts []string
	}{
		{
			name:    "change-password prompted",
			secrets: []string{masterPassword, "new-entry-pw", "new-entry-pw"},
			args:    []string{"update", "github", "--change-password"},
			prompts: []string{"Master password: ", "Entry password: ", "Repeat entry password: "},
		},
		{
			name:  "change-password --password-stdin",
			stdin: masterPassword + "\nnew-entry-pw\n",
			args:  []string{"--password-stdin", "update", "github", "--change-password"},
		},
		{
			name:    "generate",
			secrets: []string{masterPassword},
			args:    []string{"update", "github", "--generate"},
			prompts: []string{"Master password: "},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := vaultWithGithub(t)
			h := newVaultHarness(t, tt.secrets...).withStdin(tt.stdin)
			calls := h.countStoreCalls()

			code := h.run(t, append([]string{"--vault", path}, tt.args...)...)

			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
			}
			assertPrompts(t, h, tt.prompts...)
			assertStoreCalls(t, calls, 1, 1)
		})
	}
}
