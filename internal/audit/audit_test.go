package audit

import (
	"strings"
	"testing"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/detect"
)

func TestEvaluateMatchesExactObservationsAndKeepsFlatpakScopes(t *testing.T) {
	set, err := catalog.Load(catalog.LoadOptions{})
	if err != nil {
		t.Fatalf("load catalogue: %v", err)
	}

	result := Evaluate(set, []detect.Observation{
		{Name: "brave", Kind: detect.KindPackage},
		{Name: "brave-extra", Kind: detect.KindPackage, Product: "brave"},
		{Name: "com.brave.Browser", Kind: detect.KindFlatpak, Scope: "system"},
		{Name: "com.brave.Browser", Kind: detect.KindFlatpak, Scope: "user"},
	}, completeCoverage())

	if got, want := len(result.Inventory), 4; got != want {
		t.Fatalf("inventory length = %d, want %d", got, want)
	}
	if got := result.Inventory[0].Product; got != "brave" {
		t.Fatalf("package match = %q, want brave", got)
	}
	if got := result.Inventory[1].Grade; got != catalog.GradeUnknown {
		t.Fatalf("substring observation grade = %q, want unknown", got)
	}
	for i, scope := range []string{"system", "user"} {
		item := result.Inventory[i+2]
		if item.Product != "brave" || item.Observation.Scope != scope {
			t.Fatalf("flatpak inventory[%d] = product %q scope %q, want brave at %s", i+2, item.Product, item.Observation.Scope, scope)
		}
	}
}

func TestContradictedEvidenceWorsensTheCatalogueGradeByOne(t *testing.T) {
	set, err := catalog.Load(catalog.LoadOptions{})
	if err != nil {
		t.Fatalf("load catalogue: %v", err)
	}
	if got := set.Entry("brave-origin").Grade; got != catalog.GradeGreen {
		t.Fatalf("test entry grade = %q, want green", got)
	}

	base := Evaluate(set, []detect.Observation{{Name: "brave-origin", Kind: detect.KindPackage}}, completeCoverage())
	if got := base.Inventory[0].Grade; got != catalog.GradeGreen {
		t.Fatalf("unset evidence grade = %q, want catalogue floor green", got)
	}

	measured := detect.Observation{
		Name: "brave-origin", Kind: detect.KindPackage,
		Facts: detect.Facts{Licence: detect.Observed("GPL-3.0-or-later"), Vendor: detect.Unavailable[string]("not reported")},
	}
	if got := Evaluate(set, []detect.Observation{measured}, completeCoverage()).Inventory[0].Grade; got != catalog.GradeGreen {
		t.Fatalf("observed or unavailable signals changed grade to %q, want green", got)
	}

	for _, tc := range []struct {
		product string
		floor   catalog.Grade
		want    catalog.Grade
	}{
		{product: "brave-origin", floor: catalog.GradeGreen, want: catalog.GradeAmber},
		{product: "brave", floor: catalog.GradeAmber, want: catalog.GradeRed},
		{product: "chrome", floor: catalog.GradeRed, want: catalog.GradeRed},
	} {
		entry := set.Entry(tc.product)
		if entry.Grade != tc.floor {
			t.Fatalf("%s test entry grade = %q, want %q", tc.product, entry.Grade, tc.floor)
		}
		observation := detect.Observation{
			Name: tc.product, Kind: detect.KindPackage,
			Facts: detect.Facts{Licence: detect.Contradicted("Proprietary", "package metadata contradicts the catalogue")},
		}
		if got := Evaluate(set, []detect.Observation{observation}, completeCoverage()).Inventory[0].Grade; got != tc.want {
			t.Errorf("%s contradicted grade = %q, want %q", tc.product, got, tc.want)
		}
	}
}

