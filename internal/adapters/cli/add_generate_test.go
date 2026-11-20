package cli_test

import (
	"strings"
	"testing"
)

const generatedHint = "(generated password; view it with: save_pwd get bank --show)\n"

// shownPassword reads the bank entry back through `get --show` and returns its
// password, so tests see what was stored without trusting add's output.
func shownPassword(t *testing.T, path string) string {
	t.Helper()
	get := newVaultHarness(t, masterPassword)
	if code := get.run(t, "--vault", path, "get", "bank", "--show"); code != 0 {
		t.Fatalf("get exit = %d (stderr %q)", code, get.stderr.String())
	}
	for _, line := range strings.Split(get.stdout.String(), "\n") {
		if pw, ok := strings.CutPrefix(line, "Password: "); ok {
			return pw
		}
	}
	t.Fatalf("get stdout %q has no Password line", get.stdout.String())
	return ""
}

func TestAddGenerateStoresPasswordWithoutPromptingOrPrinting(t *testing.T) {
	path := newVault(t)
	h := newVaultHarness(t, masterPassword) // an entry password prompt fails the test

	code := h.run(t, "--vault", path, "add", "bank", "--generate", "--length", "24")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if got, want := strings.Join(h.prompter.prompts, "|"), "Master password: "; got != want {
		t.Fatalf("prompts = %q, want %q", got, want)
	}
	if got, want := h.stdout.String(), "Added \"bank\"\n"+generatedHint; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if h.stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", h.stderr.String())
	}

	pw := shownPassword(t, path)

	if len(pw) != 24 {
		t.Fatalf("stored password %q has length %d, want 24", pw, len(pw))
	}
	assertOnlyFrom(t, pw, lowerSet+upperSet+digitSet+symbolSet)
	assertContainsEach(t, pw, lowerSet, upperSet, digitSet, symbolSet)
	if strings.Contains(h.stdout.String(), pw) || strings.Contains(h.stderr.String(), pw) {
		t.Fatal("add printed the generated password")
	}
}

func TestAddGenerateHonoursClassOptions(t *testing.T) {
	path := newVault(t)
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "add", "bank", "--generate", "--no-symbols", "--no-ambiguous")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	pw := shownPassword(t, path)
	if len(pw) != 20 {
		t.Fatalf("stored password %q has length %d, want the default 20", pw, len(pw))
	}
	assertOnlyFrom(t, pw, lowerSet+upperSet+digitSet)
	if strings.ContainsAny(pw, symbolSet+ambiguousSet) {
		t.Fatalf("stored password %q has a symbol or an ambiguous character", pw)
	}
}

func TestAddGenerationFlagsAreUsageErrorsBeforeAnyPrompt(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "length without generate", args: []string{"--length", "24"}, want: "--length requires --generate"},
		{name: "no-symbols without generate", args: []string{"--no-symbols"}, want: "--no-symbols requires --generate"},
		{name: "no-ambiguous without generate", args: []string{"--no-ambiguous"}, want: "--no-ambiguous requires --generate"},
		{name: "length below minimum", args: []string{"--generate", "--length", "5"}, want: "length"},
		{name: "length above maximum", args: []string{"--generate", "--length", "257"}, want: "length"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := newVault(t)
			before := readFile(t, path)
			h := newVaultHarness(t) // any prompt fails the test

			code := h.run(t, append([]string{"--vault", path, "add", "bank"}, tt.args...)...)

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

func TestAddGenerationFlagsWithoutGenerateNeverTouchTheVault(t *testing.T) {
	path := t.TempDir() + "/missing.db"
	h := newVaultHarness(t)

	code := h.run(t, "--vault", path, "add", "bank", "--length", "24")

	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr %q)", code, h.stderr.String())
	}
	if strings.Contains(h.stderr.String(), "no vault") {
		t.Fatalf("stderr = %q, the vault was consulted before the usage check", h.stderr.String())
	}
	assertNoFile(t, path)
}

func TestAddGenerateWeakLengthWarns(t *testing.T) {
	path := newVault(t)
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "add", "bank", "--generate", "--length", "10")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if got, want := h.stdout.String(), "Added \"bank\"\n"+generatedHint; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	want := "save_pwd: warning: a 10-character password is weak; use at least 12\n"
	if got := h.stderr.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if pw := shownPassword(t, path); len(pw) != 10 {
		t.Fatalf("stored password %q has length %d, want 10", pw, len(pw))
	}
}

func TestAddGenerateWithPasswordStdinReadsOnlyMasterLine(t *testing.T) {
	path := newVault(t)
	h := newVaultHarness(t).withStdin(masterPassword + "\n")

	code := h.run(t, "--password-stdin", "--vault", path, "add", "bank", "--generate")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if pw := shownPassword(t, path); len(pw) != 20 {
		t.Fatalf("stored password %q has length %d, want 20", pw, len(pw))
	}
}

func TestAddGenerateWithFailingRandomLeavesVaultUnchanged(t *testing.T) {
	path := newVault(t)
	before := readFile(t, path)
	h := newVaultHarness(t, masterPassword)
	h.deps.Random = failingReader{}

	code := h.run(t, "--vault", path, "add", "bank", "--generate")

	assertFailure(t, h, code, "entropy unavailable")
	assertUnchanged(t, path, before)
}
