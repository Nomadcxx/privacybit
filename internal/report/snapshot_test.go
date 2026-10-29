package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

func TestBuildPreservesDeduplicatedOriginContextByAppAndFlatpakScope(t *testing.T) {
	set, err := catalog.Load(catalog.LoadOptions{UserFragments: false})
	if err != nil {
		t.Fatal(err)
	}
	entry := set.Entry("brave-origin")
	observations := []detect.Observation{
		{Name: "brave-origin", Kind: detect.KindPackage, Origin: detect.Observed(detect.Origin{Source: "extra", Official: true})},
		{Name: "brave-origin-bin", Kind: detect.KindPackage, Origin: detect.Observed(detect.Origin{Source: "extra", Official: true})},
		{Name: "brave-origin-custom", Kind: detect.KindPackage, Origin: detect.Observed(detect.Origin{Source: "extra", Official: false})},
		{Name: "com.brave.Browser", Kind: detect.KindFlatpak, Scope: "user", Origin: detect.Observed(detect.Origin{Source: "flathub", Official: true})},
		{Name: "com.brave.Browser", Kind: detect.KindFlatpak, Scope: "system", Origin: detect.Observed(detect.Origin{Source: "flathub", Official: true})},
	}
	result := audit.Result{Overall: catalog.GradeGreen, Categories: []audit.CategoryResult{{
		ID: entry.Category, Name: "Browser", Grade: catalog.GradeGreen,
	}}}
	for _, observation := range observations {
		result.Inventory = append(result.Inventory, audit.InstalledEntry{
			Observation: observation, Product: entry.ID, Category: entry.Category, Grade: catalog.GradeGreen,
		})
	}
	snapshot := Build(set, result, nil, network.UnknownResult("not checked"), nil, time.Now(), 0, BuildOptions{})
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Overall struct {
			Grade catalog.Grade `json:"grade"`
		} `json:"overall"`
		Categories []struct {
			Installed []struct {
				Scope   string `json:"scope"`
				Origins []struct {
					Source   string `json:"source"`
					Official bool   `json:"official"`
				} `json:"origins"`
				Shown     int  `json:"origins_shown"`
				Total     int  `json:"origins_total"`
				Truncated bool `json:"origins_truncated"`
			} `json:"installed"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.Overall.Grade != catalog.GradeGreen {
		t.Fatalf("origin context changed overall grade to %q", got.Overall.Grade)
	}
	if len(got.Categories) != 1 || len(got.Categories[0].Installed) != 3 {
		t.Fatalf("app groups = %+v, want a package group and separate Flatpak scopes", got.Categories)
	}
	var packageApp, userCopy, systemCopy *struct {
		Scope   string `json:"scope"`
		Origins []struct {
			Source   string `json:"source"`
			Official bool   `json:"official"`
		} `json:"origins"`
		Shown     int  `json:"origins_shown"`
		Total     int  `json:"origins_total"`
		Truncated bool `json:"origins_truncated"`
	}
	for i := range got.Categories[0].Installed {
		app := &got.Categories[0].Installed[i]
		switch app.Scope {
		case "":
			packageApp = app
		case "user":
			userCopy = app
		case "system":
			systemCopy = app
		}
	}
	if packageApp == nil || packageApp.Total != 2 || packageApp.Shown != 2 || packageApp.Truncated {
		t.Fatalf("package origins = %+v, want two deduplicated status variants", packageApp)
	}
	if got := packageApp.Origins; got[0].Source != "extra" || got[0].Official || got[1].Source != "extra" || !got[1].Official {
		t.Fatalf("package origins = %+v, want stable order and both official states", got)
	}
	if userCopy == nil || systemCopy == nil || len(userCopy.Origins) != 1 || len(systemCopy.Origins) != 1 {
		t.Fatalf("Flatpak copies lost their separate origins: user=%+v system=%+v", userCopy, systemCopy)
	}
}

func TestBuildPreservesOriginContextForUnmatchedApps(t *testing.T) {
	set, err := catalog.Load(catalog.LoadOptions{UserFragments: false})
	if err != nil {
		t.Fatal(err)
	}
	result := audit.Result{
		Overall: catalog.GradeUnknown,
		Inventory: []audit.InstalledEntry{{
			Observation: detect.Observation{Name: "mystery-browser", Kind: detect.KindPackage,
				Origin: detect.Observed(detect.Origin{Source: "community"})},
			Grade: catalog.GradeUnknown,
		}},
	}
	snapshot := Build(set, result, nil, network.UnknownResult("not checked"), nil, time.Now(), 0, BuildOptions{})
	if snapshot.Overall.Grade != catalog.GradeUnknown || len(snapshot.Unmatched) != 1 {
		t.Fatalf("unmatched result = %+v, want one unrated app", snapshot)
	}
	app := snapshot.Unmatched[0]
	if app.Grade != catalog.GradeUnknown || len(app.Origins) != 1 || app.Origins[0] != (AppOrigin{Source: "community"}) {
		t.Fatalf("unmatched app = %+v, want unchanged unrated grade and origin context", app)
	}
}

func TestBuildSanitizesAndBoundsOriginContext(t *testing.T) {
	set, err := catalog.Load(catalog.LoadOptions{UserFragments: false})
	if err != nil {
		t.Fatal(err)
	}
	entry := set.Entry("brave-origin")
	result := audit.Result{Overall: catalog.GradeGreen, Categories: []audit.CategoryResult{{
		ID: entry.Category, Name: "Browser", Grade: catalog.GradeGreen,
	}}}
	for i := range 10 {
		source := fmt.Sprintf("repo-%02d", i)
		if i == 0 {
			source = " [31mééé\nrepo " + strings.Repeat("界", 40)
		}
		result.Inventory = append(result.Inventory, audit.InstalledEntry{
			Product: entry.ID, Category: entry.Category, Grade: catalog.GradeGreen,
			Observation: detect.Observation{Name: "brave-origin", Kind: detect.KindPackage,
				Origin: detect.Observed(detect.Origin{Source: source})},
		})
	}
	snapshot := Build(set, result, nil, network.UnknownResult("not checked"), nil, time.Now(), 0, BuildOptions{})
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Categories []struct {
			Installed []struct {
				Origins []struct {
					Source string `json:"source"`
				} `json:"origins"`
				Shown     int  `json:"origins_shown"`
				Total     int  `json:"origins_total"`
				Truncated bool `json:"origins_truncated"`
			} `json:"installed"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	app := got.Categories[0].Installed[0]
	if len(app.Origins) != 8 || app.Shown != 8 || app.Total != 10 || !app.Truncated {
		t.Fatalf("origin cap = %+v, want 8 shown of 10", app)
	}
	for _, origin := range app.Origins {
		if len(origin.Source) > 80 || !utf8.ValidString(origin.Source) || strings.ContainsAny(origin.Source, "\n\r\x1b") {
			t.Errorf("unsafe origin label %q (%d bytes)", origin.Source, len(origin.Source))
		}
	}
	if app.Origins[0].Source != "[31mééé repo "+strings.Repeat("界", 20)+"…" {
		t.Errorf("sanitized long origin = %q", app.Origins[0].Source)
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
