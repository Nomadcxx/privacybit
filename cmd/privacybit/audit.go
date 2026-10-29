package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/engine"
	"github.com/Nomadcxx/privacybit/internal/report"
	"github.com/spf13/cobra"
)

type scanFunc func(context.Context, *catalog.Set, engine.Options) (report.Snapshot, error)

type terminalEvent struct {
	Type     string           `json:"t"`
	Schema   int              `json:"schema,omitempty"`
	Mode     string           `json:"mode,omitempty"`
	Grade    catalog.Grade    `json:"grade,omitempty"`
	Score    *int             `json:"score,omitempty"`
	Snapshot *report.Snapshot `json:"snapshot,omitempty"`
	Error    string           `json:"error,omitempty"`
}

func newAuditCommand(userFragments *bool, scan scanFunc) *cobra.Command {
	var jsonOutput, snapshotOnly, networkEnabled, noNetwork bool
	var categories, excluded []string
	var severity, failOn string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Run a point-in-time privacy audit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if severity == "" {
				severity = "low"
			}
			if !validSeverity(severity) {
				return commandError{code: 1, err: fmt.Errorf("unknown severity %q (want low, medium, or high)", severity)}
			}
			threshold, useThreshold := failThreshold(failOn)
			if failOn != "" && !useThreshold {
				return commandError{code: 1, err: fmt.Errorf("unknown --fail-on grade %q", failOn)}
			}
			set, err := loadCatalogue(userFragments)
			if err != nil {
				if jsonOutput || snapshotOnly {
					if writeErr := writeEvent(cmd.OutOrStdout(), terminalEvent{Type: "error", Error: err.Error()}); writeErr != nil {
						return commandError{code: 3, err: writeErr}
					}
				}
				return commandError{code: 2, err: err}
			}
			for _, id := range append(append([]string(nil), categories...), excluded...) {
				if set.Category(id) == nil {
					return commandError{code: 1, err: fmt.Errorf("unknown category %q", id)}
				}
			}
			if scan == nil {
				scan = engine.Run
			}
			if noNetwork {
				networkEnabled = false
			}
			if jsonOutput && !snapshotOnly {
				if err := writeEvent(cmd.OutOrStdout(), terminalEvent{Type: "start", Schema: report.Schema, Mode: "full"}); err != nil {
					return commandError{code: 3, err: err}
				}
			}
			result, err := scan(cmd.Context(), set, engine.Options{
				Network: networkEnabled,
				Reporting: report.BuildOptions{
					IncludedCategories: categories,
					ExcludedCategories: excluded,
					MinimumSeverity:    severity,
				},
			})
			if err != nil {
				if errors.Is(err, context.Canceled) {
					if jsonOutput || snapshotOnly {
						return writeEvent(cmd.OutOrStdout(), terminalEvent{Type: "cancelled"})
					}
					fmt.Fprintln(cmd.ErrOrStderr(), "privacybit: audit cancelled")
					return nil
				}
				if jsonOutput || snapshotOnly {
					if writeErr := writeEvent(cmd.OutOrStdout(), terminalEvent{Type: "error", Error: err.Error()}); writeErr != nil {
						return commandError{code: 3, err: writeErr}
					}
				}
				return commandError{code: 2, err: fmt.Errorf("audit: %w", err)}
			}
			if jsonOutput || snapshotOnly {
				if err := writeEvent(cmd.OutOrStdout(), terminalEvent{
					Type: "done", Grade: result.Overall.Grade, Score: result.Overall.Score, Snapshot: &result,
				}); err != nil {
					return commandError{code: 3, err: err}
				}
			} else {
				if err := writeHuman(cmd.OutOrStdout(), result, severity); err != nil {
					return commandError{code: 3, err: err}
				}
			}
			if !hasCoverage(result) {
				return commandError{code: 2, err: errors.New("no detection source produced coverage")}
			}
			if useThreshold && result.Overall.Grade.Risk() >= threshold {
				return commandError{code: 10, err: fmt.Errorf("overall grade %s met --fail-on %s", result.Overall.Grade, failOn)}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit newline-delimited JSON events")
	cmd.Flags().BoolVar(&snapshotOnly, "snapshot", false, "emit one snapshot event and exit")
	cmd.Flags().StringSliceVar(&categories, "categories", nil, "restrict displayed categories")
	cmd.Flags().StringSliceVar(&excluded, "exclude-categories", nil, "hide selected categories")
	cmd.Flags().StringVar(&severity, "severity", "low", "minimum finding severity: low, medium, or high")
	cmd.Flags().BoolVar(&networkEnabled, "network", true, "run point-in-time VPN route and DNS checks")
	cmd.Flags().BoolVar(&noNetwork, "no-network", false, "skip network checks; affected results remain unknown")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "exit 10 when the overall grade is at or worse than a grade")
	return cmd
}

