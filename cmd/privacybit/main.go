// Command privacybit audits the privacy posture of a Linux desktop.
//
// The catalogue is the only thing this command reads in phase 1. Detection and
// grading arrive in phase 2, so list and explain read a table rather than the
// machine. That ordering is deliberate (D6 §13): the three-state
// Observed|Unavailable|Contradicted invariant has to exist before the first
// grade is computed, and retrofitting it would mean auditing every signal the
// engine had already emitted.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/engine"
	"github.com/Nomadcxx/privacybit/internal/report"
	"github.com/Nomadcxx/privacybit/internal/tui"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		// SilenceErrors is set so that this one write controls how a failure
		// reaches the user. Cobra would otherwise print it, and a printed
		// failure with no exit code is a failure that gets ignored.
		fmt.Fprintln(os.Stderr, "privacybit: "+err.Error())
		os.Exit(exitCode(err))
	}
}

type commandError struct {
	code int
	err  error
}

func (e commandError) Error() string { return e.err.Error() }
func (e commandError) Unwrap() error { return e.err }

func exitCode(err error) int {
	var coded commandError
	if errors.As(err, &coded) {
		return coded.code
	}
	return 1
}

func newRootCommand() *cobra.Command {
	var userFragments bool

	root := &cobra.Command{
		Use:   "privacybit",
		Short: "Audit the privacy posture of this machine",
		// SilenceUsage keeps a runtime failure from dumping the whole flag set
		// under a message the user cannot act on. Usage errors still print it.
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: "privacybit checks installed applications and, when a VPN tunnel can be\n" +
			"verified, inspects its routing and resolver state using local system data.\n" +
			"It reports a graded breakdown by category with a stated reason for each\n" +
			"judgement.\n\n" +
			"It grades privacy. Privacy, security and anonymity are different\n" +
			"properties and are not interchangeable.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().BoolVar(&userFragments, "user-catalog", true,
		"merge fragments from ~/.config/privacybit/catalog.d")

	root.AddCommand(newListCommand(&userFragments))
	root.AddCommand(newExplainCommand(&userFragments))
	root.AddCommand(newAuditCommand(&userFragments, engine.Run))
	root.AddCommand(newTUICommand(&userFragments))
	root.AddCommand(newDaemonCommand(&userFragments))
	root.AddCommand(newScanCommand(&userFragments, engine.Run, report.UserStore))
	root.AddCommand(newReportCommand(&userFragments, engine.Run, report.UserStore, tui.RunWithSnapshot))
	root.AddCommand(newPurgeCommand(&userFragments))
	root.AddCommand(newCatalogCommand(&userFragments))

	return root
}

// loadCatalogue is the single path every subcommand uses, so the flag handling
// and the error wording cannot drift between them.
func loadCatalogue(userFragments *bool) (*catalog.Set, error) {
	opts := catalog.LoadOptions{UserFragments: *userFragments}
	if *userFragments {
		dir, err := catalog.UserFragmentDir()
		if err == nil {
			opts.Dir = dir
		}
	}
	set, err := catalog.Load(opts)
	if err != nil {
		return nil, fmt.Errorf("load catalogue: %w", err)
	}
	return set, nil
}

// gradeCell renders a grade on all three channels: colour, glyph and word.
// Two of the three fail a colourblind reader, so a caller that renders any
// two of them has told the user nothing (D1 §8, D2 §8).
//
// Colour is applied only when the output is a terminal and NO_COLOR is unset.
// The glyph and word are what remain on a pipe, in a log, and for anyone the
// colour channel fails.
func gradeCell(g catalog.Grade) string {
	style := report.GradeChannels(g)
	if !useColour {
		return fmt.Sprintf("%s %s", style.ASCII, style.Word)
	}
	if style.ANSI == 0 {
		return fmt.Sprintf("%s %s", style.Glyph, style.Word)
	}
	return fmt.Sprintf("\x1b[%dm%s %s\x1b[0m", style.ANSI, style.Glyph, style.Word)
}
