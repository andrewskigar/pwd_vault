package passgen

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"strings"

	"save_pwd/internal/domain"
)

const (
	lowerChars     = "abcdefghijklmnopqrstuvwxyz"
	upperChars     = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digitChars     = "0123456789"
	symbolChars    = "!@#$%^&*()-_=+[]{};:,.?/~"
	ambiguousChars = "O0o1lI"
)

// Generator draws passwords from an injected randomness source.
type Generator struct {
	rand io.Reader
}

// New returns a Generator reading from r. Production code passes
// crypto/rand.Reader; there is deliberately no fallback source.
func New(r io.Reader) *Generator {
	return &Generator{rand: r}
}

// Generate picks one character from each enabled class, fills the rest from
// their union, then shuffles, so every enabled class is guaranteed to appear.
func (g *Generator) Generate(p Policy) (domain.Password, error) {
	sets := p.alphabets()
	out := make([]byte, 0, p.length)
	for _, set := range sets {
		c, err := g.pick(set)
		if err != nil {
			return domain.Password{}, err
		}
		out = append(out, c)
	}
	union := strings.Join(sets, "")
	for len(out) < p.length {
		c, err := g.pick(union)
		if err != nil {
			return domain.Password{}, err
		}
		out = append(out, c)
	}
	if err := g.shuffle(out); err != nil {
		return domain.Password{}, err
	}
	return domain.NewPassword(string(out))
}

func (g *Generator) pick(set string) (byte, error) {
	i, err := g.index(len(set))
	if err != nil {
		return 0, err
	}
	return set[i], nil
}

// shuffle is a Fisher-Yates shuffle with uniform indices.
func (g *Generator) shuffle(b []byte) error {
	for i := len(b) - 1; i > 0; i-- {
		j, err := g.index(i + 1)
		if err != nil {
			return err
		}
		b[i], b[j] = b[j], b[i]
	}
	return nil
}

// index returns a uniform value in [0, n); rand.Int avoids modulo bias.
func (g *Generator) index(n int) (int, error) {
	v, err := rand.Int(g.rand, big.NewInt(int64(n)))
	if err != nil {
		return 0, fmt.Errorf("read randomness: %w", err)
	}
	return int(v.Int64()), nil
}

// alphabets returns the character set of each enabled class, in a fixed order.
func (p Policy) alphabets() []string {
	var sets []string
	for _, c := range [...]Classes{Lower, Upper, Digits, Symbols} {
		if p.classes&c == 0 {
			continue
		}
		sets = append(sets, p.filter(charsOf(c)))
	}
	return sets
}

func (p Policy) filter(set string) string {
	if p.ambiguity != ExcludeAmbiguous {
		return set
	}
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(ambiguousChars, r) {
			return -1
		}
		return r
	}, set)
}

func charsOf(c Classes) string {
	switch c {
	case Lower:
		return lowerChars
	case Upper:
		return upperChars
	case Digits:
		return digitChars
	case Symbols:
		return symbolChars
	}
	return ""
}
