// Package audit matches detected software to the catalogue and grades the
// inventory using explicit coverage and measured evidence.
package audit

import (
	"sort"
	"strings"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/detect"
	"github.com/Nomadcxx/privacybit/internal/render"
)

// ActionKind names a user-facing operation attached to a finding.
type ActionKind string

const (
	ActionInform ActionKind = "inform"
	ActionLaunch ActionKind = "launch"
)

// Action describes a safe operation for a consumer to present. Commands and
// URLs are data only; this package never executes them.
type Action struct {
	Tier    int        `json:"tier"`
	Kind    ActionKind `json:"kind"`
	Label   string     `json:"label"`
	Target  string     `json:"target,omitempty"`
	Command string     `json:"command,omitempty"`
	URL     string     `json:"url,omitempty"`
	Detail  string     `json:"detail,omitempty"`
}

// Finding is a graded issue or a system fact derived from the audit result.
type Finding struct {
	ID           string        `json:"id"`
	Severity     string        `json:"severity"`
	Grade        catalog.Grade `json:"grade,omitempty"`
	Category     string        `json:"category"`
	Title        string        `json:"title"`
	Detail       string        `json:"detail"`
	Alternatives []string      `json:"alternatives,omitempty"`
	Actions      []Action      `json:"actions"`
}

// SourceCoverage records whether one source could inspect a detect type.
type SourceCoverage struct {
	Name string
	// Type is the detect-rule kind this source can enumerate. Package
	// backends share DetectPackage and may contribute independently.
	Type         catalog.DetectType
	Availability detect.Availability
	// Observed is the number of records the source returned before matching.
	Observed int
}

// InstalledEntry is one inventory observation after catalogue matching.
type InstalledEntry struct {
	Observation detect.Observation
	Product     string
	Category    string
	Grade       catalog.Grade
	Findings    []string
}

// ExpectationResult records whether an expectation was met and whether
// available sources were sufficient to judge an absence.
type ExpectationResult struct {
	ID       string
	Severity string
	Met      bool
	Judged   bool
	Grade    catalog.Grade
	Why      string
}

// CategoryResult is the worst supported grade and coverage state for one
// catalogue category.
type CategoryResult struct {
	ID              string
	Name            string
	Grade           catalog.Grade
	PartialCoverage bool
	Expectations    []ExpectationResult
}

// Result is an evaluated inventory. Unmatched applications remain visible in
// Inventory but do not contribute to any category grade.
type Result struct {
	Inventory  []InstalledEntry
	Categories []CategoryResult
	Sandbox    SandboxResult
	Overall    catalog.Grade
	Coverage   []SourceCoverage
}

