package passgen_test

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"save_pwd/internal/domain"
	"save_pwd/internal/passgen"
)

// Character sets copied from the spec, independent of the implementation.
const (
	lowerSet     = "abcdefghijklmnopqrstuvwxyz"
	upperSet     = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digitSet     = "0123456789"
	symbolSet    = "!@#$%^&*()-_=+[]{};:,.?/~"
	ambiguousSet = "O0o1lI"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

func mustPolicy(t *testing.T, length int, classes passgen.Classes, a passgen.Ambiguity) passgen.Policy {
	t.Helper()
	p, err := passgen.NewPolicy(length, classes, a)
	if err != nil {
		t.Fatalf("NewPolicy(%d, %v, %v): %v", length, classes, a, err)
	}
	return p
}

func TestNewPolicyBounds(t *testing.T) {
	tests := []struct {
		name    string
		length  int
		classes passgen.Classes
		wantErr bool
	}{
		{name: "length 7 too short", length: 7, classes: passgen.AllClasses, wantErr: true},
		{name: "length 8 minimum", length: 8, classes: passgen.AllClasses},
		{name: "length 256 maximum", length: 256, classes: passgen.AllClasses},
		{name: "length 257 too long", length: 257, classes: passgen.AllClasses, wantErr: true},
		{name: "no classes", length: 20, classes: 0, wantErr: true},
		{name: "unknown class bit", length: 20, classes: 1 << 6, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := passgen.NewPolicy(tt.length, tt.classes, passgen.AllowAmbiguous)
			if tt.wantErr && !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("error = %v, want domain.ErrInvalid", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
		})
	}
}

func TestPolicyIsWeak(t *testing.T) {
	if !mustPolicy(t, 11, passgen.AllClasses, passgen.AllowAmbiguous).IsWeak() {
		t.Fatal("length 11 should be weak")
	}
	if mustPolicy(t, 12, passgen.AllClasses, passgen.AllowAmbiguous).IsWeak() {
		t.Fatal("length 12 should not be weak")
	}
	if passgen.DefaultPolicy().IsWeak() {
		t.Fatal("default policy should not be weak")
	}
}

func TestDefaultPolicyGeneratesTwentyCharactersOfEveryClass(t *testing.T) {
	pw, err := passgen.New(rand.Reader).Generate(passgen.DefaultPolicy())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := pw.Reveal()
	if len(got) != 20 {
		t.Fatalf("len = %d, want 20 (%q)", len(got), got)
	}
	assertOnlyFrom(t, got, lowerSet+upperSet+digitSet+symbolSet)
	for _, set := range []string{lowerSet, upperSet, digitSet, symbolSet} {
		if !strings.ContainsAny(got, set) {
			t.Fatalf("%q has no character from %q", got, set)
		}
	}
}

func TestGenerateGuaranteesEveryEnabledClass(t *testing.T) {
	tests := []struct {
		name    string
		classes passgen.Classes
		sets    []string
	}{
		{name: "all classes", classes: passgen.AllClasses, sets: []string{lowerSet, upperSet, digitSet, symbolSet}},
		{name: "no symbols", classes: passgen.Lower | passgen.Upper | passgen.Digits, sets: []string{lowerSet, upperSet, digitSet}},
		{name: "digits and symbols", classes: passgen.Digits | passgen.Symbols, sets: []string{digitSet, symbolSet}},
	}
	gen := passgen.New(rand.Reader)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := mustPolicy(t, 8, tt.classes, passgen.AllowAmbiguous)
			alphabet := strings.Join(tt.sets, "")
			for range 1000 {
				pw, err := gen.Generate(policy)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				got := pw.Reveal()
				if len(got) != 8 {
					t.Fatalf("len = %d, want 8", len(got))
				}
				assertOnlyFrom(t, got, alphabet)
				for _, set := range tt.sets {
					if !strings.ContainsAny(got, set) {
						t.Fatalf("%q has no character from %q", got, set)
					}
				}
			}
		})
	}
}

func TestGenerateExcludesAmbiguousCharacters(t *testing.T) {
	policy := mustPolicy(t, 256, passgen.AllClasses, passgen.ExcludeAmbiguous)
	gen := passgen.New(rand.Reader)
	for range 200 {
		pw, err := gen.Generate(policy)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if strings.ContainsAny(pw.Reveal(), ambiguousSet) {
			t.Fatalf("%q contains an ambiguous character", pw.Reveal())
		}
	}
}

// A coarse guard against modulo bias: 26,000+ draws from one class should hit
// every letter within plus or minus 30 percent of the uniform expectation.
func TestGenerateSingleClassFrequencyIsUniform(t *testing.T) {
	policy := mustPolicy(t, 208, passgen.Lower, passgen.AllowAmbiguous)
	gen := passgen.New(rand.Reader)
	counts := make(map[rune]int)
	total := 0
	for range 125 {
		pw, err := gen.Generate(policy)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		for _, r := range pw.Reveal() {
			counts[r]++
			total++
		}
	}
	expected := float64(total) / float64(len(lowerSet))
	for _, r := range lowerSet {
		got := float64(counts[r])
		if got < expected*0.7 || got > expected*1.3 {
			t.Errorf("letter %q drawn %v times, want within 30%% of %v", r, got, expected)
		}
	}
	if len(counts) != len(lowerSet) {
		t.Errorf("drew %d distinct characters, want %d", len(counts), len(lowerSet))
	}
}

func TestGenerateReturnsReaderErrors(t *testing.T) {
	_, err := passgen.New(failingReader{}).Generate(passgen.DefaultPolicy())
	if err == nil {
		t.Fatal("Generate with a failing reader returned no error")
	}
	if !strings.Contains(err.Error(), "entropy unavailable") {
		t.Fatalf("error = %v, want it to wrap the reader error", err)
	}
}

func assertOnlyFrom(t *testing.T, s, alphabet string) {
	t.Helper()
	for _, r := range s {
		if !strings.ContainsRune(alphabet, r) {
			t.Fatalf("%q contains %q, which is outside the allowed alphabet", s, r)
		}
	}
}