func writeEvent(w io.Writer, event terminalEvent) error {
	return json.NewEncoder(w).Encode(event)
}

func validSeverity(value string) bool {
	switch value {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}

func failThreshold(value string) (int, bool) {
	switch catalog.Grade(value) {
	case catalog.GradeGreen, catalog.GradeAmber, catalog.GradeRed, catalog.GradeUnknown:
		return catalog.Grade(value).Risk(), true
	default:
		return 0, false
	}
}

func hasCoverage(snapshot report.Snapshot) bool {
	for _, source := range snapshot.Coverage {
		if source.Ready {
			return true
		}
	}
	return false
}

func writeHuman(w io.Writer, snapshot report.Snapshot, severity string) error {
	var out strings.Builder
	fmt.Fprintf(&out, "Overall: %s\n", gradeCell(snapshot.Overall.Grade))
	if snapshot.Overall.Score != nil {
		fmt.Fprintf(&out, "Category progress: %d/100 (not an overall privacy score)\n", *snapshot.Overall.Score)
	}
	fmt.Fprintln(&out, "\nCategories:")
	for _, category := range snapshot.Categories {
		fmt.Fprintf(&out, "  %s: %s (%d installed)", category.Name, gradeCell(category.Grade), category.InstalledTotal)
		if category.PartialCoverage {
			fmt.Fprint(&out, " — partial coverage")
		}
		fmt.Fprintln(&out)
		for _, app := range category.Installed {
			writeOriginContext(&out, app)
		}
		if category.InstalledTruncated {
			fmt.Fprintf(&out, "    Showing %d of %d installed apps.\n", category.InstalledShown, category.InstalledTotal)
		}
	}
	unmatchedWithOrigins := 0
	for _, app := range snapshot.Unmatched {
		if len(app.Origins) > 0 || app.OriginsTruncated {
			unmatchedWithOrigins++
		}
	}
	if unmatchedWithOrigins > 0 {
		fmt.Fprintln(&out, "\nUnmatched apps (Unrated):")
		for _, app := range snapshot.Unmatched {
			if len(app.Origins) == 0 && !app.OriginsTruncated {
				continue
			}
			fmt.Fprintf(&out, "  %s\n", app.Name)
			writeOriginContext(&out, app)
		}
	}
	fmt.Fprintf(&out, "\nFlatpak sandbox: %s\n", gradeCell(snapshot.Sandbox.Grade))
	if snapshot.Sandbox.Reason != "" {
		fmt.Fprintf(&out, "  %s\n", snapshot.Sandbox.Reason)
	}
	if snapshot.Sandbox.Applicable {
		if snapshot.Sandbox.AppsTotal > 0 {
			fmt.Fprintf(&out, "  Permission queries: %d of %d; %d incomplete\n", snapshot.Sandbox.AppsChecked, snapshot.Sandbox.AppsTotal, snapshot.Sandbox.AppsIncomplete)
		}
		if snapshot.Sandbox.PartialCoverage {
			fmt.Fprintln(&out, "  Partial coverage: some permission evidence is unknown or unclassified.")
		}
		for _, app := range snapshot.Sandbox.Apps {
			fmt.Fprintf(&out, "  %s (%s): %s\n", app.ID, app.Scope, gradeCell(app.Grade))
			if len(app.Grants) > 0 {
				fmt.Fprintf(&out, "    Grants: %s\n", strings.Join(app.Grants, ", "))
			}
			if app.GrantsTruncated {
				fmt.Fprintf(&out, "    Showing %d of %d recognized grants.\n", app.GrantsShown, app.GrantsTotal)
			}
			if len(app.Unknown) > 0 {
				fmt.Fprintf(&out, "    Unknown: %s\n", strings.Join(app.Unknown, "; "))
			}
			if app.UnknownTruncated {
				fmt.Fprintf(&out, "    Showing %d of %d unclassified items.\n", app.UnknownShown, app.UnknownTotal)
			}
		}
		if snapshot.Sandbox.AppsTruncated {
			fmt.Fprintf(&out, "  Showing %d of %d Flatpak copies.\n", snapshot.Sandbox.AppsShown, snapshot.Sandbox.AppsTotal)
		}
	}

	fmt.Fprintf(&out, "\nVPN: %s", snapshot.Network.VPN.State)
	if snapshot.Network.VPN.Client != "" {
		fmt.Fprintf(&out, " (%s)", snapshot.Network.VPN.Client)
	}
	fmt.Fprintln(&out)
	if snapshot.Network.VPN.KillSwitchNote != "" {
		fmt.Fprintf(&out, "  Kill switch: %s\n", snapshot.Network.VPN.KillSwitchNote)
	}
	for _, defect := range snapshot.Network.VPN.Defects {
		fmt.Fprintf(&out, "  %s: %s\n", defect.Title, defect.Detail)
	}

	if len(snapshot.Findings) == 0 {
		fmt.Fprintf(&out, "\nNo findings at or above %s severity. The grade and coverage still apply.\n", severity)
	} else {
		fmt.Fprintf(&out, "\nFindings (%d shown, %d total):\n", len(snapshot.Findings), snapshot.Overall.Findings)
		for _, finding := range snapshot.Findings {
			fmt.Fprintf(&out, "  %s [%s] %s — %s\n", finding.ID, finding.Severity, finding.Title, finding.Detail)
		}
	}

	fmt.Fprintln(&out, "\nCoverage:")
	for _, source := range snapshot.Coverage {
		state := "ready"
		if !source.Ready {
			state = "unavailable: " + source.Reason
		} else if source.Empty {
			state = "ready, empty"
		}
		fmt.Fprintf(&out, "  %s (%s): %s; %d observations\n", source.Name, source.Type, state, source.Observed)
	}
	if len(snapshot.Reduced) > 0 {
		fmt.Fprintln(&out, "\nReduced checks:")
		for _, reduced := range snapshot.Reduced {
			fmt.Fprintf(&out, "  %s: %s\n", reduced.Fact, reduced.Effect)
		}
	}
	_, err := io.WriteString(w, out.String())
	return err
}

func writeOriginContext(out *strings.Builder, app report.App) {
	if len(app.Origins) == 0 && !app.OriginsTruncated {
		return
	}
	fmt.Fprintf(out, "    Origin context for %s", app.Name)
	if app.Scope != "" {
		fmt.Fprintf(out, " (%s)", app.Scope)
	}
	fmt.Fprintln(out, ":")
	for _, origin := range app.Origins {
		fmt.Fprintf(out, "      %s (%s)\n", origin.Source, originStatus(origin.Official))
	}
	if app.OriginsTruncated {
		fmt.Fprintf(out, "      Showing %d of %d origins.\n", app.OriginsShown, app.OriginsTotal)
	}
}

func originStatus(official bool) string {
	if official {
		return "official"
	}
	return "not marked official"
}
