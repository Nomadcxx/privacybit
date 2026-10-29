package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/network"
	"github.com/Nomadcxx/privacybit/internal/report"
	tea "github.com/charmbracelet/bubbletea"
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
