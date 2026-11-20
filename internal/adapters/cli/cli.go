// Package cli is the driving adapter: the cobra command tree, its I/O and the
// mapping from errors to process exit codes.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"save_pwd/internal/app"
)

const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2

	programName = "save_pwd"
	helpHint    = "Run 'save_pwd --help' for usage."
)

// Streams are the process's standard streams.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Deps is the composition boundary: every field is a distinct, required port.
type Deps struct {
	Streams   Streams
	Prompter  Prompter
	LookupEnv func(key string) (string, bool)
	ConfigDir func() (string, error)
	NewStore  func(path string) app.VaultStore
	Clock     app.Clock
	Random    io.Reader
	Version   string
}

// usageError marks a problem with how the command was invoked (exit 2).
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// runError marks a failure that happened while a command was running (exit 1).
// Anything cobra returns without this mark failed during parsing, which is a
// usage error.
type runError struct{ err error }

func (e runError) Error() string { return e.err.Error() }
func (e runError) Unwrap() error { return e.err }

// Execute runs one invocation and returns the process exit code.
func Execute(ctx context.Context, args []string, d Deps) int {
	root := newRootCommand(d)
	root.SetArgs(args)
	root.SetIn(d.Streams.In)
	root.SetOut(d.Streams.Out)
	root.SetErr(d.Streams.Err)

	err := root.ExecuteContext(ctx)
	if err == nil {
		return exitOK
	}
	code := exitCode(err)
	fmt.Fprintf(d.Streams.Err, "%s: %v\n", programName, err)
	if code == exitUsage {
		fmt.Fprintln(d.Streams.Err, helpHint)
	}
	return code
}

func exitCode(err error) int {
	var usage usageError
	if errors.As(err, &usage) {
		return exitUsage
	}
	var run runError
	if errors.As(err, &run) {
		return exitFailure
	}
	return exitUsage
}

func newRootCommand(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           programName,
		Short:         "Store credentials in an encrypted vault file",
		Version:       d.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true

	var opts rootOptions
	root.PersistentFlags().StringVar(&opts.vaultPath, "vault", "", "path of the vault file")
	root.PersistentFlags().BoolVar(&opts.passwordStdin, "password-stdin", false,
		"read the master password from the first line of stdin instead of prompting")
	secrets := &secretSource{opts: &opts, prompter: d.Prompter, lines: newLineReader(d.Streams.In)}

	root.AddCommand(newGenerateCommand(d))
	locator := vaultLocator{opts: &opts, lookupEnv: d.LookupEnv, configDir: d.ConfigDir}

	root.AddCommand(newInitCommand(d, locator, secrets))
	root.AddCommand(newAddCommand(d, locator, secrets))
	root.AddCommand(newGetCommand(d, locator, secrets))
	root.AddCommand(newListCommand(d, locator, secrets))
	return root
}

// action adapts a command body to cobra, marking its errors as run failures
// unless the body already classified them as usage errors.
func action(body func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := body(cmd, args)
		if err == nil {
			return nil
		}
		var usage usageError
		if errors.As(err, &usage) {
			return err
		}
		return runError{err: err}
	}
}
