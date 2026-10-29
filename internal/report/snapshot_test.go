package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/privacybit/internal/audit"
	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/detect"
	"github.com/Nomadcxx/privacybit/internal/network"
)

func TestBuildBoundsEveryRuntimeList(t *testing.T) {
	set, err := catalog.Load(catalog.LoadOptions{UserFragments: false})
	if err != nil {
		t.Fatal(err)
	}
	result := audit.Result{Overall: catalog.GradeUnknown}
	for i := range 10_000 {
		result.Inventory = append(result.Inventory, audit.InstalledEntry{
			Observation: detect.Observation{Name: fmt.Sprintf("unknown-%05d", i), Kind: detect.KindPackage},
			Grade:       catalog.GradeUnknown,
		})
	}
	findings := make([]audit.Finding, 10_000)
	for i := range findings {
		findings[i] = audit.Finding{ID: fmt.Sprintf("finding-%05d", i), Severity: "low"}
	}
	snapshot := Build(set, result, findings, network.UnknownResult("not checked"), nil, time.Now(), 0, BuildOptions{})
	if len(snapshot.Unmatched) != ListLimit || !snapshot.UnmatchedTruncated || snapshot.UnmatchedTotal != 10_000 {
		t.Fatalf("unmatched bounds = %d/%d truncated=%t", len(snapshot.Unmatched), snapshot.UnmatchedTotal, snapshot.UnmatchedTruncated)
	}
	if len(snapshot.Findings) != FindingLimit || !snapshot.FindingsTruncated || snapshot.FindingsTotal != 10_000 {
		t.Fatalf("finding bounds = %d/%d truncated=%t", len(snapshot.Findings), snapshot.FindingsTotal, snapshot.FindingsTruncated)
	}
	for _, category := range snapshot.Categories {
		if len(category.Installed) != category.InstalledShown || category.InstalledShown > ListLimit {
			t.Fatalf("category %s installed bounds = %d shown %d", category.ID, len(category.Installed), category.InstalledShown)
		}
	}
}

func TestProgressScoreUsesOnlyJudgedCategoryGrades(t *testing.T) {
	score := progressScore([]Category{
		{Grade: catalog.GradeGreen}, {Grade: catalog.GradeAmber},
		{Grade: catalog.GradeRed}, {Grade: catalog.GradeUnknown},
	})
	if score == nil || *score != 50 {
		t.Fatalf("progress score = %v, want 50", score)
	}
	if score := progressScore([]Category{{Grade: catalog.GradeUnknown}}); score != nil {
		t.Fatalf("unjudged progress score = %v, want nil", *score)
	}
}

func TestFlatpakSandboxBandsKeepUnknownAndUseWorstEvidence(t *testing.T) {
	coverage := []audit.SourceCoverage{{
		Name: "flatpak", Type: catalog.DetectFlatpak, Availability: detect.Available(),
	}}
	for _, tc := range []struct {
		name        string
		permissions string
		want        catalog.Grade
	}{
		{"host filesystem", "[Context]\nfilesystems=host;\n", catalog.GradeRed},
		{"all devices", "[Context]\ndevices=all;\n", catalog.GradeRed},
		{"shared network", "[Context]\nshared=network;\n", catalog.GradeRed},
		{"unclassified permission with a known red grant", "[Context]\ndevices=all;\nfeatures=devel;\n", catalog.GradeUnknown},
		{"home files", "[Context]\nfilesystems=home;\n", catalog.GradeAmber},
		{"host config files", "[Context]\nfilesystems=host-etc;\n", catalog.GradeAmber},
		{"unknown mode makes permission evidence unknown", "[Context]\nfilesystems=host:future-flag;\n", catalog.GradeUnknown},
		{"X11", "[Context]\nsockets=x11;\n", catalog.GradeAmber},
		{"USB", "[Context]\ndevices=usb;\n", catalog.GradeAmber},
		{"classified low risk", "[Context]\nshared=ipc;\nsockets=wayland;\ndevices=dri;\n", catalog.GradeGreen},
		{"missing output", "", catalog.GradeUnknown},
		{"unclassified permission", "[Context]\nfeatures=devel;\n", catalog.GradeUnknown},
		{"malformed output", "not a key file", catalog.GradeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			permission := detect.Observed(tc.permissions)
			if tc.permissions == "" {
				permission = detect.Unavailable[string]("permission query failed")
			}
			result := audit.Evaluate(nil, []detect.Observation{{
				Name: "com.example.App", Kind: detect.KindFlatpak,
				Scope: "user", SandboxPermissions: permission,
			}}, coverage)
			if result.Overall != tc.want {
				t.Fatalf("sandbox grade = %q, want %q", result.Overall, tc.want)
			}
		})
	}
}

