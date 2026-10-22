package report

import (
	"encoding/json"
	"fmt"
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
