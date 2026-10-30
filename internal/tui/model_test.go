package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/privacybit/internal/audit"
	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/network"
	"github.com/Nomadcxx/privacybit/internal/report"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func testSnapshot() report.Snapshot {
	return report.Snapshot{
		Schema: report.Schema, Available: true, GeneratedAt: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		Overall: report.Overall{Grade: catalog.GradeUnknown},
		Categories: []report.Category{
			{ID: "browser", Name: "Browser", Grade: catalog.GradeUnknown, Installed: []report.App{{ID: "unknown", Name: "Unlisted app", Grade: catalog.GradeUnknown}}},
			{ID: "vpn", Name: "VPN", Grade: catalog.GradeAmber},
		},
		Coverage: []report.Coverage{{Name: "pacman", Type: "package", Reason: "database unavailable"}},
		Network:  report.Network{VPN: network.UnknownResult("tunnel could not be verified"), TelemetryConnectionsUnknown: true},
		Reduced:  []report.Reduced{{Fact: "network.telemetry", Effect: "not implemented"}},
	}
}

func TestViewDoesNotRunScan(t *testing.T) {
	calls := 0
	model := NewModel(context.Background(), func(context.Context) (report.Snapshot, error) {
		calls++
		return testSnapshot(), nil
	})
	model.View()
	model.View()
	if calls != 0 {
		t.Fatalf("View ran scan %d times", calls)
	}
}

func TestStoredSnapshotOpensWithoutScanningAndRefreshesOnRequest(t *testing.T) {
	calls := 0
	want := testSnapshot()
	want.GeneratedAt = time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	model := NewModelWithSnapshot(context.Background(), want, func(context.Context) (report.Snapshot, error) {
		calls++
		fresh := testSnapshot()
		fresh.GeneratedAt = time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC)
		return fresh, nil
	})
	if model.loading || model.snapshot == nil || !model.snapshot.GeneratedAt.Equal(want.GeneratedAt) {
		t.Fatalf("stored model state: loading=%v snapshot=%+v", model.loading, model.snapshot)
	}
	if command := model.Init(); command != nil {
		t.Fatal("opening a stored snapshot scheduled a scan")
	}
	model.View()
	if calls != 0 {
		t.Fatalf("opening or rendering the stored snapshot ran %d scans", calls)
	}
	_, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if command == nil {
		t.Fatal("refresh key did not schedule a scan")
	}
	message := command()
	model.Update(message)
	if calls != 1 || model.snapshot == nil || model.snapshot.GeneratedAt.Hour() != 2 {
		t.Fatalf("refresh result: calls=%d snapshot=%+v", calls, model.snapshot)
	}
}

func TestNavigationPushAndPopPreserveParentSelection(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.stack = []Route{{Screen: ScreenCategories, Cursor: 1}}
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := model.stack[len(model.stack)-1]; got.Screen != ScreenCategory || got.ID != "vpn" {
		t.Fatalf("pushed route = %+v", got)
	}
	model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := model.stack[len(model.stack)-1]; got.Screen != ScreenCategories || got.Cursor != 1 {
		t.Fatalf("restored route = %+v", got)
	}
}

func TestHeaderShowsNavigationBreadcrumb(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Categories[0].Installed = []report.App{{
		ID: "chrome", Name: "Google Chrome", Grade: catalog.GradeGreen,
	}}
	model.stack = []Route{
		{Screen: ScreenOverview},
		{Screen: ScreenCategories},
		{Screen: ScreenCategory, ID: "browser"},
		{Screen: ScreenApp, ID: "chrome"},
	}

	header := model.header(model.stack[len(model.stack)-1])
	if !strings.Contains(header, "Browser › Google Chrome") {
		t.Fatalf("header does not show the category and app trail: %s", header)
	}
	if strings.Contains(header, "Overview") || strings.Contains(header, "Categories") {
		t.Fatalf("header includes navigation containers in the nested trail: %s", header)
	}
}