func TestEveryCatalogueEntryAndSignalStateObeysTheGradeRule(t *testing.T) {
	set := loadCatalogue(t)
	fields := []struct {
		name  string
		apply func(*detect.Observation, detect.SignalState)
	}{
		{"version", func(o *detect.Observation, s detect.SignalState) { o.Version = testSignal[string](s) }},
		{"origin", func(o *detect.Observation, s detect.SignalState) { o.Origin = testSignal[detect.Origin](s) }},
		{"licence", func(o *detect.Observation, s detect.SignalState) { o.Facts.Licence = testSignal[string](s) }},
		{"vendor", func(o *detect.Observation, s detect.SignalState) { o.Facts.Vendor = testSignal[string](s) }},
		{"explicit", func(o *detect.Observation, s detect.SignalState) { o.Facts.Explicit = testSignal[bool](s) }},
		{"reverse-deps", func(o *detect.Observation, s detect.SignalState) { o.Facts.ReverseDeps = testSignal[[]string](s) }},
		{"signed", func(o *detect.Observation, s detect.SignalState) { o.Facts.Signed = testSignal[bool](s) }},
	}
	states := []detect.SignalState{detect.StateZero, detect.StateObserved, detect.StateUnavailable, detect.StateContradicted}
	for _, entry := range set.Entries() {
		if len(entry.Detects) == 0 {
			t.Fatalf("entry %q has no detect rules", entry.ID)
		}
		kind := observationKind(entry.Detects[0].Type)
		for _, field := range fields {
			for _, state := range states {
				observation := detect.Observation{Name: entry.Detects[0].Name, Kind: kind}
				field.apply(&observation, state)
				findings := []string(nil)
				got := gradeEntry(entry.Grade, observation, &findings)
				want := entry.Grade
				if state == detect.StateContradicted {
					want = worseOneStep(entry.Grade)
					if len(findings) != 1 || findings[0] == "" {
						t.Errorf("%s %s contradicted findings = %v, want its reason", entry.ID, field.name, findings)
					}
				} else if len(findings) != 0 {
					t.Errorf("%s %s state %s findings = %v, want none", entry.ID, field.name, state, findings)
				}
				if got != want {
					t.Errorf("%s %s state %s grade = %q, want %q", entry.ID, field.name, state, got, want)
				}
			}
		}
	}
}

func TestUnmetExpectationsUseSeverityAndUngradedPackageNamesCanSatisfy(t *testing.T) {
	set := loadCatalogue(t)
	result := Evaluate(set, nil, completeCoverage())
	passwords := categoryResult(t, result, "passwords")
	if passwords.Grade != catalog.GradeRed {
		t.Fatalf("passwords grade with missing high expectation = %q, want red", passwords.Grade)
	}
	if got := expectationResult(t, passwords, "passwords.present").Grade; got != catalog.GradeRed {
		t.Fatalf("high expectation grade = %q, want red", got)
	}
	if got := expectationResult(t, passwords, "passwords.local-first").Grade; got != catalog.GradeAmber {
		t.Fatalf("low expectation grade = %q, want amber", got)
	}
	if got := categoryResult(t, result, "media").Grade; got != catalog.GradeAmber {
		t.Fatalf("media grade with missing low expectation = %q, want amber", got)
	}

	token := ungradedToken(t, set, "browser")
	result = Evaluate(set, []detect.Observation{{Name: token, Kind: detect.KindPackage}}, completeCoverage())
	browser := categoryResult(t, result, "browser")
	present := expectationResult(t, browser, "browser.present")
	if !present.Met || !present.Judged || present.Grade != catalog.GradeGreen {
		t.Fatalf("ungraded package token %q did not satisfy browser.present: %+v", token, present)
	}
	if browser.Grade != catalog.GradeAmber {
		t.Fatalf("browser grade after ungraded browser package = %q, want amber for missing medium expectation", browser.Grade)
	}
}

func TestCoverageGapsYieldUnknownWhenTheyBlockAnUnmetExpectation(t *testing.T) {
	set := loadCatalogue(t)
	coverage := coverageWithoutFlatpak()
	result := Evaluate(set, nil, coverage)
	browser := categoryResult(t, result, "browser")
	if browser.Grade != catalog.GradeUnknown || !browser.PartialCoverage {
		t.Fatalf("browser result = grade %q partial %t, want unknown with partial coverage", browser.Grade, browser.PartialCoverage)
	}
	if result.Overall != catalog.GradeUnknown {
		t.Fatalf("overall = %q, want unknown to propagate", result.Overall)
	}
	if got := expectationResult(t, browser, "browser.present"); got.Judged {
		t.Fatalf("unmet expectation judged despite missing a detector: %+v", got)
	}
}

func TestPartialCoverageDoesNotAlterAStillJudgeableCategory(t *testing.T) {
	set := loadCatalogue(t)
	unknown := detect.Observation{
		Name: "not-in-catalogue", Kind: detect.KindPackage,
		Facts: detect.Facts{Licence: detect.Contradicted("x", "unknown app contradiction")},
	}
	result := Evaluate(set, []detect.Observation{
		{Name: "firefox", Kind: detect.KindPackage},
		{Name: "ublock-origin", Kind: detect.KindPackage},
		unknown,
	}, coverageWithoutFlatpak())
	browser := categoryResult(t, result, "browser")
	if browser.Grade != catalog.GradeAmber || !browser.PartialCoverage {
		t.Fatalf("browser result = grade %q partial %t, want known amber with partial coverage", browser.Grade, browser.PartialCoverage)
	}
	if got := expectationResult(t, browser, "browser.adblocking"); !got.Met || !got.Judged {
		t.Fatalf("installed blocker did not satisfy the expectation: %+v", got)
	}
	if got := result.Inventory[2].Grade; got != catalog.GradeUnknown {
		t.Fatalf("unmatched app grade = %q, want unknown", got)
	}
}

