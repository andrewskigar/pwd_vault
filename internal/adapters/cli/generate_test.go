package cli_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"testing"

	"save_pwd/internal/adapters/cli"
)

const (
	lowerSet     = "abcdefghijklmnopqrstuvwxyz"
	upperSet     = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digitSet     = "0123456789"
	symbolSet    = "!@#$%^&*()-_=+[]{};:,.?/~"
	ambiguousSet = "O0o1lI"
	helpHint     = "Run 'save_pwd --help' for usage."
)

// harness captures one in-process invocation of the CLI.
type harness struct {
	deps   cli.Deps
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func newHarness(random io.Reader) harness {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	return harness{
		deps: cli.Deps{
			Streams: cli.Streams{In: strings.NewReader(""), Out: stdout, Err: stderr},
			Random:  random,
			Version: "1.2.3-test",
		},
		stdout: stdout,
		stderr: stderr,
	}
}

func (h harness) run(t *testing.T, args ...string) int {
	t.Helper()
	return cli.Execute(context.Background(), args, h.deps)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

func singleLine(t *testing.T, out string) string {
	t.Helper()
	if !strings.HasSuffix(out, "\n") || strings.Count(out, "\n") != 1 {
		t.Fatalf("stdout = %q, want exactly one line", out)
	}
	return strings.TrimSuffix(out, "\n")
}

func assertOnlyFrom(t *testing.T, s, alphabet string) {
	t.Helper()
	for _, r := range s {
		if !strings.ContainsRune(alphabet, r) {
			t.Fatalf("%q contains %q, outside the allowed alphabet", s, r)
		}
	}
}

func assertContainsEach(t *testing.T, s string, sets ...string) {
	t.Helper()
	for _, set := range sets {
		if !strings.ContainsAny(s, set) {
			t.Fatalf("%q has no character from %q", s, set)
		}
	}
}

func TestGenerateDefaultPrintsTwentyCharacterPassword(t *testing.T) {
	h := newHarness(rand.Reader)

	code := h.run(t, "generate")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if h.stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", h.stderr.String())
	}
	pw := singleLine(t, h.stdout.String())
	if len(pw) != 20 {
		t.Fatalf("password %q has length %d, want 20", pw, len(pw))
	}
	assertOnlyFrom(t, pw, lowerSet+upperSet+digitSet+symbolSet)
	assertContainsEach(t, pw, lowerSet, upperSet, digitSet, symbolSet)
}

func TestGenerateWithoutSymbolsAndAmbiguousCharacters(t *testing.T) {
	h := newHarness(rand.Reader)

	code := h.run(t, "generate", "--length", "32", "--no-symbols", "--no-ambiguous")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if h.stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", h.stderr.String())
	}
	pw := singleLine(t, h.stdout.String())
	if len(pw) != 32 {
		t.Fatalf("password %q has length %d, want 32", pw, len(pw))
	}
	assertOnlyFrom(t, pw, lowerSet+upperSet+digitSet)
	if strings.ContainsAny(pw, ambiguousSet) {
		t.Fatalf("password %q contains an ambiguous character", pw)
	}
	assertContainsEach(t, pw, lowerSet, upperSet, digitSet)
}

func TestGenerateWeakLengthWarnsOnStderr(t *testing.T) {
	h := newHarness(rand.Reader)

	code := h.run(t, "generate", "--length", "10")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	pw := singleLine(t, h.stdout.String())
	if len(pw) != 10 {
		t.Fatalf("password %q has length %d, want 10", pw, len(pw))
	}
	if !strings.HasPrefix(h.stderr.String(), "save_pwd: ") || !strings.Contains(h.stderr.String(), "warning") {
		t.Fatalf("stderr = %q, want a save_pwd: weak-length warning", h.stderr.String())
	}
	if strings.Contains(h.stderr.String(), pw) {
		t.Fatal("the warning must not contain the password")
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "length below minimum", args: []string{"generate", "--length", "7"}},
		{name: "length above maximum", args: []string{"generate", "--length", "257"}},
		{name: "non-numeric length", args: []string{"generate", "--length", "abc"}},
		{name: "unknown command", args: []string{"frobnicate"}},
		{name: "unknown flag", args: []string{"generate", "--bogus"}},
		{name: "unknown root flag", args: []string{"--bogus"}},
		{name: "extra argument", args: []string{"generate", "extra-arg"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(rand.Reader)

			code := h.run(t, tt.args...)

			if code != 2 {
				t.Fatalf("exit = %d, want 2 (stderr %q)", code, h.stderr.String())
			}
			if h.stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", h.stdout.String())
			}
			stderr := h.stderr.String()
			if !strings.HasPrefix(stderr, "save_pwd: ") {
				t.Fatalf("stderr = %q, want the save_pwd: prefix", stderr)
			}
			if !strings.Contains(stderr, helpHint) {
				t.Fatalf("stderr = %q, want the --help hint", stderr)
			}
		})
	}
}

func TestHelpPrintsUsageToStdout(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"generate", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := newHarness(rand.Reader)

			code := h.run(t, args...)

			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
			}
			if !strings.Contains(h.stdout.String(), "Usage:") {
				t.Fatalf("stdout = %q, want usage", h.stdout.String())
			}
			if h.stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", h.stderr.String())
			}
		})
	}
}

func TestVersionPrintsInjectedVersion(t *testing.T) {
	h := newHarness(rand.Reader)

	code := h.run(t, "--version")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if !strings.Contains(h.stdout.String(), "1.2.3-test") {
		t.Fatalf("stdout = %q, want the injected version", h.stdout.String())
	}
}

func TestGenerateFailingRandomnessExitsOne(t *testing.T) {
	h := newHarness(failingReader{})

	code := h.run(t, "generate")

	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, h.stderr.String())
	}
	if h.stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", h.stdout.String())
	}
	stderr := h.stderr.String()
	if !strings.HasPrefix(stderr, "save_pwd: ") || !strings.Contains(stderr, "entropy unavailable") {
		t.Fatalf("stderr = %q, want a save_pwd: message naming the reader failure", stderr)
	}
	if strings.Contains(stderr, helpHint) {
		t.Fatalf("stderr = %q, a runtime failure must not show the usage hint", stderr)
	}
}
