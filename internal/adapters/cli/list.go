package cli

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

const (
	noEntries = "No entries."
	// updatedLayout shows only the day of the last change; the full
	// timestamp is in get.
	updatedLayout = "2006-01-02"
)

type listOptions struct {
	kind string
}

func newListCommand(d Deps, locator vaultLocator, secrets *secretSource) *cobra.Command {
	var opts listOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entries as a table; passwords are never shown",
		Args:  cobra.NoArgs,
		RunE: action(func(cmd *cobra.Command, _ []string) error {
			filter, err := opts.filter()
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
			return runList(cmd, d, path, mp, filter)
		}),
	}
	cmd.Flags().StringVar(&opts.kind, "kind", "", "only list entries of this kind: site, card, note or other")
	return cmd
}

// filter converts --kind to a domain kind before any prompt or file access;
// an empty value lists every kind.
func (o listOptions) filter() (app.ListFilter, error) {
	if o.kind == "" {
		return app.ListFilter{}, nil
	}
	kind, err := domain.ParseKind(o.kind)
	if err != nil {
		return app.ListFilter{}, err
	}
	return app.ListFilter{Kind: kind}, nil
}

func runList(cmd *cobra.Command, d Deps, path string, mp app.MasterPassword, filter app.ListFilter) error {
	svc := app.NewService(d.NewStore(path), d.Clock)
	entries, err := svc.List(cmd.Context(), mp, filter)
	if err != nil {
		return vaultFailure(err, path)
	}
	// The notice goes to stderr so a pipe reading stdout sees an empty table.
	if len(entries) == 0 {
		if _, err := fmt.Fprintln(d.Streams.Err, noEntries); err != nil {
			return fmt.Errorf("write notice: %w", err)
		}
		return nil
	}
	if err := writeTable(d.Streams.Out, entries); err != nil {
		return fmt.Errorf("write table: %w", err)
	}
	return nil
}

// writeTable prints the entries under a NAME KIND USERNAME UPDATED EXPIRES
// header. It deliberately has no password column.
func writeTable(w io.Writer, entries []domain.Entry) error {
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tKIND\tUSERNAME\tUPDATED\tEXPIRES")
	for _, e := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			e.Name(), e.Kind(), e.Username(), e.UpdatedAt().UTC().Format(updatedLayout), e.ExpiresOn())
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := io.WriteString(w, trimTrailingSpace(buf.String()))
	return err
}

// trimTrailingSpace drops the padding tabwriter leaves after an empty last
// cell, such as a blank expiry.
func trimTrailingSpace(table string) string {
	lines := strings.Split(table, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}
