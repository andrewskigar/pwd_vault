package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

type addOptions struct {
	kind     string
	username string
	url      string
	notes    string
	expires  string
}

func newAddCommand(d Deps, locator vaultLocator, secrets *secretSource) *cobra.Command {
	var opts addOptions
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add an entry; its password is prompted, never taken from flags",
		Args:  cobra.ExactArgs(1),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			draft, err := opts.draft(args[0])
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
			if draft.Password, err = secrets.newEntryPassword(); err != nil {
				return err
			}
			return runAdd(cmd, d, path, mp, draft)
		}),
	}
	cmd.Flags().StringVar(&opts.kind, "kind", string(domain.KindSite), "entry kind: site, card, note or other")
	cmd.Flags().StringVar(&opts.username, "username", "", "user name or login")
	cmd.Flags().StringVar(&opts.url, "url", "", "address of the resource")
	cmd.Flags().StringVar(&opts.notes, "notes", "", "free-form notes")
	cmd.Flags().StringVar(&opts.expires, "expires", "", "expiry date, YYYY-MM-DD")
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

func runAdd(cmd *cobra.Command, d Deps, path string, mp app.MasterPassword, draft app.EntryDraft) error {
	svc := app.NewService(d.NewStore(path), d.Clock)
	e, err := svc.Add(cmd.Context(), mp, draft)
	if err != nil {
		return entryFailure(err, path, draft.Name)
	}
	if _, err := fmt.Fprintf(d.Streams.Out, "Added %q\n", e.Name().String()); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}
