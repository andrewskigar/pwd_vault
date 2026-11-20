package app_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"save_pwd/internal/app"
)

func TestMasterPasswordIsMasked(t *testing.T) {
	mp := app.NewMasterPassword([]byte("hunter2-secret"))

	for _, verb := range []string{"%v", "%s", "%+v", "%#v"} {
		t.Run(verb, func(t *testing.T) {
			got := fmt.Sprintf(verb, mp)
			if strings.Contains(got, "hunter2") || strings.Contains(got, "104") {
				t.Fatalf("%s leaked the secret: %q", verb, got)
			}
			if !strings.Contains(got, "********") {
				t.Fatalf("%s = %q, want it to contain ********", verb, got)
			}
		})
	}
}

func TestMasterPasswordWipeClearsTheSourceBuffer(t *testing.T) {
	src := []byte("hunter2-secret")
	mp := app.NewMasterPassword(src)
	if !bytes.Equal(mp.Bytes(), []byte("hunter2-secret")) {
		t.Fatalf("Bytes() = %q, want the source bytes", mp.Bytes())
	}

	mp.Wipe()

	if !bytes.Equal(src, make([]byte, len(src))) {
		t.Fatalf("source buffer = %q after Wipe, want all zero", src)
	}
}
