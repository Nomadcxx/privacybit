package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newCatalogCommand(userFragments *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Inspect the catalogue itself",
	}
	cmd.AddCommand(newCatalogValidateCommand(userFragments))
	cmd.AddCommand(newCatalogPathCommand())
	return cmd
}

func newCatalogValidateCommand(userFragments *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Load the catalogue and report every problem at once",
		Long: "Parse and validate the shipped catalogue and any user fragments.\n\n" +
			"Every problem is reported in one pass rather than stopping at the\n" +
			"first, because the table is expected to be broken while it is being\n" +
			"authored and fixing one error per run across ten files is slow.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadCatalogue(userFragments)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				os.Exit(2)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ok  %s\n", set.Revision)
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", dim("no problems found"))
			return nil
		},
	}
}

func newCatalogPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print where the catalogue is read from",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := userFragmentPath()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", dir)
			return nil
		},
	}
}