// BuildFindings derives findings and tier 0/3 action descriptions from a
// graded result. family is one of the D6 distribution families. The builder is
// pure: it does not inspect the host or run the returned command.
func BuildFindings(set *catalog.Set, result Result, family string) []Finding {
	if set == nil {
		return nil
	}

	groups := make(map[string][]InstalledEntry)
	for _, item := range result.Inventory {
		if item.Product == "" || set.Entry(item.Product) == nil {
			continue
		}
		group := "installed"
		if item.Observation.Kind == detect.KindFlatpak {
			group = flatpakScopeKey(item.Observation.Scope)
		}
		key := item.Product + "\x00" + group
		groups[key] = append(groups[key], item)
	}

	findings := make([]Finding, 0)
	groupKeys := make([]string, 0, len(groups))
	for key := range groups {
		groupKeys = append(groupKeys, key)
	}
	sort.Strings(groupKeys)
	for _, key := range groupKeys {
		items := groups[key]
		product, group, _ := strings.Cut(key, "\x00")
		entry := set.Entry(product)
		grade := catalog.GradeGreen
		for _, item := range items {
			grade = catalog.Worse(grade, item.Grade)
		}
		if grade == catalog.GradeGreen {
			continue
		}
		findings = append(findings, installedFinding(set, entry, group, grade, items, family))
	}

	for _, categoryResult := range result.Categories {
		category := set.Category(categoryResult.ID)
		if category == nil {
			continue
		}
		for _, outcome := range categoryResult.Expectations {
			if outcome.Met || !outcome.Judged {
				continue
			}
			for _, expectation := range category.Expectations {
				if expectation.ID != outcome.ID {
					continue
				}
				finding := Finding{
					ID:           "expectation." + expectation.ID,
					Severity:     outcome.Severity,
					Grade:        outcome.Grade,
					Category:     category.ID,
					Title:        category.Name + " expectation is unmet",
					Detail:       outcome.Why,
					Alternatives: append([]string(nil), expectation.AnyOf...),
				}
				finding.Actions = []Action{{Tier: 0, Kind: ActionInform, Label: "Read why", Target: finding.ID}}
				finding.Actions = append(finding.Actions, alternativeActions(set, expectation.AnyOf, family)...)
				findings = append(findings, finding)
				break
			}
		}
	}

	findings = append(findings, flatpakScopeFindings(set, result.Inventory)...)
	sort.Slice(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
	return findings
}

func installedFinding(set *catalog.Set, entry *catalog.Entry, group string, grade catalog.Grade, items []InstalledEntry, family string) Finding {
	detail := []string{entry.Summary}
	for _, reason := range entry.Reasons {
		detail = append(detail, "Why: "+reason)
	}
	for _, caveat := range entry.Caveats {
		detail = append(detail, "Caveat: "+caveat)
	}
	for _, item := range items {
		for _, reason := range item.Findings {
			detail = appendUnique(detail, "Measured contradiction: "+reason)
		}
	}
	if strings.HasPrefix(group, "flatpak.") {
		detail = append(detail, "Installed at "+strings.TrimPrefix(group, "flatpak.")+" scope.")
	}
	finding := Finding{
		ID:           "entry." + entry.ID + "." + group,
		Severity:     severityForGrade(grade),
		Grade:        grade,
		Category:     entry.Category,
		Title:        entry.Name + " is rated " + string(grade),
		Detail:       strings.Join(detail, "\n\n"),
		Alternatives: append([]string(nil), entry.Alternatives...),
	}
	finding.Actions = []Action{{Tier: 0, Kind: ActionInform, Label: "Read why", Target: entry.ID}}
	finding.Actions = append(finding.Actions, alternativeActions(set, entry.Alternatives, family)...)
	return finding
}

func alternativeActions(set *catalog.Set, alternatives []string, family string) []Action {
	actions := make([]Action, 0, len(alternatives))
	for _, id := range alternatives {
		entry := set.Entry(id)
		if entry == nil {
			continue
		}
		instruction := render.Install(family, entry)
		if !instruction.Available {
			continue
		}
		label := "Show install steps for " + entry.Name
		if instruction.URL != "" {
			label = "Open upstream source for " + entry.Name
		}
		actions = append(actions, Action{
			Tier: 3, Kind: ActionLaunch, Label: label, Target: entry.ID,
			Command: instruction.Command, URL: instruction.URL, Detail: instruction.Reason,
		})
	}
	return actions
}

func flatpakScopeFindings(set *catalog.Set, inventory []InstalledEntry) []Finding {
	scopes := make(map[string]map[string]bool)
	for _, item := range inventory {
		if item.Product == "" || item.Observation.Kind != detect.KindFlatpak || set.Entry(item.Product) == nil {
			continue
		}
		if scopes[item.Product] == nil {
			scopes[item.Product] = make(map[string]bool)
		}
		scopes[item.Product][item.Observation.Scope] = true
	}

	products := make([]string, 0, len(scopes))
	for product := range scopes {
		if scopes[product]["system"] && scopes[product]["user"] {
			products = append(products, product)
		}
	}
	sort.Strings(products)
	findings := make([]Finding, 0, len(products))
	for _, product := range products {
		entry := set.Entry(product)
		finding := Finding{
			ID:       "flatpak.scope." + product,
			Severity: "low",
			Category: entry.Category,
			Title:    entry.Name + " is installed at system and user scope",
			Detail:   "Two copies exist at different privilege levels. The user installation shadows the system copy for this user.",
			Actions:  []Action{{Tier: 0, Kind: ActionInform, Label: "Read why", Target: product}},
		}
		findings = append(findings, finding)
	}
	return findings
}

func flatpakScopeKey(scope string) string {
	if scope == "" {
		return "flatpak.unknown"
	}
	return "flatpak." + scope
}

func severityForGrade(grade catalog.Grade) string {
	switch grade {
	case catalog.GradeRed:
		return "high"
	case catalog.GradeAmber:
		return "medium"
	default:
		return "low"
	}
}

func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

// Evaluate matches observations to exact catalogue detect rules. Source
// execution is owned by the caller.
func Evaluate(set *catalog.Set, observations []detect.Observation, coverage []SourceCoverage) Result {
	result := Result{
		Inventory: make([]InstalledEntry, 0, len(observations)),
		Coverage:  append([]SourceCoverage(nil), coverage...),
		Sandbox:   evaluateFlatpakSandbox(observations, coverage),
		Overall:   catalog.GradeUnknown,
	}
	for _, observation := range observations {
		item := InstalledEntry{Observation: observation, Grade: catalog.GradeUnknown}
		if set != nil {
			if kind, ok := detectType(observation.Kind); ok {
				if entry := set.Match(kind, observation.Name); entry != nil {
					item.Product = entry.ID
					item.Category = entry.Category
					item.Grade = gradeEntry(entry.Grade, observation, &item.Findings)
				}
			}
		}
		result.Inventory = append(result.Inventory, item)
	}
	if set == nil {
		if result.Sandbox.Applicable {
			result.Overall = result.Sandbox.Grade
		}
		return result
	}
	for _, category := range set.Categories {
		result.Categories = append(result.Categories, evaluateCategory(set, category, result.Inventory, coverage))
	}
	if len(result.Categories) == 0 {
		if result.Sandbox.Applicable {
			result.Overall = result.Sandbox.Grade
		}
		return result
	}
	result.Overall = catalog.GradeGreen
	for _, category := range result.Categories {
		result.Overall = catalog.Worse(result.Overall, category.Grade)
	}
	if result.Sandbox.Applicable {
		result.Overall = catalog.Worse(result.Overall, result.Sandbox.Grade)
	}
	return result
}

func evaluateCategory(set *catalog.Set, category *catalog.Category, inventory []InstalledEntry, coverage []SourceCoverage) CategoryResult {
	result := CategoryResult{ID: category.ID, Name: category.Name, Grade: catalog.GradeUnknown}
	for kind := range categoryDetectTypes(set, category) {
		if !hasReadySource(kind, coverage) {
			result.PartialCoverage = true
		}
	}

	grade := catalog.GradeGreen
	hasEvidence := false
	for _, item := range inventory {
		if item.Category != category.ID {
			continue
		}
		grade = catalog.Worse(grade, item.Grade)
		hasEvidence = true
	}
	for _, expectation := range category.Expectations {
		met := expectationMet(set, expectation, inventory)
		judged := met || expectationCovered(set, expectation, coverage)
		expectationGrade := catalog.GradeUnknown
		if judged {
			if met {
				expectationGrade = catalog.GradeGreen
			} else {
				switch expectation.Severity {
				case "high":
					expectationGrade = catalog.GradeRed
				case "medium", "low":
					expectationGrade = catalog.GradeAmber
				}
			}
		}
		result.Expectations = append(result.Expectations, ExpectationResult{
			ID: expectation.ID, Severity: expectation.Severity, Met: met,
			Judged: judged, Grade: expectationGrade, Why: expectation.Why,
		})
		grade = catalog.Worse(grade, expectationGrade)
		hasEvidence = true
	}
	if hasEvidence {
		result.Grade = grade
	}
	return result
}

func categoryDetectTypes(set *catalog.Set, category *catalog.Category) map[catalog.DetectType]bool {
	types := make(map[catalog.DetectType]bool)
	for i := range category.Entries {
		for _, rule := range category.Entries[i].Detects {
			types[rule.Type] = true
		}
	}
	for _, expectation := range category.Expectations {
		for _, token := range expectation.AnyOf {
			if entry := set.Entry(token); entry != nil {
				for _, rule := range entry.Detects {
					types[rule.Type] = true
				}
			} else {
				types[catalog.DetectPackage] = true
			}
		}
	}
	return types
}

func expectationMet(set *catalog.Set, expectation catalog.Expectation, inventory []InstalledEntry) bool {
	for _, token := range expectation.AnyOf {
		for _, item := range inventory {
			if item.Product == token {
				return true
			}
			if set.Entry(token) == nil && item.Observation.Kind == detect.KindPackage && strings.EqualFold(item.Observation.Name, token) {
				return true
			}
		}
	}
	return false
}

func expectationCovered(set *catalog.Set, expectation catalog.Expectation, coverage []SourceCoverage) bool {
	if len(expectation.AnyOf) == 0 {
		return false
	}
	for _, token := range expectation.AnyOf {
		entry := set.Entry(token)
		if entry == nil {
			if !hasReadySource(catalog.DetectPackage, coverage) {
				return false
			}
			continue
		}
		for _, rule := range entry.Detects {
			if !hasReadySource(rule.Type, coverage) {
				return false
			}
		}
	}
	return true
}

func hasReadySource(kind catalog.DetectType, coverage []SourceCoverage) bool {
	for _, source := range coverage {
		if source.Type == kind && source.Availability.Ready {
			return true
		}
	}
	return false
}

func gradeEntry(floor catalog.Grade, observation detect.Observation, findings *[]string) catalog.Grade {
	contradicted := false
	add := func(name string, signal detect.SignalState, why string) {
		if signal != detect.StateContradicted {
			return
		}
		contradicted = true
		if why == "" {
			why = name + " contradicts the catalogue"
		}
		*findings = append(*findings, why)
	}
	add("version", observation.Version.State, observation.Version.Why)
	add("origin", observation.Origin.State, observation.Origin.Why)
	add("licence", observation.Facts.Licence.State, observation.Facts.Licence.Why)
	add("vendor", observation.Facts.Vendor.State, observation.Facts.Vendor.Why)
	add("explicit-install state", observation.Facts.Explicit.State, observation.Facts.Explicit.Why)
	add("reverse dependencies", observation.Facts.ReverseDeps.State, observation.Facts.ReverseDeps.Why)
	add("signature state", observation.Facts.Signed.State, observation.Facts.Signed.Why)
	if !contradicted {
		return floor
	}
	// A contradiction downgrades one level. The metadata mismatch is evidence
	// to review, but does not by itself establish a red privacy posture.
	switch floor {
	case catalog.GradeGreen:
		return catalog.GradeAmber
	case catalog.GradeAmber:
		return catalog.GradeRed
	default:
		return floor
	}
}

func detectType(kind detect.Kind) (catalog.DetectType, bool) {
	switch kind {
	case detect.KindPackage:
		return catalog.DetectPackage, true
	case detect.KindBinary:
		return catalog.DetectBinary, true
	case detect.KindFlatpak:
		return catalog.DetectFlatpak, true
	case detect.KindDesktop:
		return catalog.DetectDesktop, true
	default:
		return "", false
	}
}
