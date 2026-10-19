package main

import (
	"fmt"
	"strings"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/spf13/cobra"
)

func newListCommand(userFragments *bool) *cobra.Command {
	var (
		categoryFlag string
		gradeFlag    string
		jsonOut      bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the catalogue by category",
		Long: "List every graded entry privacybit knows about, grouped by category.\n\n" +
			"This reads the catalogue, not the machine. It answers \"what does\n" +
			"privacybit have an opinion on\", which is a different question from\n" +
			"\"what is installed\" — that is what the audit is for.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadCatalogue(userFragments)
			if err != nil {
				return err
			}

			want := catalog.Grade(strings.ToLower(strings.TrimSpace(gradeFlag)))
			if want != "" && !want.Valid() {
				return fmt.Errorf("unknown grade %q: use green, amber or red", gradeFlag)
			}
			// An unknown category is an error rather than an empty result.
			// Silence here would be indistinguishable from "this category is
			// all green", which is the same conflation D5 §5.4 forbids in the
			// panel: nothing shown has to read as something, never as a pass.
			if categoryFlag != "" && set.Category(categoryFlag) == nil {
				return fmt.Errorf("unknown category %q; run `privacybit list` to see the table", categoryFlag)
			}

			cmd.SilenceUsage = true

			for _, c := range set.Categories {
				if categoryFlag != "" && c.ID != categoryFlag {
					continue
				}
				rows := make([]*catalog.Entry, 0, len(c.Entries))
				for i := range c.Entries {
					if want != "" && c.Entries[i].Grade != want {
						continue
					}
					rows = append(rows, &c.Entries[i])
				}
				if len(rows) == 0 {
					continue
				}

				if jsonOut {
					// JSON goes to stdout alone so it stays pipeable; the
					// human view below is suppressed rather than interleaved.
					continue
				}

				fmt.Fprintf(cmd.OutOrStdout(), "\n%s  %s\n", c.Name, dim(c.Summary))
				for _, e := range rows {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s  %-22s %s\n",
						gradeCell(e.Grade), e.ID, e.Name)
				}
			}

			if jsonOut {
				return emitEntriesJSON(set, categoryFlag, want)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", dim("catalogue revision "+set.Revision))
			return nil
		},
	}

	cmd.Flags().StringVar(&categoryFlag, "category", "", "show only this category id")
	cmd.Flags().StringVar(&gradeFlag, "grade", "", "show only entries with this grade")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit entries as JSON on stdout")
	return cmd
}

// listEntryJSON is deliberately flatter than the snapshot shape. list answers a
// catalogue question, and borrowing the snapshot's nesting here would couple two
// contracts that change for different reasons.
type listEntryJSON struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	Grade      string   `json:"grade"`
	Word       string   `json:"word"`
	Glyph      string   `json:"glyph"`
	Family     string   `json:"family,omitempty"`
	Summary    string   `json:"summary"`
	Reasons    []string `json:"reasons,omitempty"`
	Caveats    []string `json:"caveats,omitempty"`
	Overridden bool     `json:"overridden,omitempty"`
}

func emitEntriesJSON(set *catalog.Set, categoryFilter string, gradeFilter catalog.Grade) error {
	out := make([]listEntryJSON, 0, len(set.Categories)*4)
	for _, c := range set.Categories {
		if categoryFilter != "" && c.ID != categoryFilter {
			continue
		}
		for i := range c.Entries {
			e := &c.Entries[i]
			if gradeFilter != "" && e.Grade != gradeFilter {
				continue
			}
			out = append(out, listEntryJSON{
				ID:         e.ID,
				Name:       e.Name,
				Category:   c.ID,
				Grade:      string(e.Grade),
				Word:       e.Grade.Word(),
				Glyph:      e.Grade.Glyph(),
				Family:     e.Family,
				Summary:    e.Summary,
				Reasons:    e.Reasons,
				Caveats:    e.Caveats,
				Overridden: e.Overridden,
			})
		}
	}
	return writeJSON(cmdStdout(), out)
}

// dim is secondary text — a category summary, a hint, a note. It carries no
// meaning of its own, so it drops entirely when colour is off rather than
// becoming indistinguishable from the primary text beside it.
func dim(s string) string {
	if !useColour {
		return s
	}
	return "\x1b[2m" + s + "\x1b[0m"
}
