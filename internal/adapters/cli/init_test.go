package cli_test

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"save_pwd/internal/adapters/cli"
	"save_pwd/internal/adapters/encfile"
	"save_pwd/internal/app"
)

const masterPassword = "correct horse battery"

// cheapParams keeps Argon2id fast in tests; DefaultParams is never used here.
var cheapParams = encfile.Params{Time: 1, MemoryKiB: 64, Threads: 1}

// scriptedPrompter answers ReadSecret from a fixed script and fails the test
// when it is asked for more secrets than scripted. It returns the scripted
// slices themselves, so tests can check that the CLI wiped them.
type scriptedPrompter struct {
	t       *testing.T
	replies [][]byte
	errs    []error
	prompts []string
}

func (p *scriptedPrompter) ReadSecret(prompt string) ([]byte, error) {
	p.prompts = append(p.prompts, prompt)
	i := len(p.prompts) - 1
	if i < len(p.errs) && p.errs[i] != nil {
		return nil, p.errs[i]
	}
	if i >= len(p.replies) {
		p.t.Errorf("unexpected prompt %q", prompt)
		return nil, errors.New("unexpected prompt")
	}
	return p.replies[i], nil
}

// vaultHarness is a harness wired to a real encfile store in a temp dir.
type vaultHarness struct {
	harness
	prompter *scriptedPrompter
	dir      string
}

func newVaultHarness(t *testing.T, secrets ...string) vaultHarness {
	t.Helper()
	h := newHarness(rand.Reader)
	p := &scriptedPrompter{t: t}
	for _, s := range secrets {
		p.replies = append(p.replies, []byte(s))
	}
	h.deps.Prompter = p
	h.deps.NewStore = func(path string) app.VaultStore { return encfile.New(path, cheapParams) }
	h.deps.Clock = func() time.Time { return time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC) }
	// SAVE_PWD_VAULT is unset and the config dir is private to the test, so no
	// test can touch the real user config dir.
	h.deps.LookupEnv = mapEnv(map[string]string{})
	configDir := t.TempDir()
	h.deps.ConfigDir = func() (string, error) { return configDir, nil }
	return vaultHarness{harness: h, prompter: p, dir: t.TempDir()}
}

func (h vaultHarness) withStdin(s string) vaultHarness {
	h.deps.Streams.In = strings.NewReader(s)
	return h
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat %s: err = %v, want the file not to exist", path, err)
	}
}

func assertFailure(t *testing.T, h vaultHarness, code int, wantInStderr string) {
	t.Helper()
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, h.stderr.String())
	}
	if h.stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", h.stdout.String())
	}
	stderr := h.stderr.String()
	if !strings.HasPrefix(stderr, "save_pwd: ") || !strings.Contains(stderr, wantInStderr) {
		t.Fatalf("stderr = %q, want a save_pwd: message containing %q", stderr, wantInStderr)
	}
	if strings.Contains(stderr, helpHint) {
		t.Fatalf("stderr = %q, a runtime failure must not show the usage hint", stderr)
	}
}

func TestInitCreatesVaultWithPromptedPassword(t *testing.T) {
	h := newVaultHarness(t, masterPassword, masterPassword)
	path := filepath.Join(h.dir, "sub", "v.db")

	code := h.run(t, "--vault", path, "init")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if got, want := h.stdout.String(), "Created vault at "+path+"\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := h.prompter.prompts, []string{"Master password: ", "Repeat master password: "}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("prompts = %q, want %q", got, want)
	}
	file, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat vault: %v", err)
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if got := file.Mode().Perm(); got != 0o600 {
		t.Errorf("vault mode = %o, want 600", got)
	}
	if got := dir.Mode().Perm(); got != 0o700 {
		t.Errorf("dir mode = %o, want 700", got)
	}
}

