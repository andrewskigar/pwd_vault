package cli

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

// Prompter reads a secret without echo; the prompt goes to stderr.
type Prompter interface {
	ReadSecret(prompt string) ([]byte, error)
}

// ErrNoTerminal means a secret was needed but stdin is not a terminal.
var ErrNoTerminal = errors.New("no terminal available; use --password-stdin")

var (
	errNoStdinPassword = errors.New("no password provided on stdin")
	errMasterMismatch  = errors.New("master passwords do not match")
	errEntryMismatch   = errors.New("entry passwords do not match")
)

const (
	promptMaster       = "Master password: "
	promptRepeatMaster = "Repeat master password: "
	promptEntry        = "Entry password: "
	promptRepeatEntry  = "Repeat entry password: "
)

// rootOptions are the persistent flags shared by every vault command.
type rootOptions struct {
	vaultPath     string
	passwordStdin bool
}

// lineReader hands out stdin lines in order. One instance is shared by a whole
// invocation, so later secrets read the lines after the master password.
type lineReader struct {
	r *bufio.Reader
}

func newLineReader(in io.Reader) *lineReader {
	return &lineReader{r: bufio.NewReader(in)}
}

// next returns the next line without its line ending; an empty or missing
// line is an error because it can never be a valid secret.
func (l *lineReader) next() ([]byte, error) {
	line, err := l.line()
	if err != nil {
		return nil, err
	}
	if len(line) == 0 {
		return nil, errNoStdinPassword
	}
	return line, nil
}

// line returns the next line without its line ending; a missing line at EOF
// is empty, not an error.
func (l *lineReader) line() ([]byte, error) {
	line, err := l.r.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		clear(line)
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r")), nil
}

// secretSource reads secrets from stdin lines or the prompter, as chosen by
// --password-stdin.
type secretSource struct {
	opts     *rootOptions
	prompter Prompter
	lines    *lineReader
}

// newMaster reads a master password for a new vault. Prompted passwords are
// confirmed; a stdin password is not, since scripts cannot mistype.
func (s *secretSource) newMaster() (app.MasterPassword, error) {
	if s.opts.passwordStdin {
		line, err := s.lines.next()
		if err != nil {
			return app.MasterPassword{}, err
		}
		return app.NewMasterPassword(line), nil
	}
	first, err := s.promptConfirmed(promptMaster, promptRepeatMaster, errMasterMismatch)
	if err != nil {
		return app.MasterPassword{}, err
	}
	return app.NewMasterPassword(first), nil
}

// master reads the master password of an existing vault: one prompt or the
// first stdin line. A typo surfaces as the unlock error, so no confirmation.
func (s *secretSource) master() (app.MasterPassword, error) {
	b, err := s.read(promptMaster)
	if err != nil {
		return app.MasterPassword{}, err
	}
	return app.NewMasterPassword(b), nil
}

// newEntryPassword reads an entry password: two matching prompts, or the next
// stdin line. The raw bytes are wiped right after the single conversion into
// the domain value, before validation can return early.
func (s *secretSource) newEntryPassword() (domain.Password, error) {
	var (
		b   []byte
		err error
	)
	if s.opts.passwordStdin {
		b, err = s.lines.next()
	} else {
		b, err = s.promptConfirmed(promptEntry, promptRepeatEntry, errEntryMismatch)
	}
	if err != nil {
		return domain.Password{}, err
	}
	pw, err := domain.NewPassword(string(b))
	clear(b)
	if err != nil {
		return domain.Password{}, err
	}
	return pw, nil
}

// answer reads a plain, non-secret reply from the next stdin line. Without
// --password-stdin the master password came from the prompter, so this is the
// first line of stdin; with it, the line after the master password. A missing
// line (EOF) is an empty answer.
func (s *secretSource) answer() (string, error) {
	line, err := s.lines.line()
	if err != nil {
		return "", err
	}
	return string(line), nil
}

// read returns one secret from the next stdin line or a single prompt.
func (s *secretSource) read(prompt string) ([]byte, error) {
	if s.opts.passwordStdin {
		return s.lines.next()
	}
	return s.prompter.ReadSecret(prompt)
}

// promptConfirmed prompts twice and returns the first secret when both match;
// otherwise it wipes both and returns mismatch.
func (s *secretSource) promptConfirmed(prompt, repeat string, mismatch error) ([]byte, error) {
	first, err := s.prompter.ReadSecret(prompt)
	if err != nil {
		return nil, err
	}
	second, err := s.prompter.ReadSecret(repeat)
	defer clear(second)
	if err != nil {
		clear(first)
		return nil, err
	}
	if subtle.ConstantTimeCompare(first, second) != 1 {
		clear(first)
		return nil, mismatch
	}
	return first, nil
}
