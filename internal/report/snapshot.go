// Package report defines the versioned, bounded document shared by the CLI,
// TUI, and sysc-shell plugin.
package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/Nomadcxx/privacybit/internal/audit"
	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/detect"
	"github.com/Nomadcxx/privacybit/internal/network"
)

const (
	Schema               = 1
	ListLimit            = 100
	FindingLimit         = 100
	sandboxEvidenceLimit = 8
)

type Overall struct {
	Grade    catalog.Grade `json:"grade"`
	Score    *int          `json:"score,omitempty"`
	Findings int           `json:"findings"`
	High     int           `json:"high"`
}

type Coverage struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Ready    bool   `json:"ready"`
	Empty    bool   `json:"empty,omitempty"`
	Observed int    `json:"observed"`
	Reason   string `json:"reason,omitempty"`
}

type Reduced struct {
	Fact   string `json:"fact"`
	Effect string `json:"effect"`
}

type Scope struct {
	IncludedCategories []string `json:"included_categories,omitempty"`
	ExcludedCategories []string `json:"excluded_categories,omitempty"`
	MinimumSeverity    string   `json:"minimum_severity,omitempty"`
}

type BuildOptions struct {
	IncludedCategories []string
	ExcludedCategories []string
	MinimumSeverity    string
}

type App struct {
	ID       string        `json:"id,omitempty"`
	Name     string        `json:"name"`
	Grade    catalog.Grade `json:"grade"`
	Category string        `json:"category,omitempty"`
	Family   string        `json:"family,omitempty"`
	Scope    string        `json:"scope,omitempty"`
	Sources  []string      `json:"sources"`
	Why      []string      `json:"why,omitempty"`
	Summary  string        `json:"summary,omitempty"`
	Caveats  []string      `json:"caveats,omitempty"`
}

type Expectation struct {
	ID       string        `json:"id"`
	Severity string        `json:"severity"`
	Met      bool          `json:"met"`
	Judged   bool          `json:"judged"`
	Grade    catalog.Grade `json:"grade"`
	Why      string        `json:"why"`
}

type Category struct {
	ID                  string        `json:"id"`
	Name                string        `json:"name"`
	Grade               catalog.Grade `json:"grade"`
	Installed           []App         `json:"installed"`
	InstalledShown      int           `json:"installed_shown"`
	InstalledTruncated  bool          `json:"installed_truncated"`
	InstalledTotal      int           `json:"installed_total"`
	Worst               *App          `json:"worst,omitempty"`
	Expectations        []Expectation `json:"expectations,omitempty"`
	ExpectationsMet     []string      `json:"expectations_met,omitempty"`
	ExpectationsMissing []string      `json:"expectations_missing,omitempty"`
	PartialCoverage     bool          `json:"partial_coverage,omitempty"`
}

type Network struct {
	VPN                         network.Result `json:"vpn"`
	TelemetryConnectionsUnknown bool           `json:"telemetry_connections_unknown"`
}

type Sandbox struct {
	Applicable      bool          `json:"applicable"`
	Grade           catalog.Grade `json:"grade"`
	Apps            []SandboxApp  `json:"apps"`
	AppsShown       int           `json:"apps_shown"`
	AppsTotal       int           `json:"apps_total"`
	AppsTruncated   bool          `json:"apps_truncated"`
	AppsChecked     int           `json:"apps_checked"`
	AppsIncomplete  int           `json:"apps_incomplete"`
	PartialCoverage bool          `json:"partial_coverage"`
	Reason          string        `json:"reason,omitempty"`
}

type SandboxApp struct {
	ID               string        `json:"id"`
	Scope            string        `json:"scope"`
	Grade            catalog.Grade `json:"grade"`
	Grants           []string      `json:"grants,omitempty"`
	GrantsShown      int           `json:"grants_shown,omitempty"`
	GrantsTotal      int           `json:"grants_total,omitempty"`
	GrantsTruncated  bool          `json:"grants_truncated,omitempty"`
	Unknown          []string      `json:"unknown,omitempty"`
	UnknownShown     int           `json:"unknown_shown,omitempty"`
	UnknownTotal     int           `json:"unknown_total,omitempty"`
	UnknownTruncated bool          `json:"unknown_truncated,omitempty"`
}

type Snapshot struct {
	Schema             int             `json:"schema"`
	Available          bool            `json:"available"`
	GeneratedAt        time.Time       `json:"generated_at"`
	DurationMS         int64           `json:"duration_ms"`
	CatalogRev         string          `json:"catalog_rev"`
	Overall            Overall         `json:"overall"`
	Network            Network         `json:"network"`
	Sandbox            Sandbox         `json:"sandbox"`
	Categories         []Category      `json:"categories"`
	Unmatched          []App           `json:"unmatched,omitempty"`
	UnmatchedTotal     int             `json:"unmatched_total,omitempty"`
	UnmatchedTruncated bool            `json:"unmatched_truncated,omitempty"`
	Findings           []audit.Finding `json:"findings"`
	FindingsTotal      int             `json:"findings_total"`
	FindingsShown      int             `json:"findings_shown"`
	FindingsTruncated  bool            `json:"findings_truncated"`
	Coverage           []Coverage      `json:"coverage"`
	Reduced            []Reduced       `json:"reduced,omitempty"`
	Scope              Scope           `json:"scope,omitempty"`
	EntryPoint         string          `json:"entry_point"`
}

