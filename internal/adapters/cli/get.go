package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

type getOptions struct {
	show bool
}

func newGetCommand(d Deps, locator vaultLocator, secrets *secretSource) *cobra.Command {
	var opts getOptions
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Print an entry, with the password masked unless --show",
		Args:  cobra.ExactArgs(1),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			name, err := domain.NewName(args[0])
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
			return runGet(cmd, d, path, mp, name, opts)
		}),
	}
	cmd.Flags().BoolVar(&opts.show, "show", false, "print the password in clear text")
	return cmd
}

func runGet(cmd *cobra.Command, d Deps, path string, mp app.MasterPassword, name domain.Name, opts getOptions) error {
	svc := app.NewService(d.NewStore(path), d.Clock)
	e, err := svc.Get(cmd.Context(), mp, name)
	if err != nil {
		return entryFailure(err, path, name)
	}
	if err := writeEntry(d.Streams.Out, e, opts.passwordText(e.Password())); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	return nil
}

// passwordText is the Password line's value: the mask unless --show.
func (o getOptions) passwordText(pw domain.Password) string {
	if o.show {
		return pw.Reveal()
	}
	return pw.String()
}

// writeEntry prints the spec's key/value block with labels padded to 10
// columns and timestamps in RFC 3339 UTC.
func writeEntry(w io.Writer, e domain.Entry, password string) error {
	rows := [][2]string{
		{"Name:", e.Name().String()},
		{"Kind:", string(e.Kind())},
		{"Username:", e.Username()},
		{"Password:", password},
		{"URL:", e.URL()},
		{"Notes:", e.Notes()},
		{"Created:", e.CreatedAt().UTC().Format(time.RFC3339)},
		{"Updated:", e.UpdatedAt().UTC().Format(time.RFC3339)},
		{"Expires:", e.ExpiresOn().String()},
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "%-10s%s\n", r[0], r[1]); err != nil {
			return err
		}
	}
	return nil
}
