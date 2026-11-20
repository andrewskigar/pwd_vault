package cli_test

import (
	"bytes"
	"strings"
	"testing"
)

// shownPasswordOf reads the named entry back through `get --show -- <name>`,
// so leading-dash names are looked up too.
func shownPasswordOf(t *testing.T, path, name string) string {
	t.Helper()
	get := newVaultHarness(t, masterPassword)
	if code := get.run(t, "--vault", path, "get", "--show", "--", name); code != 0 {
		t.Fatalf("get %q exit = %d (stderr %q)", name, code, get.stderr.String())
	}
	for _, line := range strings.Split(get.stdout.String(), "\n") {
		if pw, ok := strings.CutPrefix(line, "Password: "); ok {
			return pw
		}
	}
	t.Fatalf("get stdout %q has no Password line", get.stdout.String())
	return ""
}

// assertPromptedSecretsWiped checks every slice the prompter handed out.
func assertPromptedSecretsWiped(t *testing.T, h vaultHarness) {
	t.Helper()
	if len(h.prompter.prompts) != len(h.prompter.replies) {
		t.Fatalf("prompts = %q, want all %d scripted secrets read", h.prompter.prompts, len(h.prompter.replies))
	}
	for i, b := range h.prompter.replies {
		if !bytes.Equal(b, make([]byte, len(b))) {
			t.Fatalf("secret %d (%q prompt) was not wiped: %q", i, h.prompter.prompts[i], b)
		}
	}
}

func TestAddGenerateHintQuotesTheNameForShells(t *testing.T) {
	tests := []struct {
		name  string
		added string // the name as Go %q prints it; unchanged by the quoting rules
		args  []string
		hint  string
	}{
		{name: "bank", added: `"bank"`, hint: `save_pwd get bank --show`},
		{name: "alice@example.com", added: `"alice@example.com"`, hint: `save_pwd get alice@example.com --show`},
		{name: "my bank", added: `"my bank"`, hint: `save_pwd get "my bank" --show`},
		{name: `say "hi"`, added: `"say \"hi\""`, hint: `save_pwd get 'say "hi"' --show`},
		{name: "$HOME", added: `"$HOME"`, hint: `save_pwd get '$HOME' --show`},
		{name: "100%", added: `"100%"`, hint: `save_pwd get '100%' --show`},
		{name: "it's $5", added: `"it's $5"`, hint: `save_pwd get 'it'"'"'s $5' --show`},
		{
			name: "x'; $(echo pwned); 'y", added: `"x'; $(echo pwned); 'y"`,
			hint: `save_pwd get 'x'"'"'; $(echo pwned); '"'"'y' --show`,
		},
		{
			name: "-dash", added: `"-dash"`, hint: `save_pwd get --show -- "-dash"`,
			args: []string{"add", "--generate", "--", "-dash"},
		},
		{name: "café", added: `"café"`, hint: `save_pwd get "café" --show`},
		// PowerShell ends a double-quoted string at any of U+201C U+201D U+201E,
		// so these names must be single-quoted.
		{name: "a” ; calc ; “b", added: `"a” ; calc ; “b"`, hint: `save_pwd get 'a” ; calc ; “b' --show`},
		{name: "„low", added: `"„low"`, hint: `save_pwd get '„low' --show`},
		// PowerShell ends a single-quoted string at any of U+2018 U+2019 U+201A
		// U+201B, but treats them as literal inside double quotes.
		{name: "it’s", added: `"it’s"`, hint: `save_pwd get "it’s" --show`},
		{name: "it’s $5", added: `"it’s $5"`, hint: `save_pwd get 'it'"’"'s $5' --show`},
		{name: "‘a‚b‛%", added: `"‘a‚b‛%"`, hint: `save_pwd get ''"‘"'a'"‚"'b'"‛"'%' --show`},
		{name: "“it’s”", added: `"“it’s”"`, hint: `save_pwd get '“it'"’"'s”' --show`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := newVault(t)
			h := newVaultHarness(t, masterPassword)
			args := tt.args
			if args == nil {
				args = []string{"add", tt.name, "--generate"}
			}

			code := h.run(t, append([]string{"--vault", path}, args...)...)

			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
			}
			want := "Added " + tt.added + "\n" +
				"(generated password; view it with: " + tt.hint + ")\n"
			if got := h.stdout.String(); got != want {
				t.Fatalf("stdout = %q, want %q", got, want)
			}
			pw := shownPasswordOf(t, path, tt.name)
			if strings.Contains(h.stdout.String(), pw) || strings.Contains(h.stderr.String(), pw) {
				t.Fatal("add printed the generated password")
			}
		})
	}
}

func TestPromptedAddWipesEverySecret(t *testing.T) {
	tests := []struct {
		name    string
		secrets []string
		code    int
	}{
		{name: "success", secrets: []string{masterPassword, entryPassword, entryPassword}, code: 0},
		{name: "entry mismatch", secrets: []string{masterPassword, entryPassword, entryPassword + "x"}, code: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := newVault(t)
			h := newVaultHarness(t, tt.secrets...)

			code := h.run(t, "--vault", path, "add", "bank")

			if code != tt.code {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, tt.code, h.stderr.String())
			}
			assertPromptedSecretsWiped(t, h)
		})
	}
}

func TestUpdateChangePasswordWipesEverySecret(t *testing.T) {
	path := vaultWithGithub(t)
	h := newVaultHarness(t, masterPassword, "new-entry-pw", "new-entry-pw")

	code := h.run(t, "--vault", path, "update", "github", "--change-password")

	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, h.stderr.String())
	}
	assertPromptedSecretsWiped(t, h)
}
