package main

import (
	"context"
	"fmt"

	"github.com/Nomadcxx/privacybit/internal/engine"
	"github.com/Nomadcxx/privacybit/internal/report"
	"github.com/Nomadcxx/privacybit/internal/tui"
	"github.com/spf13/cobra"
)

func newTUICommand(userFragments *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Browse an audit in a full-screen terminal interface",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			set, err := loadCatalogue(userFragments)
			if err != nil {
				return commandError{code: 2, err: err}
			}
			scan := func(ctx context.Context) (report.Snapshot, error) {
				return engine.Run(ctx, set, engine.Options{Network: true})
			}
			if err := tui.Run(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), scan); err != nil {
				return commandError{code: 3, err: fmt.Errorf("run TUI: %w", err)}
			}
			return nil
		},
	}
}
