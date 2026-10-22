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