func TestHeaderKeepsUsefulRootTitles(t *testing.T) {
	model := NewModel(context.Background(), nil)
	for _, screen := range []struct {
		route Route
		want  string
	}{
		{route: Route{Screen: ScreenOverview}, want: "Overview"},
		{route: Route{Screen: ScreenCategories}, want: "Categories"},
	} {
		model.stack = []Route{screen.route}
		header := model.header(screen.route)
		if !strings.Contains(header, "privacybit  "+screen.want) {
			t.Errorf("root header = %q, want title %q", header, screen.want)
		}
	}
}

func TestGradeLabelsAlignAndOverallUsesGradeColor(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })

	snapshot := testSnapshot()
	snapshot.Overall.Grade = catalog.GradeRed
	snapshot.Categories = []report.Category{
		{ID: "good", Name: "Good row", Grade: catalog.GradeGreen},
		{ID: "review", Name: "Review row", Grade: catalog.GradeAmber},
		{ID: "risky", Name: "Risky row", Grade: catalog.GradeRed},
		{ID: "unknown", Name: "Unknown row", Grade: catalog.GradeUnknown},
	}
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(snapshot)
	model.width, model.height = 120, 36
	route := Route{Screen: ScreenOverview}
	model.stack = []Route{route}

	var positions []int
	list := model.listView(Route{Screen: ScreenCategories})
	for _, name := range []string{"Good row", "Review row", "Risky row", "Unknown row"} {
		found := false
		for _, line := range strings.Split(list, "\n") {
			if index := strings.Index(line, name); index >= 0 {
				positions = append(positions, lipgloss.Width(line[:index]))
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("list is missing %q:\n%s", name, list)
		}
	}
	for _, position := range positions[1:] {
		if position != positions[0] {
			t.Errorf("grade labels start names at columns %v, want one aligned column", positions)
			break
		}
	}

	header := model.header(route)
	if !strings.Contains(header, "\x1b[31m■ Risky") {
		t.Errorf("overall grade has no shared red grade color: %q", header)
	}
}

func TestUnknownAndCoverageRenderAsExplicitStates(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.stack = []Route{{Screen: ScreenNetwork}}
	view := model.View()
	for _, expected := range []string{"Unrated", "VPN state: unknown", "Default IPv4 route via tunnel: unknown", "Live telemetry connections: unavailable"} {
		if !strings.Contains(view, expected) {
			t.Errorf("View missing %q:\n%s", expected, view)
		}
	}
	model.stack = []Route{{Screen: ScreenCoverage}}
	view = model.View()
	for _, expected := range []string{"unavailable: database unavailable", "network.telemetry"} {
		if !strings.Contains(view, expected) {
			t.Errorf("coverage view missing %q:\n%s", expected, view)
		}
	}
}

func TestCoverageViewShowsSupportedAndReducedFacts(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Coverage = []report.Coverage{{
		Name: "dpkg", Type: "package", Ready: true, Observed: 1,
		Facts: []string{"explicit", "version"},
	}}
	model.snapshot.Reduced = []report.Reduced{{
		Fact: "licence", Effect: "not supported by the listed sources",
		Sources: []string{"desktop", "dpkg"},
	}}
	view := model.coverageView()
	for _, want := range []string{
		"dpkg (package): ready — 1 observations; facts: explicit, version",
		"Reduced · licence (desktop, dpkg): not supported by the listed sources",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("coverage view does not contain %q: %s", want, view)
		}
	}
}

func TestCoverageViewDoesNotInferMissingLegacyFactsAreEmpty(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Coverage = []report.Coverage{{
		Name: "dpkg", Type: "package", Ready: true, Observed: 1,
	}}
	if view := model.coverageView(); strings.Contains(view, "facts:") {
		t.Fatalf("legacy coverage without a facts field was presented as empty: %s", view)
	}
}

