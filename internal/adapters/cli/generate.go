package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"save_pwd/internal/passgen"
)

const (
	flagGenerate    = "generate"
	flagLength      = "length"
	flagNoSymbols   = "no-symbols"
	flagNoAmbiguous = "no-ambiguous"
)

// generationFlags is the one definition of the flags that shape a generated
// password, shared by every command that generates one.
type generationFlags struct {
	length      int
	noSymbols   bool
	noAmbiguous bool
}

func (g *generationFlags) register(fs *pflag.FlagSet) {
	fs.IntVar(&g.length, flagLength, passgen.DefaultLength, "password length")
	fs.BoolVar(&g.noSymbols, flagNoSymbols, false, "leave out symbol characters")
	fs.BoolVar(&g.noAmbiguous, flagNoAmbiguous, false, "leave out look-alike characters (O0o1lI)")
}

func (g generationFlags) policy() (passgen.Policy, error) {
	classes := passgen.AllClasses
	if g.noSymbols {
		classes &^= passgen.Symbols
	}
	ambiguity := passgen.AllowAmbiguous
	if g.noAmbiguous {
		ambiguity = passgen.ExcludeAmbiguous
	}
	return passgen.NewPolicy(g.length, classes, ambiguity)
}

// optionalGeneration is --generate plus the generation flags, for commands
// whose entry password is prompted unless --generate asks for a random one.
type optionalGeneration struct {
	enabled bool
	flags   generationFlags
}

func (o *optionalGeneration) register(fs *pflag.FlagSet) {
	fs.BoolVar(&o.enabled, flagGenerate, false, "generate a random entry password instead of prompting")
	o.flags.register(fs)
}

// policy reports whether a password is to be generated and with which policy.
// Generation flags without --generate are rejected rather than ignored, so a
// user never believes a length took effect when it did not.
func (o optionalGeneration) policy(fs *pflag.FlagSet) (passgen.Policy, bool, error) {
	if !o.enabled {
		for _, name := range []string{flagLength, flagNoSymbols, flagNoAmbiguous} {
			if fs.Changed(name) {
				return passgen.Policy{}, false, fmt.Errorf("--%s requires --%s", name, flagGenerate)
			}
		}
		return passgen.Policy{}, false, nil
	}
	p, err := o.flags.policy()
	if err != nil {
		return passgen.Policy{}, false, err
	}
	return p, true, nil
}

func newGenerateCommand(d Deps) *cobra.Command {
	var flags generationFlags
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Print a random password",
		Args:  cobra.NoArgs,
		RunE: action(func(cmd *cobra.Command, _ []string) error {
			policy, err := flags.policy()
			if err != nil {
				return usageError{err: err}
			}
			return runGenerate(d, policy)
		}),
	}
	flags.register(cmd.Flags())
	return cmd
}

func runGenerate(d Deps, policy passgen.Policy) error {
	pw, err := passgen.New(d.Random).Generate(policy)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(d.Streams.Out, pw.Reveal()); err != nil {
		return fmt.Errorf("write password: %w", err)
	}
	warnIfWeak(d.Streams.Err, policy)
	return nil
}

// warnIfWeak flags a short but accepted length on stderr; a failed write of a
// warning is not worth failing a command that already succeeded.
func warnIfWeak(w io.Writer, policy passgen.Policy) {
	if policy.IsWeak() {
		fmt.Fprintf(w, "%s: warning: a %d-character password is weak; use at least %d\n",
			programName, policy.Length(), passgen.WeakBelow)
	}
}