func TestOverallUsesWorstCategoryEvidence(t *testing.T) {
	set := loadCatalogue(t)
	result := Evaluate(set, []detect.Observation{{Name: "brave", Kind: detect.KindPackage}}, completeCoverage())
	if got := categoryResult(t, result, "passwords").Grade; got != catalog.GradeRed {
		t.Fatalf("passwords grade = %q, want red", got)
	}
	if result.Overall != catalog.GradeRed {
		t.Fatalf("overall = %q, want worst category red", result.Overall)
	}
}

func TestBuildFindingsIncludesNonGreenEntriesAndTierThreeAlternatives(t *testing.T) {
	set := loadCatalogue(t)
	result := Evaluate(set, []detect.Observation{{
		Name: "google-chrome-stable", Kind: detect.KindPackage,
		Facts: detect.Facts{Licence: detect.Contradicted("Proprietary", "package metadata contradicts the catalogue")},
	}}, completeCoverage())
	findings := BuildFindings(set, result, "arch")
	finding := findingByID(t, findings, "entry.chrome.installed")
	if finding.Grade != catalog.GradeRed || finding.Severity != "high" || finding.Category != "browser" {
		t.Fatalf("Chrome finding = %+v, want red/high/browser", finding)
	}
	if !strings.Contains(finding.Detail, "Google's browser") || !strings.Contains(finding.Detail, "Manifest V3") || !strings.Contains(finding.Detail, "package metadata contradicts the catalogue") {
		t.Fatalf("Chrome finding detail omits catalogue rationale: %q", finding.Detail)
	}
	if len(finding.Alternatives) == 0 || finding.Alternatives[0] != "brave-origin" {
		t.Fatalf("Chrome alternatives = %v, want brave-origin first", finding.Alternatives)
	}
	if len(finding.Actions) < 2 || finding.Actions[0].Tier != 0 || finding.Actions[0].Kind != ActionInform {
		t.Fatalf("Chrome actions = %+v, want tier 0 information and tier 3 alternatives", finding.Actions)
	}
	var install bool
	for _, action := range finding.Actions {
		if action.Tier == 3 && action.Target == "brave-origin" && action.Command == "pacman -S brave-origin" {
			install = true
		}
		if action.Tier == 1 || action.Tier == 2 {
			t.Fatalf("unsupported placeholder action emitted: %+v", action)
		}
	}
	if !install {
		t.Fatalf("Chrome finding has no tier 3 Brave Origin command: %+v", finding.Actions)
	}
}

func TestBuildFindingsUsesJudgedExpectationSeverityAndOmitsUnknowns(t *testing.T) {
	set := loadCatalogue(t)
	result := Evaluate(set, nil, completeCoverage())
	findings := BuildFindings(set, result, "arch")
	passwords := findingByID(t, findings, "expectation.passwords.present")
	if passwords.Grade != catalog.GradeRed || passwords.Severity != "high" || passwords.Detail == "" {
		t.Fatalf("missing password manager finding = %+v, want authored high-severity explanation", passwords)
	}
	if len(passwords.Actions) == 0 || passwords.Actions[0].Tier != 0 {
		t.Fatalf("expectation actions = %+v, want tier 0 information", passwords.Actions)
	}

	unknown := Evaluate(set, []detect.Observation{{Name: "not-catalogued", Kind: detect.KindPackage}}, nil)
	unknownFindings := BuildFindings(set, unknown, "arch")
	for _, finding := range unknownFindings {
		if strings.HasPrefix(finding.ID, "entry.") {
			t.Fatalf("unmatched observation created a product finding: %+v", finding)
		}
		if strings.HasPrefix(finding.ID, "expectation.") && finding.ID == "expectation.passwords.present" {
			t.Fatalf("unjudged expectation created a finding: %+v", finding)
		}
	}
}

