package cli_test

import (
	"strings"
	"testing"
)

const (
	deletedGithub   = "Deleted \"github\"\n"
	confirmQuestion = "Delete entry \"github\"? [y/N] "
)

// assertGithubGone checks through get that the github entry no longer exists.
func assertGithubGone(t *testing.T, path string) {
	t.Helper()
	get := newVaultHarness(t, masterPassword)
	code := get.run(t, "--vault", path, "get", "github")
	assertFailure(t, get, code, `entry "github" not found`)
}

func TestDeleteWithYesSkipsConfirmation(t *testing.T) {
	path := vaultWithGithub(t)
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "delete", "github", "--yes")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if got := h.stdout.String(); got != deletedGithub {
		t.Fatalf("stdout = %q, want %q", got, deletedGithub)
	}
	if h.stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty (no confirmation question)", h.stderr.String())
	}
	assertGithubGone(t, path)
}

func TestDeleteInteractiveConfirmationReadsStreamsIn(t *testing.T) {
	path := vaultWithGithub(t)
	h := newVaultHarness(t, masterPassword).withStdin("y\n")

	code := h.run(t, "--vault", path, "delete", "github")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if got := h.stdout.String(); got != deletedGithub {
		t.Fatalf("stdout = %q, want %q (the question must not go to stdout)", got, deletedGithub)
	}
	if got := h.stderr.String(); got != confirmQuestion {
		t.Fatalf("stderr = %q, want %q", got, confirmQuestion)
	}
	if got := strings.Join(h.prompter.prompts, "|"); got != "Master password: " {
		t.Fatalf("prompts = %q, want only the master password prompt", got)
	}
	assertGithubGone(t, path)
}

func TestDeletePasswordStdinConfirmationIsTheNextLine(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
	}{
		{name: "YES", stdin: masterPassword + "\nYES\n"},
		{name: "padded y with CRLF", stdin: masterPassword + "\r\n  y \r\n"},
		{name: "yes without newline", stdin: masterPassword + "\nyes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := vaultWithGithub(t)
			h := newVaultHarness(t).withStdin(tt.stdin)

			code := h.run(t, "--password-stdin", "--vault", path, "delete", "github")

			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
			}
			if got := h.stdout.String(); got != deletedGithub {
				t.Fatalf("stdout = %q, want %q", got, deletedGithub)
			}
			assertGithubGone(t, path)
		})
	}
}

func TestDeleteAbortsUnlessConfirmed(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
	}{
		{name: "n", stdin: masterPassword + "\nn\n"},
		{name: "empty line", stdin: masterPassword + "\n\n"},
		{name: "EOF", stdin: masterPassword + "\n"},
		{name: "other word", stdin: masterPassword + "\nyesplease\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := vaultWithGithub(t)
			before := readFile(t, path)
			h := newVaultHarness(t).withStdin(tt.stdin)

			code := h.run(t, "--password-stdin", "--vault", path, "delete", "github")

			if code != 1 {
				t.Fatalf("exit = %d, want 1 (stderr %q)", code, h.stderr.String())
			}
			if h.stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", h.stdout.String())
			}
			stderr := h.stderr.String()
			if !strings.HasPrefix(stderr, confirmQuestion) || !strings.Contains(stderr, "save_pwd: deletion aborted\n") {
				t.Fatalf("stderr = %q, want the question then the aborted message", stderr)
			}
			if strings.Contains(stderr, helpHint) {
				t.Fatalf("stderr = %q, an abort must not show the usage hint", stderr)
			}
			assertUnchanged(t, path, before)
		})
	}
}

func TestDeleteMissingEntryNeverAsks(t *testing.T) {
	path := vaultWithGithub(t)
	before := readFile(t, path)
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "delete", "nope", "--yes")

	assertFailure(t, h, code, `entry "nope" not found`)
	assertUnchanged(t, path, before)
}

func TestDeleteMissingEntryFailsBeforeTheQuestion(t *testing.T) {
	path := vaultWithGithub(t)
	before := readFile(t, path)
	h := newVaultHarness(t, masterPassword).withStdin("y\n")

	code := h.run(t, "--vault", path, "delete", "nope")

	assertFailure(t, h, code, `entry "nope" not found`)
	if strings.Contains(h.stderr.String(), "[y/N]") {
		t.Fatalf("stderr = %q, want no confirmation question", h.stderr.String())
	}
	assertUnchanged(t, path, before)
}

func TestDeleteIsCaseInsensitiveAndKeepsOtherEntries(t *testing.T) {
	path := vaultWithGithub(t)
	add := newVaultHarness(t).withStdin(masterPassword + "\ngitlab-pw\n")
	if code := add.run(t, "--password-stdin", "--vault", path, "add", "gitlab"); code != 0 {
		t.Fatalf("add gitlab exit = %d (stderr %q)", code, add.stderr.String())
	}
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "delete", "GITHUB", "--yes")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if got := h.stdout.String(); got != deletedGithub {
		t.Fatalf("stdout = %q, want the stored name in %q", got, deletedGithub)
	}
	list := newVaultHarness(t, masterPassword)
	if code := list.run(t, "--vault", path, "list"); code != 0 {
		t.Fatalf("list exit = %d (stderr %q)", code, list.stderr.String())
	}
	out := list.stdout.String()
	if !strings.Contains(out, "gitlab") || strings.Contains(out, "github") {
		t.Fatalf("list stdout = %q, want gitlab without github", out)
	}
}