func TestAppRouteKeepsFlatpakScope(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	category := &model.snapshot.Categories[0]
	category.Installed = []report.App{
		{ID: "browser", Name: "Browser (system)", Scope: "system"},
		{ID: "browser", Name: "Browser (user)", Scope: "user"},
	}
	model.stack = []Route{{Screen: ScreenApp, ID: "browser\x00user"}}
	if got := model.appView(model.stack[0].ID); !strings.Contains(got, "Scope: user") {
		t.Fatalf("user-scope app view = %q", got)
	}
}

func TestAppViewShowsOriginContextWithoutChangingGrade(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Categories[0].Installed = []report.App{{
		ID: "browser", Name: "Browser", Grade: catalog.GradeGreen,
		Origins:      []report.AppOrigin{{Source: "flathub", Official: true}, {Source: "community"}},
		OriginsShown: 2, OriginsTotal: 3, OriginsTruncated: true,
	}}
	model.stack = []Route{{Screen: ScreenApp, ID: "browser"}}
	view := model.appView("browser")
	for _, want := range []string{"Origin context: flathub (recognized official label; configured source not verified)", "Origin context: community (not marked official)", "Showing 2 of 3 origins"} {
		if !strings.Contains(view, want) {
			t.Errorf("app view missing %q: %s", want, view)
		}
	}
	if !strings.Contains(view, "Good") || model.snapshot.Categories[0].Installed[0].Grade != catalog.GradeGreen {
		t.Errorf("origin context changed the app grade: %s", view)
	}
}

func TestUnmatchedAppDetailsShowOriginContext(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Unmatched = []report.App{{
		Name: "Mystery browser", Grade: catalog.GradeUnknown, Scope: "user", Sources: []string{"flatpak"},
		Origins: []report.AppOrigin{{Source: "community"}}, OriginsShown: 1, OriginsTotal: 1,
	}}
	model.snapshot.UnmatchedTotal = 1
	model.stack = []Route{{Screen: ScreenUnmatched}}
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	route := model.stack[len(model.stack)-1]
	if route.Screen != ScreenApp {
		t.Fatalf("selected unmatched app opened %q, want app details", route.Screen)
	}
	view := model.View()
	for _, want := range []string{"Mystery browser", "Unrated", "Origin context: community (not marked official)"} {
		if !strings.Contains(view, want) {
			t.Errorf("unmatched app view missing %q:\n%s", want, view)
		}
	}
}

func TestOverviewOpensSandboxDetails(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.stack = []Route{{Screen: ScreenOverview}}
	rows := model.visibleRows(model.stack[0])
	for i, item := range rows {
		if item.kind != "sandbox" {
			continue
		}
		model.stack[0].Cursor = i
		model.enterSelected()
		if got := model.stack[len(model.stack)-1].Screen; got != ScreenSandbox {
			t.Fatalf("sandbox route = %q", got)
		}
		return
	}
	t.Fatal("overview has no sandbox details route")
}

