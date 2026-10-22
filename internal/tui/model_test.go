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
