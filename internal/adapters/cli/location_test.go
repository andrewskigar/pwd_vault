package cli_test

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// mapEnv is a map-backed LookupEnv.
func mapEnv(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

func TestInitResolvesVaultLocation(t *testing.T) {
	type where int
	const (
		atFlag where = iota
		atEnv
		atDefault
	)
	tests := []struct {
		name string
		flag bool
		env  *string // nil: SAVE_PWD_VAULT unset; "<env>" is replaced by the env path
		want where
	}{
		{name: "default when neither flag nor env is set", want: atDefault},
		{name: "env var when no flag", env: ptr("<env>"), want: atEnv},
		{name: "flag beats env var", flag: true, env: ptr("<env>"), want: atFlag},
		{name: "flag without env var", flag: true, want: atFlag},
		{name: "empty env var falls back to default", env: ptr(""), want: atDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newVaultHarness(t, masterPassword, masterPassword)
			configDir := t.TempDir()
			paths := map[where]string{
				atFlag:    filepath.Join(h.dir, "flag.db"),
				atEnv:     filepath.Join(h.dir, "env.db"),
				atDefault: filepath.Join(configDir, "save_pwd", "vault.db"),
			}
			env := map[string]string{}
			if tt.env != nil {
				env["SAVE_PWD_VAULT"] = *tt.env
				if *tt.env == "<env>" {
					env["SAVE_PWD_VAULT"] = paths[atEnv]
				}
			}
			h.deps.LookupEnv = mapEnv(env)
			h.deps.ConfigDir = func() (string, error) { return configDir, nil }
			args := []string{"init"}
			if tt.flag {
				args = []string{"--vault", paths[atFlag], "init"}
			}

			code := h.run(t, args...)

			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
			}
			want := paths[tt.want]
			if got := h.stdout.String(); got != "Created vault at "+want+"\n" {
				t.Fatalf("stdout = %q, want it to name %s", got, want)
			}
			if _, err := os.Stat(want); err != nil {
				t.Fatalf("stat %s: %v", want, err)
			}
			for w, p := range paths {
				if w != tt.want {
					assertNoFile(t, p)
				}
			}
			if tt.want != atDefault {
				assertNoFile(t, filepath.Join(configDir, "save_pwd"))
			}
		})
	}
}

func TestInitCreatesDefaultDirectoryPrivately(t *testing.T) {
	h := newVaultHarness(t, masterPassword, masterPassword)
	configDir := t.TempDir()
	h.deps.ConfigDir = func() (string, error) { return configDir, nil }
	path := filepath.Join(configDir, "save_pwd", "vault.db")

	if code := h.run(t, "init"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
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

func TestInitErrorNamesEnvResolvedPath(t *testing.T) {
	h := newVaultHarness(t, masterPassword, masterPassword)
	path := filepath.Join(h.dir, "env.db")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	h.deps.LookupEnv = mapEnv(map[string]string{"SAVE_PWD_VAULT": path})

	code := h.run(t, "init")

	assertFailure(t, h, code, "vault already exists at "+path)
}

func TestInitWithoutConfigDirSuggestsExplicitLocation(t *testing.T) {
	h := newVaultHarness(t, masterPassword, masterPassword)
	h.deps.ConfigDir = func() (string, error) { return "", errors.New("$HOME is not defined") }

	code := h.run(t, "init")

	assertFailure(t, h, code, "--vault")
	assertFailure(t, h, code, "SAVE_PWD_VAULT")
	if len(h.prompter.prompts) != 0 {
		t.Fatalf("prompter was called with %q before the path was known", h.prompter.prompts)
	}
}

func TestGenerateNeverResolvesVaultLocation(t *testing.T) {
	h := newHarness(rand.Reader)
	h.deps.LookupEnv = func(key string) (string, bool) {
		t.Errorf("generate called LookupEnv(%q)", key)
		return "", false
	}
	h.deps.ConfigDir = func() (string, error) {
		t.Error("generate called ConfigDir")
		return "", errors.New("must not be called")
	}

	if code := h.run(t, "generate"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	if code := h.run(t, "--vault", "ignored.db", "generate", "--length", "12"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
}