func TestSandboxViewShowsScopedEvidenceAndCoverage(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Sandbox = report.Sandbox{
		Applicable: true, Grade: catalog.GradeUnknown, AppsTotal: 3, AppsShown: 2,
		AppsTruncated: true, AppsChecked: 1, AppsIncomplete: 1, PartialCoverage: true,
		Apps: []report.SandboxApp{
			{
				ID: "org.example.App", Scope: "system", Grade: catalog.GradeRed,
				Grants: []string{"host filesystem"}, GrantsShown: 1, GrantsTotal: 2, GrantsTruncated: true,
			},
			{
				ID: "org.example.App", Scope: "user", Grade: catalog.GradeUnknown,
				Unknown: []string{"unclassified permission"}, UnknownShown: 1, UnknownTotal: 2, UnknownTruncated: true,
			},
		},
	}
	model.stack = []Route{{Screen: ScreenSandbox}}

	view := model.View()
	for _, want := range []string{
		"Flatpak sandbox", "Sandbox grade: ? Unrated", "Permission queries: 1 of 3; 1 incomplete",
		"Partial coverage", "org.example.App (system)", "org.example.App (user)",
		"Risky", "Unrated", "Showing 2 of 3 Flatpak copies",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("sandbox view missing %q:\n%s", want, view)
		}
	}

	model.stack = []Route{{Screen: ScreenSandbox, Cursor: 0}}
	model.enterSelected()
	if route := model.stack[len(model.stack)-1]; route.Screen != ScreenSandboxApp || route.ID != sandboxAppRouteID(model.snapshot.Sandbox.Apps[0]) {
		t.Fatalf("system copy route = %+v", route)
	}
	model.stack = []Route{{Screen: ScreenSandboxApp, ID: sandboxAppRouteID(model.snapshot.Sandbox.Apps[0])}}
	view = model.View()
	for _, want := range []string{"Scope: system", "Grants: host filesystem", "Showing 1 of 2 recognized grants"} {
		if !strings.Contains(view, want) {
			t.Errorf("system sandbox copy view missing %q:\n%s", want, view)
		}
	}
	model.stack = []Route{{Screen: ScreenSandbox, Cursor: 1}}
	model.enterSelected()
	if route := model.stack[len(model.stack)-1]; route.Screen != ScreenSandboxApp || route.ID != sandboxAppRouteID(model.snapshot.Sandbox.Apps[1]) {
		t.Fatalf("user copy route = %+v", route)
	}
	model.stack = []Route{{Screen: ScreenSandboxApp, ID: sandboxAppRouteID(model.snapshot.Sandbox.Apps[1])}}
	view = model.View()
	for _, want := range []string{"Scope: user", "Unknown: unclassified permission", "Showing 1 of 2 unclassified items"} {
		if !strings.Contains(view, want) {
			t.Errorf("user sandbox copy view missing %q:\n%s", want, view)
		}
	}
}

func TestSandboxViewExplainsNotApplicableState(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Sandbox = report.Sandbox{Grade: catalog.GradeUnknown, Reason: "Flatpak is not installed."}
	model.stack = []Route{{Screen: ScreenSandbox}}

	view := model.View()
	for _, want := range []string{"Flatpak sandbox", "not applicable", "Flatpak is not installed."} {
		if !strings.Contains(view, want) {
			t.Errorf("sandbox view missing %q:\n%s", want, view)
		}
	}
}

func TestSandboxViewExplainsKnownEmptyInventory(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Sandbox = report.Sandbox{
		Grade: catalog.GradeUnknown, Reason: "No Flatpak applications are installed.",
	}
	model.stack = []Route{{Screen: ScreenSandbox}}
	model.width, model.height = 80, 24

	view := model.View()
	if !strings.Contains(view, "not applicable") || !strings.Contains(view, "No Flatpak applications are installed.") {
		t.Fatalf("known empty inventory is not explicit:\n%s", view)
	}
	if strings.Contains(view, "Partial coverage") {
		t.Fatalf("known empty inventory was presented as incomplete:\n%s", view)
	}
}

func TestSandboxViewTreatsMissingApplicabilityAsUnknown(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.stack = []Route{{Screen: ScreenSandbox}}

	view := model.View()
	for _, want := range []string{"Sandbox grade: ? Unrated", "applicability was not recorded"} {
		if !strings.Contains(view, want) {
			t.Errorf("sandbox view missing %q:\n%s", want, view)
		}
	}
}

func TestSandboxViewDoesNotTreatIncompleteInventoryAsEmpty(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Sandbox = report.Sandbox{
		Applicable: true, Grade: catalog.GradeUnknown, PartialCoverage: true,
		Reason: "Flatpak inventory could not be checked.",
	}
	model.stack = []Route{{Screen: ScreenSandbox}}
	model.loading = false
	model.width, model.height = 80, 24

	view := model.View()
	if !strings.Contains(view, "Flatpak inventory could not be checked.") || !strings.Contains(view, "Partial coverage") {
		t.Fatalf("incomplete inventory context is missing:\n%s", view)
	}
	if strings.Contains(view, "No Flatpak copies were reported.") {
		t.Fatalf("incomplete inventory was presented as empty:\n%s", view)
	}
}

