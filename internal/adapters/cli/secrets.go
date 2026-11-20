package cli

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"

	"save_pwd/internal/app"
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
)

const (
	promptMaster       = "Master password: "
	promptRepeatMaster = "Repeat master password: "
)

// rootOptions are the persistent flags shared by every vault command.
type rootOptions struct {
	vaultPath     string
	passwordStdin bool
}

func (o *rootOptions) vault() (string, error) {
	if o.vaultPath == "" {
		return "", usageError{err: errors.New("no vault path given; use --vault PATH")}
	}
	return o.vaultPath, nil
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
	line, err := l.r.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		clear(line)
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	trimmed := bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
	if len(trimmed) == 0 {
		return nil, errNoStdinPassword
	}
	return trimmed, nil
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
	return s.promptConfirmedMaster()
}

func (s *secretSource) promptConfirmedMaster() (app.MasterPassword, error) {
	first, err := s.prompter.ReadSecret(promptMaster)
	if err != nil {
		return app.MasterPassword{}, err
	}
	second, err := s.prompter.ReadSecret(promptRepeatMaster)
	defer clear(second)
	if err != nil {
		clear(first)
		return app.MasterPassword{}, err
	}
	if subtle.ConstantTimeCompare(first, second) != 1 {
		clear(first)
		return app.MasterPassword{}, errMasterMismatch
	}
	return app.NewMasterPassword(first), nil
}
