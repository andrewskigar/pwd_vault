package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
	"save_pwd/internal/passgen"
)

type addOptions struct {
	kind       string
	username   string
	url        string
	notes      string
	expires    string
	generation optionalGeneration
}

func newAddCommand(d Deps, locator vaultLocator, secrets *secretSource) *cobra.Command {
	var opts addOptions
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add an entry; its password is prompted or generated, never taken from flags",
		Args:  cobra.ExactArgs(1),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			draft, err := opts.draft(args[0])
			if err != nil {
				return usageError{err: err}
			}
			policy, generating, err := opts.generation.policy(cmd.Flags())
			if err != nil {
				return usageError{err: err}
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
			// Unlock before asking for the entry password, so a wrong master
			// password or a missing vault fails before the user types more.
			sess, err := app.NewService(d.NewStore(path), d.Clock).Open(cmd.Context(), mp)
			if err != nil {
				return entryFailure(err, path, draft.Name)
			}
			if generating {
				draft.Password, err = passgen.New(d.Random).Generate(policy)
			} else {
				draft.Password, err = secrets.newEntryPassword()
			}
			if err != nil {
				return err
			}
			if err := runAdd(cmd.Context(), d.Streams.Out, sess, path, draft); err != nil {
				return err
			}
			if generating {
				return reportGenerated(d.Streams, draft.Name, policy)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&opts.kind, "kind", string(domain.KindSite), "entry kind: site, card, note or other")
	cmd.Flags().StringVar(&opts.username, "username", "", "user name or login")
	cmd.Flags().StringVar(&opts.url, "url", "", "address of the resource")
	cmd.Flags().StringVar(&opts.notes, "notes", "", "free-form notes")
	cmd.Flags().StringVar(&opts.expires, "expires", "", "expiry date, YYYY-MM-DD")
	opts.generation.register(cmd.Flags())
	return cmd
}

// draft converts the argument and flags to domain types, so invalid input is
// a usage error raised before any prompt or file access.
func (o addOptions) draft(rawName string) (app.EntryDraft, error) {
	name, err := domain.NewName(rawName)
	if err != nil {
		return app.EntryDraft{}, err
	}
	kind, err := domain.ParseKind(o.kind)
	if err != nil {
		return app.EntryDraft{}, err
	}
	expires, err := domain.ParseDate(o.expires)
	if err != nil {
		return app.EntryDraft{}, err
	}
	return app.EntryDraft{
		Name:      name,
		Kind:      kind,
		Username:  o.username,
		URL:       o.url,
		Notes:     o.notes,
		ExpiresOn: expires,
	}, nil
}

func runAdd(ctx context.Context, out io.Writer, sess *app.Session, path string, draft app.EntryDraft) error {
	e, err := sess.Add(ctx, draft)
	if err != nil {
		return entryFailure(err, path, draft.Name)
	}
	if _, err := fmt.Fprintf(out, "Added %q\n", e.Name().String()); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}

// reportGenerated tells the user how to view a password they never saw; the
// password itself is never printed.
func reportGenerated(s Streams, name domain.Name, policy passgen.Policy) error {
	if _, err := fmt.Fprintf(s.Out, "(generated password; view it with: %s)\n", showCommand(name.String())); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	warnIfWeak(s.Err, policy)
	return nil
}

// showCommand is the get command that reveals name. A name starting with '-'
// goes after "--" so get does not parse it as a flag.
func showCommand(name string) string {
	if strings.HasPrefix(name, "-") {
		return programName + " get --show -- " + shellQuote(name)
	}
	return programName + " get " + shellQuote(name) + " --show"
}