func TestSmallTerminalRequestsResizeInsteadOfClipping(t *testing.T) {
	model := NewModelWithSnapshot(context.Background(), testSnapshot(), nil)
	model.width, model.height = 20, 8
	if view := model.View(); !strings.Contains(view, "Terminal is too small") {
		t.Fatalf("small terminal view = %q", view)
	}
}

func TestUnknownOverallExplainsUnratedAreasAndWorstKnownGrade(t *testing.T) {
	model := NewModel(context.Background(), nil)
	snapshot := testSnapshot()
	snapshot.Overall.Grade = catalog.GradeUnknown
	snapshot.Categories = []report.Category{
		{ID: "browser", Name: "Browser", Grade: catalog.GradeUnknown, PartialCoverage: true},
		{ID: "chat", Name: "Chat", Grade: catalog.GradeAmber},
	}
	snapshot.Sandbox = report.Sandbox{Applicable: true, Grade: catalog.GradeUnknown}
	model.snapshot = ptr(snapshot)
	model.stack = []Route{{Screen: ScreenOverview}}

	view := model.View()
	for _, want := range []string{
		"Overall: ? Unrated", "Unrated areas: Browser, Flatpak sandbox",
		"Worst known result: ▲ Review (Chat)",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("overview missing %q:\n%s", want, view)
		}
	}
	snapshot.Sandbox.Grade = catalog.GradeRed
	model.snapshot = ptr(snapshot)
	if view := model.View(); !strings.Contains(view, "Worst known result: ■ Risky (Flatpak sandbox)") {
		t.Fatalf("overview omitted the worst known sandbox grade:\n%s", view)
	}
}

func TestLongDetailViewWrapsAndScrolls(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Findings = []audit.Finding{{
		ID: "long", Title: "Long finding",
		Detail: "BEGIN " + strings.Repeat("evidence ", 120) + "ENDMARKER",
	}}
	model.stack = []Route{{Screen: ScreenFinding, ID: "long"}}
	model.width, model.height = 48, 14

	first := model.View()
	if !strings.Contains(first, "BEGIN") || strings.Contains(first, "ENDMARKER") {
		t.Fatalf("first detail viewport = %q", first)
	}
	if lines := strings.Split(strings.TrimSuffix(first, "\n"), "\n"); len(lines) > model.height {
		t.Fatalf("detail view has %d lines for a %d-line terminal: %q", len(lines), model.height, first)
	}
	for range 100 {
		model.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if last := model.View(); !strings.Contains(last, "ENDMARKER") {
		t.Fatalf("last detail viewport does not reach the final evidence: %q", last)
	}
}

func TestFooterMatchesTheCurrentScreen(t *testing.T) {
	tests := []struct {
		name    string
		stack   []Route
		prepare func(*Model)
		want    []string
		avoid   []string
	}{
		{
			name:  "root list",
			stack: []Route{{Screen: ScreenOverview}},
			want:  []string{"Enter open", "/ filter", "q quit", "r refresh"},
			avoid: []string{"Esc back", "PgUp/PgDn page", "p purge plan"},
		},
		{
			name:  "nested detail",
			stack: []Route{{Screen: ScreenOverview}, {Screen: ScreenFinding, ID: "missing"}},
			want:  []string{"↑/↓ scroll", "PgUp/PgDn page", "Esc back", "q back"},
			avoid: []string{"Enter open", "/ filter", "p purge plan"},
		},
		{
			name:  "red app",
			stack: []Route{{Screen: ScreenOverview}, {Screen: ScreenApp, ID: "chrome"}},
			prepare: func(model *Model) {
				model.snapshot.Categories[0].Installed = []report.App{{ID: "chrome", Name: "Google Chrome", Grade: catalog.GradeRed}}
			},
			want: []string{"p purge plan"},
		},
		{
			name:  "amber app",
			stack: []Route{{Screen: ScreenOverview}, {Screen: ScreenApp, ID: "chrome"}},
			prepare: func(model *Model) {
				model.snapshot.Categories[0].Installed = []report.App{{ID: "chrome", Name: "Google Chrome", Grade: catalog.GradeAmber}}
			},
			avoid: []string{"p purge plan"},
		},
		{
			name:  "help",
			stack: []Route{{Screen: ScreenOverview}, {Screen: ScreenHelp}},
			avoid: []string{"? help", "Enter open", "/ filter"},
		},
		{
			name:  "filter input",
			stack: []Route{{Screen: ScreenOverview}, {Screen: ScreenCategories, Filtering: true}},
			want:  []string{"Type to filter", "Enter apply", "Esc clear"},
			avoid: []string{"r refresh", "? help", "/ filter"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModelWithSnapshot(context.Background(), testSnapshot(), nil)
			model.stack = tt.stack
			model.width, model.height = 120, 36
			if tt.prepare != nil {
				tt.prepare(model)
			}
			view := model.View()
			for _, want := range tt.want {
				if !strings.Contains(view, want) {
					t.Errorf("view is missing %q:\n%s", want, view)
				}
			}
			for _, avoid := range tt.avoid {
				if strings.Contains(view, avoid) {
					t.Errorf("view contains inapplicable hint %q:\n%s", avoid, view)
				}
			}
		})
	}
}

func TestHelpListsKeyboardBindingsAndGradeLegend(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.stack = []Route{{Screen: ScreenOverview}, {Screen: ScreenHelp}}
	model.width, model.height = 120, 40

	view := model.View()
	for _, want := range []string{
		"● Good", "▲ Review", "■ Risky", "? Unrated",
		"↑/↓, j/k", "Enter, →, l", "Esc, ←, h", "q",
		"PgUp/PgDn", "Home/End", "Refresh the audit", "network checks", "coverage",
		"Review a purge plan", "Open Help from another screen",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("help view is missing %q:\n%s", want, view)
		}
	}
}