// Build creates a stable, bounded report from the evaluator's result.
func Build(set *catalog.Set, result audit.Result, findings []audit.Finding, netResult network.Result, coverage []audit.SourceCoverage, generated time.Time, duration time.Duration, opts BuildOptions) Snapshot {
	snapshot := Snapshot{
		Schema: Schema, Available: true, GeneratedAt: generated.UTC(),
		DurationMS: duration.Milliseconds(), Network: Network{
			VPN: netResult, TelemetryConnectionsUnknown: true,
		}, Findings: append([]audit.Finding(nil), findings...),
		EntryPoint: "privacybit",
	}
	snapshot.Scope = Scope{
		IncludedCategories: sortedCopy(opts.IncludedCategories),
		ExcludedCategories: sortedCopy(opts.ExcludedCategories),
		MinimumSeverity:    opts.MinimumSeverity,
	}
	if set != nil {
		snapshot.CatalogRev = set.Revision
	}
	for _, source := range coverage {
		snapshot.Coverage = append(snapshot.Coverage, Coverage{
			Name: source.Name, Type: string(source.Type), Ready: source.Availability.Ready,
			Empty: source.Availability.Empty, Observed: source.Observed,
			Reason: source.Availability.Reason,
		})
		if !source.Availability.Ready {
			snapshot.Reduced = append(snapshot.Reduced, Reduced{Fact: string(source.Type), Effect: source.Name + ": " + source.Availability.Reason})
		}
	}
	if netResult.State == network.Unknown {
		for _, why := range netResult.Coverage {
			snapshot.Reduced = append(snapshot.Reduced, Reduced{Fact: "network.vpn", Effect: why})
		}
	}
	snapshot.Reduced = append(snapshot.Reduced,
		Reduced{Fact: "network.telemetry", Effect: "live telemetry endpoint attribution is not implemented"},
	)
	if result.Sandbox.Applicable && result.Sandbox.PartialCoverage {
		effect := result.Sandbox.Reason
		if effect == "" {
			effect = fmt.Sprintf("permission evidence is incomplete for %d of %d installed copies", result.Sandbox.AppsIncomplete, result.Sandbox.AppsTotal)
		}
		snapshot.Reduced = append(snapshot.Reduced, Reduced{Fact: "flatpak.sandbox", Effect: effect})
	}
	snapshot.Sandbox = buildSandbox(result.Sandbox)

	appsByCategory := make(map[string][]App)
	for _, app := range groupedApps(set, result.Inventory) {
		if app.Category == "" {
			snapshot.Unmatched = append(snapshot.Unmatched, app)
		} else {
			appsByCategory[app.Category] = append(appsByCategory[app.Category], app)
		}
	}
	for _, category := range result.Categories {
		out := Category{
			ID: category.ID, Name: category.Name, Grade: category.Grade,
			PartialCoverage: category.PartialCoverage,
		}
		out.Installed = appsByCategory[category.ID]
		out.InstalledTotal = len(out.Installed)
		if len(out.Installed) > 0 {
			worst := out.Installed[0]
			for _, app := range out.Installed[1:] {
				if app.Grade.Risk() > worst.Grade.Risk() || (app.Grade == worst.Grade && app.Name < worst.Name) {
					worst = app
				}
			}
			out.Worst = &worst
		}
		out.InstalledTruncated = len(out.Installed) > ListLimit
		if out.InstalledTruncated {
			out.Installed = out.Installed[:ListLimit]
		}
		out.InstalledShown = len(out.Installed)
		for _, expectation := range category.Expectations {
			out.Expectations = append(out.Expectations, Expectation{
				ID: expectation.ID, Severity: expectation.Severity, Met: expectation.Met,
				Judged: expectation.Judged, Grade: expectation.Grade, Why: expectation.Why,
			})
			if expectation.Met {
				out.ExpectationsMet = append(out.ExpectationsMet, expectation.ID)
			} else if expectation.Judged {
				out.ExpectationsMissing = append(out.ExpectationsMissing, expectation.Why)
			}
		}
		snapshot.Categories = append(snapshot.Categories, out)
	}
	snapshot.UnmatchedTotal = len(snapshot.Unmatched)
	snapshot.FindingsTotal = len(findings)
	snapshot.Overall = Overall{Grade: result.Overall, Score: progressScore(snapshot.Categories), Findings: len(findings)}
	for _, finding := range findings {
		if finding.Severity == "high" {
			snapshot.Overall.High++
		}
	}
	snapshot.Findings = filterFindings(snapshot.Findings, opts)
	if len(opts.IncludedCategories) > 0 || len(opts.ExcludedCategories) > 0 {
		selected := make([]Category, 0, len(snapshot.Categories))
		for _, category := range snapshot.Categories {
			if categorySelected(category.ID, opts) {
				selected = append(selected, category)
			}
		}
		snapshot.Categories = selected
	}
	if len(snapshot.Findings) > FindingLimit {
		snapshot.Findings = snapshot.Findings[:FindingLimit]
		snapshot.FindingsTruncated = true
	}
	snapshot.FindingsShown = len(snapshot.Findings)
	if len(snapshot.Unmatched) > ListLimit {
		snapshot.Unmatched = snapshot.Unmatched[:ListLimit]
		snapshot.UnmatchedTruncated = true
	}
	return snapshot
}

