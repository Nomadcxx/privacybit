package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/render"
	"github.com/spf13/cobra"
)

func newExplainCommand(userFragments *bool) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "explain <entry-id>",
		Short: "Explain why an entry has the grade it has",
		Long: "Print the reasoning behind one catalogue entry: its grade, why, what\n" +
			"it does well, what it does not, and what to use instead.\n\n" +
			"Every grade in this tool is an opinion held in a table, and an\n" +
			"opinion you cannot interrogate is not worth much. This is the command\n" +
			"that makes it interrogable.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadCatalogue(userFragments)
			if err != nil {
				return err
			}

			id := strings.TrimSpace(args[0])
			entry := set.Entry(id)
			if entry == nil {
				return fmt.Errorf("no catalogue entry %q; run `privacybit list` to see the table", id)
			}

			if jsonOut {
				return writeJSON(os.Stdout, explainJSON(set, entry))
			}

			out := cmd.OutOrStdout()
			cmd.SilenceUsage = true

			// The header uses the same gradeCell as the alternatives list below it. Two
			// renderings of one grade on one screen is a rendering bug waiting to
			// be misread as two different grades.
			fmt.Fprintf(out, "%s  %s\n", gradeCell(entry.Grade), entry.ID)
			fmt.Fprintf(out, "%s\n", entry.Name)
			if entry.Category != "" {
				fmt.Fprintf(out, "%s\n", dim("category: "+entry.Category))
			}
			if entry.Family != "" {
				fmt.Fprintf(out, "%s\n", dim("family: "+entry.Family))
			}
			if entry.Overridden {
				// D1 §5.1: an override changes a label, and the label it
				// replaced stays visible so the effect is never silent.
				fmt.Fprintf(out, "%s\n", dim("overridden locally; shipped grade was "+string(entry.CatalogueGrade)))
			}

			fmt.Fprintf(out, "\n%s\n", entry.Summary)

			printSection(out, "Why this grade", entry.Reasons)
			printSection(out, "Caveats", entry.Caveats)

			if len(entry.Alternatives) > 0 {
				fmt.Fprintf(out, "\n%s\n", "Alternatives")
				for _, alt := range entry.Alternatives {
					a := set.Entry(alt)
					if a == nil {
						fmt.Fprintf(out, "  - %s %s\n", "?", alt)
						continue
					}
					fmt.Fprintf(out, "  %s %-22s %s\n", gradeCell(a.Grade), a.ID, a.Summary)
				}
			}

			printInstalls(out, set, entry)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the explanation as JSON on stdout")
	return cmd
}

func printSection(out io.Writer, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s\n", title)
	for _, l := range lines {
		fmt.Fprintf(out, "  - %s\n", l)
	}
}

type explainJSONOut struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Category       string               `json:"category"`
	Family         string               `json:"family,omitempty"`
	Grade          string               `json:"grade"`
	Word           string               `json:"word"`
	Glyph          string               `json:"glyph"`
	CatalogueGrade string               `json:"catalogue_grade,omitempty"`
	Overridden     bool                 `json:"overridden,omitempty"`
	Summary        string               `json:"summary"`
	Reasons        []string             `json:"reasons,omitempty"`
	Caveats        []string             `json:"caveats,omitempty"`
	Alternatives   []explainAltJSON     `json:"alternatives,omitempty"`
	Installs       []explainInstallJSON `json:"installs,omitempty"`
}

type explainAltJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Grade   string `json:"grade"`
	Summary string `json:"summary,omitempty"`
}

type explainInstallJSON struct {
	Distro   string `json:"distro,omitempty"`
	Command  string `json:"command,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Note     string `json:"note,omitempty"`
}

func explainJSON(set *catalog.Set, e *catalog.Entry) explainJSONOut {
	out := explainJSONOut{
		ID: e.ID, Name: e.Name, Category: e.Category, Family: e.Family,
		Grade: string(e.Grade), Word: e.Grade.Word(), Glyph: e.Grade.Glyph(),
		CatalogueGrade: string(e.CatalogueGrade), Overridden: e.Overridden,
		Summary: e.Summary, Reasons: e.Reasons, Caveats: e.Caveats,
	}
	for _, alt := range e.Alternatives {
		a := set.Entry(alt)
		if a == nil {
			continue
		}
		out.Alternatives = append(out.Alternatives, explainAltJSON{
			ID: a.ID, Name: a.Name, Grade: string(a.Grade), Summary: a.Summary,
		})
	}
	out.Installs = installRoutes(e)
	return out
}

func printInstalls(out io.Writer, set *catalog.Set, e *catalog.Entry) {
	routes := installRoutes(e)
	if len(routes) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s\n", "Install")
	for _, r := range routes {
		switch {
		case r.Command != "":
			fmt.Fprintf(out, "  %-8s %s\n", r.Distro, r.Command)
		case r.Upstream != "":
			fmt.Fprintf(out, "  %-8s not packaged — %s\n", r.Distro, r.Upstream)
		}
		if r.Note != "" {
			fmt.Fprintf(out, "  %s\n", dim(r.Note))
		}
	}
}

// installRoutes resolves an entry's install block per distribution. D6 §8's
// three outcomes matter here: a command, an honest reason why there isn't one,
// or silence. The third is load-bearing — emitting a command that cannot run on
// the user's distribution teaches them the recommendations are untested.
func installRoutes(e *catalog.Entry) []explainInstallJSON {
	if e.Install == nil || len(e.Install.Package) == 0 {
		return nil
	}
	distros := make([]string, 0, len(e.Install.Package))
	for d := range e.Install.Package {
		distros = append(distros, d)
	}
	sort.Strings(distros)

	out := make([]explainInstallJSON, 0, len(distros))
	for _, d := range distros {
		instruction := render.Install(d, e)
		out = append(out, explainInstallJSON{
			Distro:   d,
			Command:  instruction.Command,
			Upstream: e.Install.Upstream,
			Note:     instruction.Reason,
		})
	}
	return out
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