func TestBuildFindingsReportsKnownFlatpakCopiesAtBothScopesWithoutRegrading(t *testing.T) {
	set := loadCatalogue(t)
	observations := []detect.Observation{
		{Name: "org.freetube.FreeTube", Kind: detect.KindFlatpak, Scope: "system"},
		{Name: "org.freetube.FreeTube", Kind: detect.KindFlatpak, Scope: "user"},
	}
	result := Evaluate(set, observations, completeCoverage())
	before := result.Overall
	finding := findingByID(t, BuildFindings(set, result, "arch"), "flatpak.scope.freetube")
	if finding.Severity != "low" || finding.Grade != "" || !strings.Contains(finding.Detail, "system") || !strings.Contains(finding.Detail, "user") {
		t.Fatalf("scope finding = %+v, want low-severity, ungraded system/user fact", finding)
	}
	if result.Overall != before || result.Inventory[0].Grade != catalog.GradeGreen || result.Inventory[1].Grade != catalog.GradeGreen {
		t.Fatalf("scope finding changed grades: overall %q, inventory %q/%q", result.Overall, result.Inventory[0].Grade, result.Inventory[1].Grade)
	}

	unmatched := Evaluate(set, []detect.Observation{
		{Name: "org.unknown.App", Kind: detect.KindFlatpak, Scope: "system"},
		{Name: "org.unknown.App", Kind: detect.KindFlatpak, Scope: "user"},
	}, completeCoverage())
	for _, got := range BuildFindings(set, unmatched, "arch") {
		if strings.HasPrefix(got.ID, "flatpak.scope.") {
			t.Fatalf("unmatched duplicate Flatpak created finding: %+v", got)
		}
	}
}

func TestBuildFindingsKeepsNamedFlatpakInstallationsSeparate(t *testing.T) {
	set := loadCatalogue(t)
	observations := []detect.Observation{
		{Name: "org.freetube.FreeTube", Kind: detect.KindFlatpak, Scope: "portable-a",
			Origin: detect.Contradicted(detect.Origin{Source: "custom"}, "origin mismatch in portable-a")},
		{Name: "org.freetube.FreeTube", Kind: detect.KindFlatpak, Scope: "portable-b",
			Origin: detect.Contradicted(detect.Origin{Source: "custom"}, "origin mismatch in portable-b")},
	}
	result := Evaluate(set, observations, completeCoverage())
	findings := BuildFindings(set, result, "arch")
	for _, scope := range []string{"portable-a", "portable-b"} {
		finding := findingByID(t, findings, "entry.freetube.flatpak."+scope)
		if !strings.Contains(finding.Detail, "Installed at "+scope+" scope") {
			t.Errorf("finding for %s = %+v, want that installation scope in its evidence", scope, finding)
		}
	}
}

func TestFlatpakSandboxWithoutInventoryCoverageIsUnknown(t *testing.T) {
	result := Evaluate(nil, nil, nil)
	if result.Overall != catalog.GradeUnknown || !result.Sandbox.Applicable || !result.Sandbox.PartialCoverage {
		t.Fatalf("missing Flatpak coverage = overall %q, sandbox %+v; want unknown partial coverage", result.Overall, result.Sandbox)
	}
	if result.Sandbox.AppsTotal != 0 || result.Sandbox.AppsIncomplete != 0 {
		t.Fatalf("unknown inventory must not invent app counts: %+v", result.Sandbox)
	}
}

func TestFlatpakSandboxEmptyInventoryIsNotAnUnknownCoverageGap(t *testing.T) {
	coverage := []SourceCoverage{{Name: "flatpak", Type: catalog.DetectFlatpak, Availability: detect.Availability{Ready: true, Empty: true}}}
	result := Evaluate(nil, nil, coverage)
	if result.Overall != catalog.GradeUnknown || result.Sandbox.Applicable || result.Sandbox.PartialCoverage {
		t.Fatalf("known empty Flatpak inventory = overall %q, sandbox %+v; want sandbox not applicable", result.Overall, result.Sandbox)
	}
}

func TestUnclassifiedFlatpakPermissionMakesGradeUnknownWithRedGrant(t *testing.T) {
	coverage := []SourceCoverage{{Name: "flatpak", Type: catalog.DetectFlatpak, Availability: detect.Available()}}
	result := Evaluate(nil, []detect.Observation{{
		Name: "com.example.App", Kind: detect.KindFlatpak, Scope: "user",
		SandboxPermissions: detect.Observed("[Context]\ndevices=all;\nfeatures=devel;\n"),
	}}, coverage)
	app := result.Sandbox.Apps[0]
	if result.Sandbox.Grade != catalog.GradeUnknown || app.Grade != catalog.GradeUnknown || !result.Sandbox.PartialCoverage || len(app.Unknown) == 0 {
		t.Fatalf("red grant with unclassified evidence = sandbox %+v, app %+v; want unknown", result.Sandbox, app)
	}
	if len(app.Grants) != 1 || app.Grants[0] != "all devices" {
		t.Fatalf("recognized red grant was dropped from unknown result: %+v", app)
	}
}

