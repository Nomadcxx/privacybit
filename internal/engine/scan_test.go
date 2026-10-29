package engine

import (
	"context"
	"testing"

	"github.com/Nomadcxx/privacybit/internal/audit"
	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/detect"
	"github.com/Nomadcxx/privacybit/internal/network"
)

type fixtureSource struct {
	name  string
	items []detect.Observation
}

func (s fixtureSource) Name() string                      { return s.name }
func (s fixtureSource) Availability() detect.Availability { return detect.Available() }
func (fixtureSource) Capabilities() detect.Capabilities   { return detect.Capabilities{} }
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
