package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"save_pwd/internal/app"
)

func newInitCommand(d Deps, locator vaultLocator, secrets *secretSource) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a new encrypted vault",
		Args:  cobra.NoArgs,
		RunE: action(func(cmd *cobra.Command, _ []string) error {
			path, err := locator.resolve()
			if err != nil {
				return err
			}
			mp, err := secrets.newMaster()
			if err != nil {
				return err
			}
			defer mp.Wipe()
			return runInit(cmd, d, path, mp)
		}),
	}
}

func runInit(cmd *cobra.Command, d Deps, path string, mp app.MasterPassword) error {
	svc := app.NewService(d.NewStore(path), d.Clock)
	err := svc.Init(cmd.Context(), mp)
	if errors.Is(err, app.ErrVaultExists) {
		return fmt.Errorf("vault already exists at %s", path)
	}
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(d.Streams.Out, "Created vault at %s\n", path); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}
