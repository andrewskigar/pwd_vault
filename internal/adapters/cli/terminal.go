package cli

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// terminalPrompter reads secrets from a TTY with echo off. It is a thin
// wrapper over x/term, verified by hand rather than by automated tests.
type terminalPrompter struct {
	in      *os.File
	prompts io.Writer
}

// NewTerminalPrompter reads secrets from in and writes prompts to prompts.
// ReadSecret returns ErrNoTerminal when in is not a terminal.
func NewTerminalPrompter(in *os.File, prompts io.Writer) Prompter {
	return terminalPrompter{in: in, prompts: prompts}
}

func (p terminalPrompter) ReadSecret(prompt string) ([]byte, error) {
	fd := int(p.in.Fd())
	if !term.IsTerminal(fd) {
		return nil, ErrNoTerminal
	}
	fmt.Fprint(p.prompts, prompt)
	secret, err := term.ReadPassword(fd)
	// The user's Enter is not echoed, so end the prompt line ourselves.
	fmt.Fprintln(p.prompts)
	if err != nil {
		return nil, fmt.Errorf("read secret: %w", err)
	}
	return secret, nil
}
