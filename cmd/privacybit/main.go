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
	"fmt"
	"os"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		// SilenceErrors is set so that this one write controls how a failure
		// reaches the user. Cobra would otherwise print it, and a printed
		// failure with no exit code is a failure that gets ignored.
		fmt.Fprintln(os.Stderr, "privacybit: "+err.Error())
		os.Exit(1)
	}
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
		Long: "privacybit inspects what is installed, what is running, and what is\n" +
			"actually going over the network, then reports a graded breakdown by\n" +
			"category with a stated reason for every grade.\n\n" +
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
	if !useColour {
		return fmt.Sprintf("%s %s", g.ASCII(), g.Word())
	}
	return fmt.Sprintf("\x1b[%dm%s %s\x1b[0m", ansiFor(g), g.Glyph(), g.Word())
}

// ansiFor maps a grade to its colour escape.
//
// Unknown is deliberately given no colour of its own. An item nobody has graded
// has no opinion to express, and a fourth hue would compete with the three that
// carry meaning. It renders in the terminal's default text instead.
func ansiFor(g catalog.Grade) int {
	switch g {
	case catalog.GradeGreen:
		return 32
	case catalog.GradeAmber:
		// 33 is the amber family the shell host already paints for ToneActivity.
		return 33
	case catalog.GradeRed:
		return 31
	default:
		return 0
	}
}
