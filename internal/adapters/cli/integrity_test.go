package cli_test

import (
	"os"
	"testing"
)

func TestCorruptedVaultFileFailsLikeWrongPassword(t *testing.T) {
	path := vaultWithGithub(t)
	raw := readFile(t, path)
	raw[len(raw)/2] ^= 0x01
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	h := newVaultHarness(t, masterPassword)

	code := h.run(t, "--vault", path, "get", "github")

	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", code, h.stderr.String())
	}
	if got := h.stderr.String(); got != unlockLine {
		t.Fatalf("stderr = %q, want %q", got, unlockLine)
	}
}