func TestInitWritesVersionedHeaderWithConfiguredParams(t *testing.T) {
	h := newVaultHarness(t, masterPassword, masterPassword)
	path := filepath.Join(h.dir, "v.db")

	if code := h.run(t, "--vault", path, "init"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vault: %v", err)
	}
	// Offsets from the spec's file format v1 table; 62 header bytes plus a
	// 16-byte Poly1305 tag is the smallest possible file.
	if len(raw) < 62+16 {
		t.Fatalf("vault is %d bytes, want at least 78", len(raw))
	}
	if got := string(raw[0:8]); got != "SAVE_PWD" {
		t.Errorf("magic = %q, want SAVE_PWD", got)
	}
	if got := binary.BigEndian.Uint16(raw[8:10]); got != 1 {
		t.Errorf("format version = %d, want 1", got)
	}
	if raw[10] != 1 || raw[11] != 1 || raw[37] != 1 {
		t.Errorf("unlock method, KDF id, AEAD id = %d, %d, %d, want 1, 1, 1", raw[10], raw[11], raw[37])
	}
	if got := binary.BigEndian.Uint32(raw[12:16]); got != cheapParams.Time {
		t.Errorf("argon2 time = %d, want %d", got, cheapParams.Time)
	}
	if got := binary.BigEndian.Uint32(raw[16:20]); got != cheapParams.MemoryKiB {
		t.Errorf("argon2 memory = %d, want %d", got, cheapParams.MemoryKiB)
	}
	if raw[20] != cheapParams.Threads {
		t.Errorf("argon2 threads = %d, want %d", raw[20], cheapParams.Threads)
	}
	if bytes.Contains(raw, []byte(masterPassword)) {
		t.Error("the vault file contains the master password")
	}
}

func TestInitWithPasswordStdinDoesNotPrompt(t *testing.T) {
	h := newVaultHarness(t).withStdin(masterPassword + "\r\n")
	path := filepath.Join(h.dir, "v.db")

	code := h.run(t, "--password-stdin", "--vault", path, "init")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if len(h.prompter.prompts) != 0 {
		t.Fatalf("prompter was called with %q, want no prompts", h.prompter.prompts)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat vault: %v", err)
	}
	// The stripped line is the master password: it must unlock the vault.
	mp := app.NewMasterPassword([]byte(masterPassword))
	if _, err := encfile.New(path, cheapParams).Load(t.Context(), mp); err != nil {
		t.Fatalf("Load with the stdin password: %v", err)
	}
}

func TestInitFailuresLeaveNoFile(t *testing.T) {
	tests := []struct {
		name    string
		secrets []string
		stdin   *string
		want    string
	}{
		{name: "mismatched confirmation", secrets: []string{masterPassword, masterPassword + "x"}, want: "do not match"},
		{name: "master password too short", secrets: []string{"short12", "short12"}, want: app.ErrWeakMaster.Error()},
		{name: "empty stdin", stdin: ptr(""), want: "no password provided on stdin"},
		{name: "blank stdin line", stdin: ptr("\n"), want: "no password provided on stdin"},
		{name: "short stdin password", stdin: ptr("short12\n"), want: app.ErrWeakMaster.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newVaultHarness(t, tt.secrets...)
			args := []string{"--vault", filepath.Join(h.dir, "v.db"), "init"}
			if tt.stdin != nil {
				h = h.withStdin(*tt.stdin)
				args = append([]string{"--password-stdin"}, args...)
			}

			code := h.run(t, args...)

			assertFailure(t, h, code, tt.want)
			assertNoFile(t, filepath.Join(h.dir, "v.db"))
			for _, s := range tt.secrets {
				if strings.Contains(h.stderr.String(), s) {
					t.Fatalf("stderr %q contains a secret", h.stderr.String())
				}
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestInitRefusesToOverwriteExistingVault(t *testing.T) {
	h := newVaultHarness(t, masterPassword, masterPassword)
	path := filepath.Join(h.dir, "v.db")
	original := []byte("an existing file that must survive")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	code := h.run(t, "--vault", path, "init")

	assertFailure(t, h, code, "vault already exists at "+path)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("existing file changed to %q", got)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(h.dir, ".vault-*.tmp")); len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

func TestInitWithoutTerminalSuggestsPasswordStdin(t *testing.T) {
	h := newVaultHarness(t)
	h.prompter.errs = []error{cli.ErrNoTerminal}
	path := filepath.Join(h.dir, "v.db")

	code := h.run(t, "--vault", path, "init")

	assertFailure(t, h, code, "--password-stdin")
	assertNoFile(t, path)
}

func TestInitWipesPromptedSecrets(t *testing.T) {
	tests := []struct {
		name    string
		secrets []string
	}{
		{name: "success", secrets: []string{masterPassword, masterPassword}},
		{name: "mismatch", secrets: []string{masterPassword, masterPassword + "x"}},
		{name: "weak", secrets: []string{"short12", "short12"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newVaultHarness(t, tt.secrets...)

			h.run(t, "--vault", filepath.Join(h.dir, "v.db"), "init")

			for i, b := range h.prompter.replies {
				if !bytes.Equal(b, make([]byte, len(b))) {
					t.Fatalf("secret %d was not wiped: %q", i, b)
				}
			}
		})
	}
}