func TestFlatpakSandboxOrdersWorstEvidenceFirst(t *testing.T) {
	coverage := []SourceCoverage{{Name: "flatpak", Type: catalog.DetectFlatpak, Availability: detect.Available()}}
	result := Evaluate(nil, []detect.Observation{
		{Name: "com.example.Green", Kind: detect.KindFlatpak, Scope: "user", SandboxPermissions: detect.Observed("[Context]\nshared=ipc;\n")},
		{Name: "com.example.Red", Kind: detect.KindFlatpak, Scope: "user", SandboxPermissions: detect.Observed("[Context]\ndevices=all;\n")},
		{Name: "com.example.Unknown", Kind: detect.KindFlatpak, Scope: "user", SandboxPermissions: detect.Observed("[Context]\nfeatures=devel;\n")},
	}, coverage)
	if result.Sandbox.Grade != catalog.GradeUnknown {
		t.Fatalf("sandbox grade = %q, want unknown", result.Sandbox.Grade)
	}
	for i, want := range []string{"com.example.Unknown", "com.example.Red", "com.example.Green"} {
		if got := result.Sandbox.Apps[i].ID; got != want {
			t.Fatalf("sandbox app %d = %q, want %q", i, got, want)
		}
	}
}

func findingByID(t *testing.T, findings []Finding, id string) Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.ID == id {
			return finding
		}
	}
	t.Fatalf("finding %q not found in %+v", id, findings)
	return Finding{}
}

func completeCoverage() []SourceCoverage {
	return []SourceCoverage{
		{Name: "pacman", Type: catalog.DetectPackage, Availability: detect.Available()},
		{Name: "flatpak", Type: catalog.DetectFlatpak, Availability: detect.Available()},
		{Name: "binary", Type: catalog.DetectBinary, Availability: detect.Available()},
		{Name: "desktop", Type: catalog.DetectDesktop, Availability: detect.Available()},
	}
}

func testSignal[T any](state detect.SignalState) detect.Signal[T] {
	return detect.Signal[T]{State: state, Why: "measured test contradiction"}
}

func observationKind(kind catalog.DetectType) detect.Kind {
	switch kind {
	case catalog.DetectPackage:
		return detect.KindPackage
	case catalog.DetectBinary:
		return detect.KindBinary
	case catalog.DetectFlatpak:
		return detect.KindFlatpak
	case catalog.DetectDesktop:
		return detect.KindDesktop
	default:
		return detect.Kind(255)
	}
}

func worseOneStep(grade catalog.Grade) catalog.Grade {
	switch grade {
	case catalog.GradeGreen:
		return catalog.GradeAmber
	case catalog.GradeAmber:
		return catalog.GradeRed
	default:
		return grade
	}
}

func coverageWithoutFlatpak() []SourceCoverage {
	coverage := completeCoverage()
	for i := range coverage {
		if coverage[i].Type == catalog.DetectFlatpak {
			coverage[i].Availability = detect.NotReady("flatpak is not installed")
		}
	}
	return coverage
}

func loadCatalogue(t *testing.T) *catalog.Set {
	t.Helper()
	set, err := catalog.Load(catalog.LoadOptions{})
	if err != nil {
		t.Fatalf("load catalogue: %v", err)
	}
	return set
}

func ungradedToken(t *testing.T, set *catalog.Set, categoryID string) string {
	t.Helper()
	category := set.Category(categoryID)
	if category == nil {
		t.Fatalf("category %q missing", categoryID)
	}
	for _, expectation := range category.Expectations {
		for _, token := range expectation.AnyOf {
			if set.Entry(token) == nil {
				return token
			}
		}
	}
	t.Fatalf("category %q has no ungraded expectation token", categoryID)
	return ""
}

func categoryResult(t *testing.T, result Result, id string) CategoryResult {
	t.Helper()
	for _, category := range result.Categories {
		if category.ID == id {
			return category
		}
	}
	t.Fatalf("category %q missing from result", id)
	return CategoryResult{}
}

func expectationResult(t *testing.T, category CategoryResult, id string) ExpectationResult {
	t.Helper()
	for _, expectation := range category.Expectations {
		if expectation.ID == id {
			return expectation
		}
	}
	t.Fatalf("expectation %q missing from category %q", id, category.ID)
	return ExpectationResult{}
}