func TestBuildIncludesSandboxEvidenceInOverallSnapshot(t *testing.T) {
	coverage := []audit.SourceCoverage{{
		Name: "flatpak", Type: catalog.DetectFlatpak, Availability: detect.Available(),
	}}
	result := audit.Evaluate(nil, []detect.Observation{{
		Name: "com.example.Unmatched", Kind: detect.KindFlatpak, Scope: "user",
		SandboxPermissions: detect.Observed("[Context]\ndevices=all;\n[Environment]\nTOKEN=do-not-report-this-secret\n"),
	}}, coverage)
	snapshot := Build(nil, result, nil, network.UnknownResult("not checked"), coverage, time.Now(), 0, BuildOptions{})
	if snapshot.Overall.Grade != catalog.GradeRed {
		t.Fatalf("overall grade = %q, want red from the sandbox evidence", snapshot.Overall.Grade)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Sandbox struct {
			Grade catalog.Grade `json:"grade"`
			Apps  []struct {
				ID     string        `json:"id"`
				Scope  string        `json:"scope"`
				Grade  catalog.Grade `json:"grade"`
				Grants []string      `json:"grants"`
			} `json:"apps"`
			AppsTotal int `json:"apps_total"`
		} `json:"sandbox"`
		Unmatched []struct {
			Name  string        `json:"name"`
			Grade catalog.Grade `json:"grade"`
		} `json:"unmatched"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "do-not-report-this-secret") {
		t.Fatal("raw Flatpak environment values leaked into the snapshot")
	}
	if got.Sandbox.Grade != catalog.GradeRed || got.Sandbox.AppsTotal != 1 || len(got.Sandbox.Apps) != 1 {
		t.Fatalf("sandbox snapshot = %+v, want one red app", got.Sandbox)
	}
	if got.Sandbox.Apps[0].ID != "com.example.Unmatched" || got.Sandbox.Apps[0].Scope != "user" || got.Sandbox.Apps[0].Grade != catalog.GradeRed {
		t.Fatalf("sandbox app = %+v, want the user copy graded red", got.Sandbox.Apps[0])
	}
	if len(got.Sandbox.Apps[0].Grants) != 1 || got.Sandbox.Apps[0].Grants[0] != "all devices" {
		t.Fatalf("sandbox grants = %v, want [all devices]", got.Sandbox.Apps[0].Grants)
	}
	if len(got.Unmatched) != 1 || got.Unmatched[0].Grade != catalog.GradeUnknown {
		t.Fatalf("catalogue state for unmatched app = %+v, want unknown", got.Unmatched)
	}
}

func TestBuildBoundsPerCopySandboxEvidence(t *testing.T) {
	labels := make([]string, sandboxEvidenceLimit+3)
	for i := range labels {
		labels[i] = fmt.Sprintf("evidence-%d", i)
	}
	result := audit.Result{Sandbox: audit.SandboxResult{
		Applicable: true, Grade: catalog.GradeUnknown, AppsTotal: 1,
		Apps: []audit.SandboxApp{{ID: "com.example.App", Scope: "user", Grade: catalog.GradeUnknown,
			Grants: append([]string(nil), labels...), Unknown: append([]string(nil), labels...)}},
	}}
	snapshot := Build(nil, result, nil, network.UnknownResult("not checked"), nil, time.Now(), 0, BuildOptions{})
	app := snapshot.Sandbox.Apps[0]
	if len(app.Grants) != sandboxEvidenceLimit || !app.GrantsTruncated || app.GrantsTotal != len(labels) {
		t.Fatalf("grant projection = %+v", app)
	}
	if len(app.Unknown) != sandboxEvidenceLimit || !app.UnknownTruncated || app.UnknownTotal != len(labels) {
		t.Fatalf("unknown evidence projection = %+v", app)
	}
}

func TestParseSnapshotRejectsFutureMajorAndAcceptsAdditiveFields(t *testing.T) {
	base := `{"schema":1,"available":true,"generated_at":"2026-10-20T00:00:00Z","overall":{"grade":"unknown"},"future_minor_field":true}`
	if _, err := ParseSnapshot([]byte(base)); err != nil {
		t.Fatalf("additive field rejected: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(base), &data); err != nil {
		t.Fatal(err)
	}
	data["schema"] = 2
	future, _ := json.Marshal(data)
	if _, err := ParseSnapshot(future); err == nil {
		t.Fatal("future major schema accepted")
	}
}
