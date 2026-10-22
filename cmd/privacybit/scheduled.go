package main

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/daemon"
	"github.com/Nomadcxx/privacybit/internal/detect"
	"github.com/Nomadcxx/privacybit/internal/engine"
	"github.com/Nomadcxx/privacybit/internal/report"
	"github.com/Nomadcxx/privacybit/internal/tui"
	"github.com/spf13/cobra"
)

type storeFactory func() (report.Store, error)

type storedTUIRunner func(context.Context, io.Reader, io.Writer, report.Snapshot, tui.ScanFunc) error

func newDaemonCommand(userFragments *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Watch for newly installed red-rated applications",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			set, err := loadCatalogue(userFragments)
			if err != nil {
				return commandError{code: 2, err: err}
			}
			runner := detect.ExecRunner{}
			watcher, err := daemon.NewWatcher(set, []daemon.SourceSpec{
				{Type: catalog.DetectPackage, Source: detect.NewPacman(runner, nil)},
				{Type: catalog.DetectPackage, Source: detect.NewDpkg(runner)},
				{Type: catalog.DetectFlatpak, Source: detect.NewFlatpak(runner)},
			}, daemon.NotifyDesktop)
			if err != nil {
				return commandError{code: 2, err: fmt.Errorf("start inventory watcher: %w", err)}
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			if err := watcher.Run(ctx); err != nil {
				return commandError{code: 2, err: err}
			}
			return nil
		},
	}
}

func newScanCommand(userFragments *bool, scan scanFunc, makeStore storeFactory) *cobra.Command {
	if scan == nil {
		scan = engine.Run
	}
	if makeStore == nil {
		makeStore = report.UserStore
	}
	return &cobra.Command{
		Use:   "scan",
		Short: "Run a read-only audit and save it for later",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			set, err := loadCatalogue(userFragments)
			if err != nil {
				return commandError{code: 2, err: err}
			}
			store, err := makeStore()
			if err != nil {
				return commandError{code: 3, err: fmt.Errorf("locate report store: %w", err)}
			}
			release, err := store.Lock()
			if err != nil {
				return commandError{code: 3, err: fmt.Errorf("lock report state: %w", err)}
			}
			defer release()
			snapshot, err := scan(cmd.Context(), set, engine.Options{Network: true})
			if err != nil {
				return commandError{code: 2, err: fmt.Errorf("audit: %w", err)}
			}
			if !hasCoverage(snapshot) {
				return commandError{code: 2, err: fmt.Errorf("no detection source produced coverage")}
			}
			if err := store.Save(snapshot, true); err != nil {
				return commandError{code: 3, err: fmt.Errorf("save report: %w", err)}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved unread report: %s (%s)\n",
				snapshot.GeneratedAt.Local().Format("15:04:05, 02 Jan 2006"), snapshot.Overall.Grade)
			return nil
		},
	}
}

func newReportCommand(userFragments *bool, scan scanFunc, makeStore storeFactory, runUI storedTUIRunner) *cobra.Command {
	if scan == nil {
		scan = engine.Run
	}
	if makeStore == nil {
		makeStore = report.UserStore
	}
	if runUI == nil {
		runUI = tui.RunWithSnapshot
	}
	return &cobra.Command{
		Use:   "report",
		Short: "Open the most recently saved audit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := makeStore()
			if err != nil {
				return commandError{code: 3, err: fmt.Errorf("locate report store: %w", err)}
			}
			snapshot, err := store.Load()
			if err != nil {
				return commandError{code: 2, err: err}
			}
			freshScan := func(ctx context.Context) (report.Snapshot, error) {
				set, err := loadCatalogue(userFragments)
				if err != nil {
					return report.Snapshot{}, err
				}
				release, err := store.Lock()
				if err != nil {
					return report.Snapshot{}, fmt.Errorf("lock report state: %w", err)
				}
				defer release()
				fresh, err := scan(ctx, set, engine.Options{Network: true})
				if err != nil {
					return report.Snapshot{}, err
				}
				if !hasCoverage(fresh) {
					return report.Snapshot{}, fmt.Errorf("no detection source produced coverage")
				}
				if err := store.Save(fresh, false); err != nil {
					return report.Snapshot{}, fmt.Errorf("save refreshed report: %w", err)
				}
				return fresh, nil
			}
			if err := runUI(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), *snapshot, freshScan); err != nil {
				return commandError{code: 3, err: fmt.Errorf("open report: %w", err)}
			}
			if err := store.MarkRead(); err != nil {
				return commandError{code: 3, err: fmt.Errorf("mark report read: %w", err)}
			}
			return nil
		},
	}
}
