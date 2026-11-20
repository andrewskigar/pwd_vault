package cli_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"save_pwd/internal/adapters/encfile"
	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

// countingStore is the real encfile store with every Load and Save counted, so
// tests can prove how many Argon2 derivations a command costs.
type countingStore struct {
	store app.VaultStore
	loads int
	saves int
}

func (s *countingStore) Create(ctx context.Context, mp app.MasterPassword, v domain.Vault) error {
	return s.store.Create(ctx, mp, v)
}

func (s *countingStore) Load(ctx context.Context, mp app.MasterPassword) (domain.Vault, error) {
	s.loads++
	return s.store.Load(ctx, mp)
}

func (s *countingStore) Save(ctx context.Context, mp app.MasterPassword, v domain.Vault) error {
	s.saves++
	return s.store.Save(ctx, mp, v)
}

// countStoreCalls makes h's vault store a countingStore and returns it.
func (h *vaultHarness) countStoreCalls() *countingStore {
	c := &countingStore{}
	h.deps.NewStore = func(path string) app.VaultStore {
		c.store = encfile.New(path, cheapParams)
		return c
	}
	return c
}

func assertStoreCalls(t *testing.T, c *countingStore, loads, saves int) {
	t.Helper()
	if c.loads != loads || c.saves != saves {
		t.Fatalf("store calls: %d Load, %d Save; want %d Load, %d Save", c.loads, c.saves, loads, saves)
	}
}

func assertPrompts(t *testing.T, h vaultHarness, want ...string) {
	t.Helper()
	if got := strings.Join(h.prompter.prompts, "|"); got != strings.Join(want, "|") {
		t.Fatalf("prompts = %q, want %q", h.prompter.prompts, want)
	}
}

func TestAddWithWrongMasterFailsBeforeTheEntryPrompt(t *testing.T) {
	path := newVault(t)
	before := readFile(t, path)
	h := newVaultHarness(t, "wrong master pw")
	calls := h.countStoreCalls()

	code := h.run(t, "--vault", path, "add", "github")

	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, h.stderr.String())
	}
	if got := h.stderr.String(); got != unlockLine {
		t.Fatalf("stderr = %q, want %q", got, unlockLine)
	}
	assertPrompts(t, h, "Master password: ")
	assertStoreCalls(t, calls, 1, 0)
	assertUnchanged(t, path, before)
}

func TestAddWithWrongMasterFromStdinLoadsOnceAndNeverSaves(t *testing.T) {
	path := newVault(t)
	before := readFile(t, path)
	h := newVaultHarness(t).withStdin("wrong\nentry-pw\n")
	calls := h.countStoreCalls()

	code := h.run(t, "--password-stdin", "--vault", path, "add", "github")

	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, h.stderr.String())
	}
	if got := h.stderr.String(); got != unlockLine {
		t.Fatalf("stderr = %q, want %q", got, unlockLine)
	}
	assertStoreCalls(t, calls, 1, 0)
	assertUnchanged(t, path, before)
}

func TestAddAgainstMissingVaultFailsAfterOnlyTheMasterPassword(t *testing.T) {
	t.Run("prompt", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.db")
		h := newVaultHarness(t, masterPassword)

		code := h.run(t, "--vault", path, "add", "github")

		assertFailure(t, h, code, `no vault at `+path+`; run "save_pwd init" first`)
		assertPrompts(t, h, "Master password: ")
		assertNoFile(t, path)
	})
	t.Run("password-stdin", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.db")
		// Only the master line: reading an entry password first would fail
		// with "no password provided on stdin" instead.
		h := newVaultHarness(t).withStdin(masterPassword + "\n")

		code := h.run(t, "--password-stdin", "--vault", path, "add", "github")

		assertFailure(t, h, code, `no vault at `+path+`; run "save_pwd init" first`)
		assertPrompts(t, h)
		assertNoFile(t, path)
	})
}

func TestAddGenerateWithWrongMasterFailsBeforeGenerating(t *testing.T) {
	path := newVault(t)
	before := readFile(t, path)
	h := newVaultHarness(t, "wrong master pw")
	h.deps.Random = failingReader{}

	code := h.run(t, "--vault", path, "add", "bank", "--generate")

	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, h.stderr.String())
	}
	if got := h.stderr.String(); got != unlockLine {
		t.Fatalf("stderr = %q, want only the unlock error %q", got, unlockLine)
	}
	if h.stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty (no Added line, no hint)", h.stdout.String())
	}
	assertUnchanged(t, path, before)
}

func TestPromptedAddAsksMasterThenEntryAndLoadsOnceSavesOnce(t *testing.T) {
	path := newVault(t)
	h := newVaultHarness(t, masterPassword, entryPassword, entryPassword)
	calls := h.countStoreCalls()

	code := h.run(t, "--vault", path, "add", "github")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	assertPrompts(t, h, "Master password: ", "Entry password: ", "Repeat entry password: ")
	assertStoreCalls(t, calls, 1, 1)
}

func TestDeleteLoadsOnceAndSavesOnlyWhenConfirmed(t *testing.T) {
	tests := []struct {
		name   string
		stdin  string
		args   []string
		code   int
		saves  int
		stdout string
	}{
		{name: "answered y", stdin: "y\n", args: []string{"delete", "github"}, code: 0, saves: 1, stdout: deletedGithub},
		{name: "--yes", stdin: "", args: []string{"delete", "github", "--yes"}, code: 0, saves: 1, stdout: deletedGithub},
		{name: "declined n", stdin: "n\n", args: []string{"delete", "github"}, code: 1, saves: 0},
		{name: "declined empty line", stdin: "\n", args: []string{"delete", "github"}, code: 1, saves: 0},
		{name: "declined EOF", stdin: "", args: []string{"delete", "github"}, code: 1, saves: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := vaultWithGithub(t)
			before := readFile(t, path)
			h := newVaultHarness(t, masterPassword).withStdin(tt.stdin)
			calls := h.countStoreCalls()

			code := h.run(t, append([]string{"--vault", path}, tt.args...)...)

			if code != tt.code {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, tt.code, h.stderr.String())
			}
			if got := h.stdout.String(); got != tt.stdout {
				t.Fatalf("stdout = %q, want %q", got, tt.stdout)
			}
			assertStoreCalls(t, calls, 1, tt.saves)
			if tt.saves == 0 {
				assertUnchanged(t, path, before)
			}
		})
	}
}
