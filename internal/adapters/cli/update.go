package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
	"save_pwd/internal/passgen"
)

const (
	flagKind           = "kind"
	flagUsername       = "username"
	flagURL            = "url"
	flagNotes          = "notes"
	flagExpires        = "expires"
	flagChangePassword = "change-password"
)

var errPasswordSourceConflict = fmt.Errorf("--%s and --%s are mutually exclusive", flagChangePassword, flagGenerate)

type updateOptions struct {
	kind           string
	username       string
	url            string
	notes          string
	expires        string
	changePassword bool
	generation     optionalGeneration
}

// passwordSource is where an update's new entry password comes from, if any.
type passwordSource int

const (
	keepPassword passwordSource = iota
	promptPassword
	generatePassword
)

func newUpdateCommand(d Deps, locator vaultLocator, secrets *secretSource) *cobra.Command {
	var opts updateOptions
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Change an entry's fields or password; only the flags given are changed",
		Args:  cobra.ExactArgs(1),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			name, err := domain.NewName(args[0])
			if err != nil {
				return usageError{err: err}
			}
			changes, err := opts.fieldChanges(cmd.Flags())
			if err != nil {
				return usageError{err: err}
			}
			source, policy, err := opts.passwordSource(cmd.Flags())
			if err != nil {
				return usageError{err: err}
			}
			if source == keepPassword && changes.IsEmpty() {
				return usageError{err: fmt.Errorf("%w; give a field flag, --%s or --%s",
					app.ErrNoChanges, flagChangePassword, flagGenerate)}
			}
			path, err := locator.resolve()
			if err != nil {
				return err
			}
			mp, err := secrets.master()
			if err != nil {
				return err
			}
			defer mp.Wipe()
			// Unlock and find the entry before asking for or generating the new
			// password, so a wrong master password, a missing vault or a missing
			// entry fails before the user types more.
			sess, err := app.NewService(d.NewStore(path), d.Clock).Open(cmd.Context(), mp)
			if err != nil {
				return entryFailure(err, path, name)
			}
			if _, err := sess.Find(name); err != nil {
				return entryFailure(err, path, name)
			}
			pw, err := newPassword(d, secrets, source, policy)
			if err != nil {
				return err
			}
			if source != keepPassword {
				changes.Password = &pw
			}
			stored, err := runUpdate(cmd.Context(), d.Streams.Out, sess, path, name, changes)
			if err != nil {
				return err
			}
			if source == generatePassword {
				return reportGenerated(d.Streams, stored, policy)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&opts.kind, flagKind, "", "new entry kind: site, card, note or other")
	cmd.Flags().StringVar(&opts.username, flagUsername, "", "new user name; empty clears it")
	cmd.Flags().StringVar(&opts.url, flagURL, "", "new address; empty clears it")
	cmd.Flags().StringVar(&opts.notes, flagNotes, "", "new notes; empty clears them")
	cmd.Flags().StringVar(&opts.expires, flagExpires, "", "new expiry date, YYYY-MM-DD; empty clears it")
	cmd.Flags().BoolVar(&opts.changePassword, flagChangePassword, false, "prompt for a new entry password")
	opts.generation.register(cmd.Flags())
	return cmd
}

// fieldChanges turns only the flags the user set into changes, so an omitted
// flag keeps the stored value while an explicit empty value clears it.
func (o updateOptions) fieldChanges(fs *pflag.FlagSet) (app.EntryChanges, error) {
	var c app.EntryChanges
	if fs.Changed(flagKind) {
		kind, err := domain.ParseKind(o.kind)
		if err != nil {
			return app.EntryChanges{}, err
		}
		c.Kind = &kind
	}
	if fs.Changed(flagExpires) {
		expires, err := domain.ParseDate(o.expires)
		if err != nil {
			return app.EntryChanges{}, err
		}
		c.ExpiresOn = &expires
	}
	if fs.Changed(flagUsername) {
		c.Username = &o.username
	}
	if fs.Changed(flagURL) {
		c.URL = &o.url
	}
	if fs.Changed(flagNotes) {
		c.Notes = &o.notes
	}
	return c, nil
}

func (o updateOptions) passwordSource(fs *pflag.FlagSet) (passwordSource, passgen.Policy, error) {
	policy, generating, err := o.generation.policy(fs)
	if err != nil {
		return keepPassword, passgen.Policy{}, err
	}
	switch {
	case generating && o.changePassword:
		return keepPassword, passgen.Policy{}, errPasswordSourceConflict
	case generating:
		return generatePassword, policy, nil
	case o.changePassword:
		return promptPassword, passgen.Policy{}, nil
	}
	return keepPassword, passgen.Policy{}, nil
}

// newPassword reads or generates the replacement entry password; it returns
// the zero Password when the password is kept.
func newPassword(d Deps, secrets *secretSource, source passwordSource, policy passgen.Policy) (domain.Password, error) {
	switch source {
	case promptPassword:
		return secrets.newEntryPassword()
	case generatePassword:
		return passgen.New(d.Random).Generate(policy)
	case keepPassword:
		return domain.Password{}, nil
	}
	return domain.Password{}, errors.New("unknown password source")
}

func runUpdate(ctx context.Context, out io.Writer, sess *app.Session, path string, name domain.Name, changes app.EntryChanges) (domain.Name, error) {
	e, err := sess.Update(ctx, name, changes)
	if err != nil {
		return domain.Name{}, entryFailure(err, path, name)
	}
	if _, err := fmt.Fprintf(out, "Updated %q\n", e.Name().String()); err != nil {
		return domain.Name{}, fmt.Errorf("write result: %w", err)
	}
	return e.Name(), nil
}
