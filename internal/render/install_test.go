package render

import (
	"testing"

	"github.com/Nomadcxx/privacybit/internal/catalog"
)

func TestInstallUsesDistributionPackageManager(t *testing.T) {
	entry := catalog.Entry{
		ID: "brave-origin",
		Install: &catalog.Install{Package: map[string]string{
			"arch":   "brave-origin",
			"debian": "brave-browser",
			"fedora": "brave-browser",
		}},
	}

	for _, tc := range []struct {
		family string
		want   string
	}{
		{"arch", "pacman -S brave-origin"},
		{"debian", "apt install brave-browser"},
		{"fedora", "dnf install brave-browser"},
	} {
		t.Run(tc.family, func(t *testing.T) {
			got := Install(tc.family, &entry)
			if !got.Available || got.Command != tc.want || got.URL != "" {
				t.Fatalf("Install(%q) = %+v, want command %q", tc.family, got, tc.want)
			}
		})
	}
}

func TestInstallUsesHTTPSUpstreamWhenNotPackaged(t *testing.T) {
	entry := catalog.Entry{
		ID:      "brave-origin",
		Install: &catalog.Install{Package: map[string]string{"arch": "brave-origin"}, Upstream: "https://brave.com/download"},
	}
	got := Install("debian", &entry)
	if !got.Available || got.Command != "" || got.URL != "https://brave.com/download" || got.Reason == "" {
		t.Fatalf("Install for unlisted distro = %+v, want upstream route with explanation", got)
	}
}

func TestInstallDoesNotRenderUnsafePackageOrURL(t *testing.T) {
	entry := catalog.Entry{
		ID: "brave-origin",
		Install: &catalog.Install{
			Package:  map[string]string{"arch": "brave; touch /tmp/pwned"},
			Upstream: "javascript:alert(1)",
		},
	}
	got := Install("arch", &entry)
	if got.Available || got.Command != "" || got.URL != "" || got.Reason == "" {
		t.Fatalf("Install with unsafe route = %+v, want unavailable with a reason", got)
	}
}

func TestInstallExplainsMissingRoute(t *testing.T) {
	entry := catalog.Entry{ID: "unmapped-app"}
	got := Install("debian", &entry)
	if got.Available || got.Command != "" || got.URL != "" || got.Reason == "" {
		t.Fatalf("Install without route = %+v, want unavailable with a reason", got)
	}
}

func TestEveryShippedPackageRouteRenders(t *testing.T) {
	set, err := catalog.Load(catalog.LoadOptions{})
	if err != nil {
		t.Fatalf("load catalogue: %v", err)
	}
	for _, entry := range set.Entries() {
		if entry.Install == nil {
			continue
		}
		for family := range entry.Install.Package {
			got := Install(family, entry)
			if !got.Available || got.Command == "" {
				t.Errorf("install route for %s on %s = %+v", entry.ID, family, got)
			}
		}
	}
}
