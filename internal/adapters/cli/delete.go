package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"save_pwd/internal/app"
	"save_pwd/internal/domain"
)

var errDeleteAborted = errors.New("deletion aborted")

type deleteOptions struct {
	yes bool
}

func newDeleteCommand(d Deps, locator vaultLocator, secrets *secretSource) *cobra.Command {
	var opts deleteOptions
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an entry after a y/N confirmation, or at once with --yes",
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
			return runDelete(cmd, d, secrets, path, mp, name, opts)
		}),
	}
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "delete without asking for confirmation")
	return cmd
}

func runDelete(cmd *cobra.Command, d Deps, secrets *secretSource, path string, mp app.MasterPassword, name domain.Name, opts deleteOptions) error {
	// Unlocking and finding the entry first means a wrong password or a typo
	// in the name fails before the user is asked to confirm anything. One
	// session serves the whole command: one Load, and one Save if confirmed.
	sess, err := app.NewService(d.NewStore(path), d.Clock).Open(cmd.Context(), mp)
	if err != nil {
		return entryFailure(err, path, name)
	}
	e, err := sess.Find(name)
	if err != nil {
		return entryFailure(err, path, name)
	}
	stored := e.Name().String()
	if !opts.yes {
		if err := confirmDeletion(d.Streams.Err, secrets, stored); err != nil {
			return err
		}
	}
	if err := sess.Delete(cmd.Context(), name); err != nil {
		return entryFailure(err, path, name)
	}
	if _, err := fmt.Fprintf(d.Streams.Out, "Deleted %q\n", stored); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}

// confirmDeletion asks on stderr, so stdout stays clean for scripts, and
// returns errDeleteAborted for anything but an explicit yes, including EOF.
func confirmDeletion(w io.Writer, secrets *secretSource, name string) error {
	if _, err := fmt.Fprintf(w, "Delete entry %q? [y/N] ", name); err != nil {
		return fmt.Errorf("write confirmation question: %w", err)
	}
	answer, err := secrets.answer()
	if err != nil {
		return err
	}
	if !isYes(answer) {
		return errDeleteAborted
	}
	return nil
}

func isYes(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}
