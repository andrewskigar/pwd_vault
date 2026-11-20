package domain_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"save_pwd/internal/domain"
)

func TestPasswordIsMaskedInFormatting(t *testing.T) {
	pw, err := domain.NewPassword("hunter2-secret")
	if err != nil {
		t.Fatalf("NewPassword: %v", err)
	}

	for _, verb := range []string{"%v", "%s", "%+v", "%#v"} {
		t.Run(verb, func(t *testing.T) {
			got := fmt.Sprintf(verb, pw)
			if strings.Contains(got, "hunter2") {
				t.Fatalf("%s leaked the secret: %q", verb, got)
			}
			if !strings.Contains(got, "********") {
				t.Fatalf("%s = %q, want it to contain ********", verb, got)
			}
		})
	}

	t.Run("inside a struct", func(t *testing.T) {
		got := fmt.Sprintf("%+v", struct{ P domain.Password }{pw})
		if strings.Contains(got, "hunter2") {
			t.Fatalf("struct formatting leaked the secret: %q", got)
		}
	})
}

func TestPasswordIsMaskedInSlogText(t *testing.T) {
	pw, err := domain.NewPassword("hunter2-secret")
	if err != nil {
		t.Fatalf("NewPassword: %v", err)
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	logger.Info("saving", "password", pw)

	if strings.Contains(buf.String(), "hunter2") {
		t.Fatalf("slog leaked the secret: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "password=********") {
		t.Fatalf("slog output = %q, want password=********", buf.String())
	}
}

func TestPasswordRevealReturnsValue(t *testing.T) {
	pw, err := domain.NewPassword("hunter2-secret")
	if err != nil {
		t.Fatalf("NewPassword: %v", err)
	}
	if got := pw.Reveal(); got != "hunter2-secret" {
		t.Fatalf("Reveal() = %q, want %q", got, "hunter2-secret")
	}
	if pw.IsZero() {
		t.Fatal("IsZero() = true for a constructed password")
	}
	if !(domain.Password{}).IsZero() {
		t.Fatal("IsZero() = false for the zero Password")
	}
}

func TestNewPasswordRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "longer than 1024 bytes", value: strings.Repeat("a", 1025)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewPassword(tt.value)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("NewPassword(%q) error = %v, want ErrInvalid", tt.name, err)
			}
		})
	}

	if _, err := domain.NewPassword(strings.Repeat("a", 1024)); err != nil {
		t.Fatalf("NewPassword(1024 bytes) error = %v, want nil", err)
	}
}
