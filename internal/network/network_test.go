package network

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/privacybit/internal/detect"
)

func TestTunnelInterfaceAloneDoesNotClaimConnected(t *testing.T) {
	runner := detect.MapRunner{Output: map[string]string{
		"ip -o link":         "1: lo: <LOOPBACK>\n2: tun0: <POINTOPOINT,UP>\n",
		"wg show interfaces": "",
	}}
	got := inspect(context.Background(), runner, filepath.Join(t.TempDir(), "missing"), nil)
	if got.State != Unknown || len(got.Defects) != 0 {
		t.Fatalf("unverified tunnel result = %+v, want unknown and no defects", got)
	}
}

func TestRouteDNSAndIPv6EscapesBecomeDefects(t *testing.T) {
	runner := detect.MapRunner{Output: map[string]string{
		"ip -o link":                           "1: lo: <LOOPBACK>\n2: tailscale0: <POINTOPOINT,UP>\n3: enp7s0: <BROADCAST,UP>\n",
		"wg show interfaces":                   "",
		"ip -4 route get 1.1.1.1":              "1.1.1.1 via 192.168.0.2 dev enp7s0 src 192.168.0.10\n",
		"resolvectl dns":                       "Global: 100.100.100.100\nLink 2 (enp7s0): 192.168.0.15 1.1.1.1\n",
		"resolvectl default-route":             "Global: no\nLink 2 (enp7s0): yes\nLink 3 (tailscale0): no\n",
		"ip -6 route get 2606:4700:4700::1111": "2606:4700:4700::1111 dev enp7s0\n",
		"ip route show default":                "default via 192.168.0.2 dev enp7s0\n",
		"ip -6 route show default":             "",
	}}
	got := inspect(context.Background(), runner, filepath.Join(t.TempDir(), "missing"), []string{"tailscaled"})
	if got.State != Connected || got.Client != "Tailscale" {
		t.Fatalf("tunnel state = %+v, want verified Tailscale", got)
	}
	if got.DefaultRouteViaTunnel == nil || *got.DefaultRouteViaTunnel {
		t.Errorf("IPv4 tunnel route = %v, want false", got.DefaultRouteViaTunnel)
	}
	if got.DNSContained == nil || *got.DNSContained {
		t.Errorf("DNS containment = %v, want false", got.DNSContained)
	}
	if got.IPv6Contained == nil || *got.IPv6Contained {
		t.Errorf("IPv6 containment = %v, want false", got.IPv6Contained)
	}
	if len(got.Defects) != 3 {
		t.Fatalf("defects = %+v, want route, DNS, and IPv6 defects", got.Defects)
	}
	if got.KillSwitchNote == "" || got.KillSwitchNote == "active" {
		t.Errorf("kill-switch note = %q, want a caveat rather than a pass", got.KillSwitchNote)
	}
}

func TestForeignResolverUsesResolvConfAndRouteEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 100.100.100.100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := detect.MapRunner{Output: map[string]string{
		"ip -o link":                           "1: lo: <LOOPBACK>\n2: tailscale0: <POINTOPOINT,UP>\n",
		"wg show interfaces":                   "",
		"resolvectl dns":                       "",
		"resolvectl default-route":             "",
		"ip route get 100.100.100.100":         "100.100.100.100 dev tailscale0\n",
		"ip -4 route get 1.1.1.1":              "1.1.1.1 dev tailscale0\n",
		"ip -6 route get 2606:4700:4700::1111": "2606:4700:4700::1111 dev tailscale0\n",
		"ip route show default":                "default dev tailscale0\n",
		"ip -6 route show default":             "",
	}}
	got := inspect(context.Background(), runner, path, []string{"tailscaled"})
	if got.DNSContained == nil || !*got.DNSContained {
		t.Fatalf("foreign resolver DNS result = %v, want contained by route", got.DNSContained)
	}
	if len(got.Defects) != 0 {
		t.Fatalf("defects = %+v, want none", got.Defects)
	}
}

func TestNoNetworkIsUnknown(t *testing.T) {
	got := UnknownResult("network checks disabled")
	if got.State != Unknown || len(got.Coverage) == 0 {
		t.Fatalf("no-network result = %+v, want explicit unknown coverage", got)
	}
}