func buildSandbox(source audit.SandboxResult) Sandbox {
	out := Sandbox{
		Applicable: source.Applicable, Grade: source.Grade,
		AppsTotal: source.AppsTotal, AppsChecked: source.AppsChecked,
		AppsIncomplete: source.AppsIncomplete, PartialCoverage: source.PartialCoverage,
		Reason: source.Reason,
	}
	apps := make([]SandboxApp, 0, len(source.Apps))
	for _, app := range source.Apps {
		projected := SandboxApp{
			ID: app.ID, Scope: app.Scope, Grade: app.Grade,
			GrantsTotal: len(app.Grants), UnknownTotal: len(app.Unknown),
		}
		projected.Grants = append([]string(nil), app.Grants[:min(len(app.Grants), sandboxEvidenceLimit)]...)
		projected.GrantsShown = len(projected.Grants)
		projected.GrantsTruncated = projected.GrantsShown < projected.GrantsTotal
		projected.Unknown = append([]string(nil), app.Unknown[:min(len(app.Unknown), sandboxEvidenceLimit)]...)
		projected.UnknownShown = len(projected.Unknown)
		projected.UnknownTruncated = projected.UnknownShown < projected.UnknownTotal
		apps = append(apps, projected)
	}
	out.AppsTruncated = len(apps) > ListLimit
	if out.AppsTruncated {
		apps = apps[:ListLimit]
	}
	out.Apps = apps
	out.AppsShown = len(apps)
	return out
}

func filterFindings(findings []audit.Finding, opts BuildOptions) []audit.Finding {
	out := findings[:0]
	minimum := severityRank(opts.MinimumSeverity)
	for _, finding := range findings {
		if !categorySelected(finding.Category, opts) || severityRank(finding.Severity) < minimum {
			continue
		}
		out = append(out, finding)
	}
	return out
}

func categorySelected(id string, opts BuildOptions) bool {
	if len(opts.IncludedCategories) > 0 {
		found := false
		for _, included := range opts.IncludedCategories {
			if included == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, excluded := range opts.ExcludedCategories {
		if excluded == id {
			return false
		}
	}
	return true
}

func severityRank(value string) int {
	switch value {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	default:
		return 1
	}
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func groupedApps(set *catalog.Set, inventory []audit.InstalledEntry) []App {
	groups := make(map[string]*App)
	for _, item := range inventory {
		key := item.Product
		if key == "" {
			key = "?" + item.Observation.Kind.String() + ":" + item.Observation.Name
		}
		if item.Observation.Kind == detect.KindFlatpak {
			key += "\x00" + item.Observation.Scope
		}
		app := groups[key]
		if app == nil {
			app = &App{Name: item.Observation.Name, Grade: item.Grade, Scope: item.Observation.Scope}
			if item.Product != "" && set != nil {
				if entry := set.Entry(item.Product); entry != nil {
					app.ID, app.Name, app.Category, app.Family = entry.ID, entry.Name, entry.Category, entry.Family
					app.Grade = entry.Grade
					app.Summary = entry.Summary
					app.Caveats = append([]string(nil), entry.Caveats...)
					app.Why = append([]string(nil), entry.Reasons...)
				}
			}
			groups[key] = app
		}
		app.Grade = catalog.Worse(app.Grade, item.Grade)
		app.Sources = appendUnique(app.Sources, item.Observation.Kind.String())
		for _, reason := range item.Findings {
			app.Why = appendUnique(app.Why, reason)
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]App, 0, len(keys))
	for _, key := range keys {
		app := groups[key]
		sort.Strings(app.Sources)
		out = append(out, *app)
	}
	return out
}

func appendUnique(items []string, value string) []string {
	for _, existing := range items {
		if existing == value {
			return items
		}
	}
	return append(items, value)
}

func progressScore(categories []Category) *int {
	total, judged := 0, 0
	for _, category := range categories {
		switch category.Grade {
		case catalog.GradeGreen:
			total += 100
			judged++
		case catalog.GradeAmber:
			total += 50
			judged++
		case catalog.GradeRed:
			judged++
		}
	}
	if judged == 0 {
		return nil
	}
	value := (total + judged/2) / judged
	return &value
}

// ParseSnapshot rejects unsupported major schemas and refuses partial data.
func ParseSnapshot(data []byte) (*Snapshot, error) {
	var snapshot Snapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("snapshot has trailing JSON")
		}
		return nil, err
	}
	if snapshot.Schema != Schema {
		return nil, errors.New("unsupported snapshot schema")
	}
	if !snapshot.Available || snapshot.GeneratedAt.IsZero() {
		return nil, errors.New("snapshot is unavailable or has no timestamp")
	}
	return &snapshot, nil
}
