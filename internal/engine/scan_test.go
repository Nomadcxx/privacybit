package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Nomadcxx/privacybit/internal/audit"
	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/detect"
	"github.com/Nomadcxx/privacybit/internal/network"
)

type fixtureSource struct {
	name         string
	capabilities detect.Capabilities
	items        []detect.Observation
}

func (s fixtureSource) Name() string                      { return s.name }
func (s fixtureSource) Availability() detect.Availability { return detect.Available() }
func (s fixtureSource) Capabilities() detect.Capabilities { return s.capabilities }
func (s fixtureSource) Inventory(context.Context) ([]detect.Observation, error) {
	return s.items, nil
}

func completeEmptyCoverage() []SourceSpec {
	return []SourceSpec{
		{Type: catalog.DetectPackage, Source: fixtureSource{name: "packages"}},
		{Type: catalog.DetectFlatpak, Source: fixtureSource{name: "flatpak"}},
		{Type: catalog.DetectDesktop, Source: fixtureSource{name: "desktop"}},
		{Type: catalog.DetectBinary, Source: fixtureSource{name: "binary"}},
	}
}

func TestSkippedNetworkDoesNotPassInstalledVPN(t *testing.T) {
	set, err := catalog.Load(catalog.LoadOptions{UserFragments: false})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := Run(context.Background(), set, Options{
		Sources: append([]SourceSpec{{Type: catalog.DetectPackage, Source: fixtureSource{
			name: "packages", items: []detect.Observation{{Name: "mullvad-vpn", Kind: detect.KindPackage}},
		}}}, completeEmptyCoverage()[1:]...),
		Network: false,
		Family:  "arch",
	})
	if err != nil {
		t.Fatal(err)
	}
	var vpn catalog.Grade
	for _, category := range snapshot.Categories {
		if category.ID == "vpn" {
			vpn = category.Grade
		}
	}
	if vpn != catalog.GradeUnknown || snapshot.Overall.Grade != catalog.GradeUnknown {
		t.Fatalf("no-network grades = vpn %q, overall %q, want unknown", vpn, snapshot.Overall.Grade)
	}
	if snapshot.Network.VPN.State != network.Unknown || len(snapshot.Reduced) == 0 {
		t.Fatalf("no-network status = %+v, reduced=%+v", snapshot.Network.VPN, snapshot.Reduced)
	}
}

func TestMissingVPNExpectationRemainsJudgedWithoutTunnel(t *testing.T) {
	set, err := catalog.Load(catalog.LoadOptions{UserFragments: false})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := Run(context.Background(), set, Options{
		Sources: completeEmptyCoverage(),
		Network: false,
		Family:  "arch",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range snapshot.Categories {
		if category.ID == "vpn" {
			if category.Grade == catalog.GradeUnknown {
				t.Fatal("known absence of the expected VPN client became unknown")
			}
			return
		}
	}
	t.Fatal("vpn category missing")
}

func TestNetworkRollupKeepsSandboxGradeInOverall(t *testing.T) {
	result := audit.Result{
		Categories: []audit.CategoryResult{{ID: "browser", Grade: catalog.GradeGreen}},
		Sandbox:    audit.SandboxResult{Applicable: true, Grade: catalog.GradeRed},
	}
	applyNetwork(&catalog.Set{Categories: []*catalog.Category{{ID: "vpn"}}}, &result, network.Result{})
	if result.Overall != catalog.GradeRed {
		t.Fatalf("overall grade after network roll-up = %q, want red from sandbox", result.Overall)
	}
}

func TestRunReportsSourceFactsAndUnsupportedCoverage(t *testing.T) {
	set, err := catalog.Load(catalog.LoadOptions{UserFragments: false})
	if err != nil {
		t.Fatal(err)
	}
	source := fixtureSource{
		name: "dpkg",
		capabilities: detect.Capabilities{
			Version: true, Explicit: true,
		},
		items: []detect.Observation{{Name: "brave-origin", Kind: detect.KindPackage}},
	}
	emptySource := fixtureSource{
		name:         "empty-scan",
		capabilities: detect.Capabilities{Licence: true},
	}
	plainSource := fixtureSource{name: "desktop"}
	snapshot, err := Run(context.Background(), set, Options{
		Sources: []SourceSpec{
			{Type: catalog.DetectPackage, Source: source},
			{Type: catalog.DetectPackage, Source: emptySource},
			{Type: catalog.DetectDesktop, Source: plainSource},
		},
		Network: false, Family: "debian",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Coverage []struct {
			Name  string   `json:"name"`
			Facts []string `json:"facts"`
		} `json:"coverage"`
		Reduced []struct {
			Fact    string   `json:"fact"`
			Sources []string `json:"sources"`
		} `json:"reduced"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Coverage) != 3 || got.Coverage[0].Name != "dpkg" || got.Coverage[1].Name != "empty-scan" || got.Coverage[2].Name != "desktop" {
		t.Fatalf("coverage = %+v, want dpkg, empty-scan, and desktop sources", got.Coverage)
	}
	if want := []string{"explicit", "version"}; len(got.Coverage[0].Facts) != len(want) || got.Coverage[0].Facts[0] != want[0] || got.Coverage[0].Facts[1] != want[1] {
		t.Fatalf("dpkg facts = %v, want %v", got.Coverage[0].Facts, want)
	}
	if got.Coverage[1].Facts == nil || len(got.Coverage[1].Facts) != 1 || got.Coverage[1].Facts[0] != "licence" {
		t.Fatalf("empty-scan facts = %v, want declared licence capability", got.Coverage[1].Facts)
	}
	if got.Coverage[2].Facts == nil || len(got.Coverage[2].Facts) != 0 {
		t.Fatalf("desktop facts = %v, want an empty fact list", got.Coverage[2].Facts)
	}
	found := false
	for _, reduced := range got.Reduced {
		if reduced.Fact == "licence" && len(reduced.Sources) == 2 && reduced.Sources[0] == "desktop" && reduced.Sources[1] == "dpkg" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reduced coverage = %+v, want licence unavailable from desktop and dpkg", got.Reduced)
	}

	withoutCapabilities := source
	withoutCapabilities.capabilities = detect.Capabilities{}
	emptyWithoutCapabilities := emptySource
	emptyWithoutCapabilities.capabilities = detect.Capabilities{}
	baseline, err := Run(context.Background(), set, Options{
		Sources: []SourceSpec{
			{Type: catalog.DetectPackage, Source: withoutCapabilities},
			{Type: catalog.DetectPackage, Source: emptyWithoutCapabilities},
			{Type: catalog.DetectDesktop, Source: plainSource},
		},
		Network: false, Family: "debian",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Overall.Grade != baseline.Overall.Grade {
		t.Fatalf("capability reporting changed overall grade from %q to %q", baseline.Overall.Grade, snapshot.Overall.Grade)
	}
	if !reflect.DeepEqual(snapshot.Categories, baseline.Categories) {
		t.Fatalf("capability reporting changed category or installed-entry grades:\n got: %+v\nwant: %+v", snapshot.Categories, baseline.Categories)
	}
}