func TestHelpKeyDoesNotStackAnotherHelpScreen(t *testing.T) {
	model := NewModelWithSnapshot(context.Background(), testSnapshot(), nil)
	model.stack = []Route{{Screen: ScreenOverview}, {Screen: ScreenHelp}}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if len(model.stack) != 2 {
		t.Fatalf("Help key added another route: %+v", model.stack)
	}
}

func TestScreenShortcutsDoNotStackTheCurrentRoute(t *testing.T) {
	for _, tt := range []struct {
		screen Screen
		key    rune
	}{{screen: ScreenNetwork, key: 'n'}, {screen: ScreenCoverage, key: 'c'}} {
		model := NewModelWithSnapshot(context.Background(), testSnapshot(), nil)
		model.stack = []Route{{Screen: ScreenOverview}, {Screen: tt.screen}}
		model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tt.key}})
		if len(model.stack) != 2 {
			t.Errorf("%q on %s added another route: %+v", tt.key, tt.screen, model.stack)
		}
	}
}

func TestPurgeKeyLaunchesHelperFlowOnlyForRedCatalogueApps(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.snapshot = ptr(testSnapshot())
	model.snapshot.Categories[0].Installed = []report.App{{ID: "chrome", Name: "Google Chrome", Grade: catalog.GradeRed}}
	model.stack = []Route{{Screen: ScreenApp, ID: "chrome"}}
	model.loading = false
	var selected string
	model.purgeAction = func(entry string) tea.Cmd {
		selected = entry
		return func() tea.Msg { return nil }
	}
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil || selected != "chrome" {
		t.Fatalf("purge action command=%v entry=%q", cmd, selected)
	}

	model.snapshot.Categories[0].Installed[0].Grade = catalog.GradeAmber
	selected = ""
	_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd != nil || selected != "" {
		t.Fatalf("non-red entry launched purge: command=%v entry=%q", cmd, selected)
	}
}

func ptr[T any](value T) *T { return &value }
