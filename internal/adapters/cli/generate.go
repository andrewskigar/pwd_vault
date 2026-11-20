package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"save_pwd/internal/passgen"
)

type generateOptions struct {
	length      int
	noSymbols   bool
	noAmbiguous bool
}

func newGenerateCommand(d Deps) *cobra.Command {
	var opts generateOptions
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Print a random password",
		Args:  cobra.NoArgs,
		RunE: action(func(cmd *cobra.Command, _ []string) error {
			policy, err := opts.policy()
			if err != nil {
				return usageError{err: err}
			}
			return runGenerate(d, policy)
		}),
	}
	cmd.Flags().IntVar(&opts.length, "length", passgen.DefaultLength, "password length")
	cmd.Flags().BoolVar(&opts.noSymbols, "no-symbols", false, "leave out symbol characters")
	cmd.Flags().BoolVar(&opts.noAmbiguous, "no-ambiguous", false, "leave out look-alike characters (O0o1lI)")
	return cmd
}

func (o generateOptions) policy() (passgen.Policy, error) {
	classes := passgen.AllClasses
	if o.noSymbols {
		classes &^= passgen.Symbols
	}
	ambiguity := passgen.AllowAmbiguous
	if o.noAmbiguous {
		ambiguity = passgen.ExcludeAmbiguous
	}
	return passgen.NewPolicy(o.length, classes, ambiguity)
}

func runGenerate(d Deps, policy passgen.Policy) error {
	pw, err := passgen.New(d.Random).Generate(policy)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(d.Streams.Out, pw.Reveal()); err != nil {
		return fmt.Errorf("write password: %w", err)
	}
	if policy.IsWeak() {
		fmt.Fprintf(d.Streams.Err, "%s: warning: a %d-character password is weak; use at least %d\n",
			programName, policy.Length(), passgen.WeakBelow)
	}
	return nil
}
