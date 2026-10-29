package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/engine"
	"github.com/Nomadcxx/privacybit/internal/report"
)

func fixtureSnapshot() report.Snapshot {
	return report.Snapshot{
		Schema: report.Schema, Available: true, GeneratedAt: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		Overall:  report.Overall{Grade: catalog.GradeUnknown},
		Coverage: []report.Coverage{{Name: "fixture", Type: "package", Ready: true}},
	}
}

func TestSnapshotFlagEmitsOneTerminalEvent(t *testing.T) {
	var out, stderr bytes.Buffer
	userFragments := false
	called := false
	cmd := newAuditCommand(&userFragments, func(_ context.Context, _ *catalog.Set, opts engine.Options) (report.Snapshot, error) {
		called = true
		if !opts.Network {
			t.Error("network should be enabled by default")
		}
		return fixtureSnapshot(), nil
	})
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	if err := cmd.Flags().Set("snapshot", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("scan was not called")
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("snapshot output has %d events, want one: %s", len(lines), out.String())
	}
	var event struct {
		Type     string          `json:"t"`
		Snapshot report.Snapshot `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "done" || event.Snapshot.Schema != report.Schema {
		t.Fatalf("event = %+v, want one done snapshot", event)
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON mode wrote to stderr: %s", stderr.String())
	}
}

func TestWriteHumanShowsFlatpakSandboxGradeAndCoverage(t *testing.T) {
	snapshot := report.Snapshot{Sandbox: report.Sandbox{
		Applicable: true, Grade: catalog.GradeAmber, AppsTotal: 2,
		AppsChecked: 1, AppsIncomplete: 1, PartialCoverage: true,
		Apps: []report.SandboxApp{
			{ID: "org.example.App", Scope: "user", Grade: catalog.GradeAmber, Grants: []string{"home files"}},
			{ID: "org.example.Other", Scope: "system", Grade: catalog.GradeUnknown, Unknown: []string{"permission query failed"}},
		},
	}}
	var out bytes.Buffer
	if err := writeHuman(&out, snapshot, "low"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Flatpak sandbox", "org.example.App", "user", "home files", "org.example.Other", "system", "Partial coverage"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("human output does not contain %q: %s", want, out.String())
		}
	}
}

func TestWriteHumanShowsOriginsAsContext(t *testing.T) {
	snapshot := report.Snapshot{
		Overall: report.Overall{Grade: catalog.GradeGreen},
		Categories: []report.Category{{
			Name: "Browser", Grade: catalog.GradeGreen, InstalledShown: 1, InstalledTotal: 1,
			Installed: []report.App{{
				Name: "Brave Origin", Grade: catalog.GradeGreen, Scope: "user",
				Origins:      []report.AppOrigin{{Source: "flathub", Official: true}, {Source: "aur"}},
				OriginsShown: 2, OriginsTotal: 3, OriginsTruncated: true,
			}},
		}},
	}
	var out bytes.Buffer
	if err := writeHuman(&out, snapshot, "low"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Origin context", "Brave Origin (user)", "flathub (recognized official label; configured source not verified)", "aur (not marked official)", "Showing 2 of 3 origins"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("human output does not contain %q: %s", want, out.String())
		}
	}
	if !strings.Contains(out.String(), "Overall: o Good") {
		t.Errorf("origin context changed the displayed grade: %s", out.String())
	}
}

func TestWriteHumanShowsUnmatchedOriginsAsUnratedContext(t *testing.T) {
	snapshot := report.Snapshot{
		Overall: report.Overall{Grade: catalog.GradeGreen},
		Unmatched: []report.App{{
			Name: "Mystery browser", Grade: catalog.GradeUnknown,
			Origins: []report.AppOrigin{{Source: "community"}}, OriginsShown: 1, OriginsTotal: 1,
		}},
		UnmatchedTotal: 2, UnmatchedTruncated: true,
	}
	var out bytes.Buffer
	if err := writeHuman(&out, snapshot, "low"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Unmatched apps", "Mystery browser", "Unrated", "Origin context", "community (not marked official)", "Origin context shown for 1 of 1 unmatched apps; 2 total"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("human output does not contain %q: %s", want, out.String())
		}
	}
	if !strings.Contains(out.String(), "Overall: o Good") {
		t.Errorf("unmatched origin context changed the overall grade: %s", out.String())
	}
}

func TestJSONEmitsStartAndDoneAndPassesNoNetworkScope(t *testing.T) {
	var out bytes.Buffer
	userFragments := false
	cmd := newAuditCommand(&userFragments, func(_ context.Context, _ *catalog.Set, opts engine.Options) (report.Snapshot, error) {
		if opts.Network {
			t.Error("--no-network must disable checks")
		}
		if opts.Reporting.MinimumSeverity != "high" {
			t.Errorf("minimum severity = %q, want high", opts.Reporting.MinimumSeverity)
		}
		return fixtureSnapshot(), nil
	})
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	_ = cmd.Flags().Set("json", "true")
	_ = cmd.Flags().Set("no-network", "true")
	_ = cmd.Flags().Set("severity", "high")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSON output has %d events, want start and done: %s", len(lines), out.String())
	}
	var first, last map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &last); err != nil {
		t.Fatal(err)
	}
	if first["t"] != "start" || last["t"] != "done" {
		t.Fatalf("terminal event sequence = %v then %v", first["t"], last["t"])
	}
}

func TestExitCodesPreserveAuditAndPolicyFailures(t *testing.T) {
	if got := exitCode(commandError{code: 10, err: errors.New("policy")}); got != 10 {
		t.Fatalf("policy exit = %d, want 10", got)
	}
	if got := exitCode(commandError{code: 2, err: errors.New("audit")}); got != 2 {
		t.Fatalf("audit exit = %d, want 2", got)
	}
	if got := exitCode(errors.New("usage")); got != 1 {
		t.Fatalf("default exit = %d, want 1", got)
	}
	if _, enabled := failThreshold(""); enabled {
		t.Fatal("empty fail-on unexpectedly enabled a threshold")
	}
}
