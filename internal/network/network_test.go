package network

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

func TestMultipleVerifiedTunnelsAreCheckedAsASet(t *testing.T) {
	runner := detect.MapRunner{Output: map[string]string{
		"ip -o link":                           "1: lo: <LOOPBACK>\n2: tailscale0: <POINTOPOINT,UP>\n3: wg0: <POINTOPOINT,UP>\n",
		"wg show interfaces":                   "wg0\n",
		"resolvectl status":                    "Global\n           resolv.conf mode: stub\n",
		"ip -4 route get 1.1.1.1":              "1.1.1.1 dev tailscale0\n",
		"resolvectl dns":                       "Global: 100.100.100.100\n",
		"resolvectl default-route":             "Global: yes\n",
		"ip route get 100.100.100.100":         "100.100.100.100 dev tailscale0\n",
		"ip -6 route get 2606:4700:4700::1111": "2606:4700:4700::1111 dev tailscale0\n",
		"ip route show default":                "default dev tailscale0\ndefault dev wg0\n",
		"ip -6 route show default":             "",
	}}
	got := inspect(context.Background(), runner, filepath.Join(t.TempDir(), "missing"), []string{"tailscaled"})
	if got.State != Connected || got.Client != "Tailscale, WireGuard" {
		t.Fatalf("tunnel result = %+v, want both clients in stable order", got)
	}
	for name, value := range map[string]*bool{
		"IPv4": got.DefaultRouteViaTunnel,
		"DNS":  got.DNSContained,
		"IPv6": got.IPv6Contained,
	} {
		if value == nil || !*value {
			t.Errorf("%s containment = %v, want true", name, value)
		}
	}
	if len(got.Defects) != 0 {
		t.Errorf("defects = %+v, want no route leaks", got.Defects)
	}
	if got.KillSwitchNote != "No outside default route was observed; firewall rules and all traffic paths were not verified." {
		t.Errorf("kill-switch note = %q, want only physical routes marked outside", got.KillSwitchNote)
	}
}

func TestRouteDNSAndIPv6EscapesBecomeDefects(t *testing.T) {
	runner := detect.MapRunner{Output: map[string]string{
		"ip -o link":                           "1: lo: <LOOPBACK>\n2: tailscale0: <POINTOPOINT,UP>\n3: enp7s0: <BROADCAST,UP>\n",
		"wg show interfaces":                   "",
		"resolvectl status":                    "Global\n           resolv.conf mode: stub\n",
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

func TestResolverWithoutResolvedUsesResolvConfAndRouteEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 100.100.100.100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := detect.MapRunner{Output: map[string]string{
		"ip -o link":                           "1: lo: <LOOPBACK>\n2: tailscale0: <POINTOPOINT,UP>\n",
		"wg show interfaces":                   "",
		"ip route get 100.100.100.100":         "100.100.100.100 dev tailscale0\n",
		"ip -4 route get 1.1.1.1":              "1.1.1.1 dev tailscale0\n",
		"ip -6 route get 2606:4700:4700::1111": "2606:4700:4700::1111 dev tailscale0\n",
		"ip route show default":                "default dev tailscale0\n",
		"ip -6 route show default":             "",
	}, Missing: map[string]bool{"resolvectl": true}}
	got := inspect(context.Background(), runner, path, []string{"tailscaled"})
	if got.DNSContained == nil || !*got.DNSContained {
		t.Fatalf("resolver DNS result without systemd-resolved = %v, want contained by route", got.DNSContained)
	}
	if len(got.Defects) != 0 {
		t.Fatalf("defects = %+v, want none", got.Defects)
	}
}

func TestForeignResolverModePrefersResolvConfOverResolvedDNS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 203.0.113.53\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := detect.MapRunner{Output: map[string]string{
		"ip -o link":                           "1: lo: <LOOPBACK>\n2: tailscale0: <POINTOPOINT,UP>\n3: enp7s0: <BROADCAST,UP>\n",
		"wg show interfaces":                   "",
		"resolvectl status":                    "Global\n           resolv.conf mode: foreign\n",
		"resolvectl dns":                       "Global: 203.0.113.53\nLink 2 (enp7s0): 198.51.100.15 198.51.100.1\n",
		"resolvectl default-route":             "Global: no\nLink 2 (enp7s0): yes\nLink 4 (tailscale0): no\n",
		"ip route get 203.0.113.53":            "203.0.113.53 dev tailscale0\n",
		"ip -4 route get 1.1.1.1":              "1.1.1.1 dev tailscale0\n",
		"ip -6 route get 2606:4700:4700::1111": "2606:4700:4700::1111 dev tailscale0\n",
		"ip route show default":                "default dev tailscale0\n",
		"ip -6 route show default":             "",
	}}

	got := inspect(context.Background(), runner, path, []string{"tailscaled"})
	if got.DNSContained == nil || !*got.DNSContained {
		t.Fatalf("foreign resolver DNS result = %v, want contained by configured resolver route", got.DNSContained)
	}
	if len(got.Defects) != 0 {
		t.Fatalf("defects = %+v, want no VPN leaks", got.Defects)
	}
}

func TestUnknownResolverModeLeavesDNSUnknown(t *testing.T) {
	for _, status := range []string{
		"Global\n",
		"Global\n           resolv.conf mode: future\n",
	} {
		path := filepath.Join(t.TempDir(), "resolv.conf")
		if err := os.WriteFile(path, []byte("nameserver 100.100.100.100\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runner := detect.MapRunner{Output: map[string]string{
			"ip -o link":                           "1: lo: <LOOPBACK>\n2: tailscale0: <POINTOPOINT,UP>\n3: enp7s0: <BROADCAST,UP>\n",
			"wg show interfaces":                   "",
			"resolvectl status":                    status,
			"resolvectl dns":                       "Link 2 (enp7s0): 198.51.100.15\n",
			"resolvectl default-route":             "Link 2 (enp7s0): yes\n",
			"ip route get 198.51.100.15":           "198.51.100.15 dev enp7s0\n",
			"ip -4 route get 1.1.1.1":              "1.1.1.1 dev tailscale0\n",
			"ip -6 route get 2606:4700:4700::1111": "2606:4700:4700::1111 dev tailscale0\n",
			"ip route show default":                "default dev tailscale0\n",
			"ip -6 route show default":             "",
		}}

		got := inspect(context.Background(), runner, path, []string{"tailscaled"})
		if got.DNSContained != nil {
			t.Fatalf("status %q DNS containment = %v, want unknown", status, got.DNSContained)
		}
		if len(got.Defects) != 0 {
			t.Fatalf("status %q defects = %+v, want no unsupported DNS finding", status, got.Defects)
		}
		if len(got.Coverage) == 0 {
			t.Fatalf("status %q has no coverage note for ambiguous resolver ownership", status)
		}
	}
}

func TestDNSUsesResolvConfWhenResolvedSocketIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 100.100.100.100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := detect.MapRunner{
		Output: map[string]string{
			"resolvectl":                   "",
			"ip route get 100.100.100.100": "100.100.100.100 dev tailscale0\n",
		},
		Err: map[string]error{
			"resolvectl status": stderrTextError{resolvedSocketMissingDiagnostic},
		},
	}

	var result Result
	got := checkDNS(context.Background(), runner, path, map[string]string{"tailscale0": "Tailscale"}, &result)
	if got == nil || !*got {
		t.Fatalf("DNS containment = %v, want configured resolver route inside the tunnel", got)
	}
	if len(result.Defects) != 0 {
		t.Fatalf("defects = %+v, want none", result.Defects)
	}
}

func TestDNSStatusFailureKeepsDNSUnknownWhenResolvedServiceMayBeActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 100.100.100.100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := detect.MapRunner{
		Output: map[string]string{
			"resolvectl":                   "",
			"ip route get 100.100.100.100": "100.100.100.100 dev tailscale0\n",
		},
		Err: map[string]error{"resolvectl status": errors.New("status permission denied")},
	}
	var result Result
	got := checkDNS(context.Background(), runner, path, map[string]string{"tailscale0": "Tailscale"}, &result)
	if got != nil {
		t.Fatalf("DNS containment = %v, want unknown for a generic status failure", got)
	}
	if len(result.Coverage) == 0 {
		t.Fatal("missing resolver status failure coverage note")
	}
}

func TestDNSStatusFailureWithExtraSocketTextRemainsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 100.100.100.100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := detect.MapRunner{
		Output: map[string]string{
			"resolvectl":                   "",
			"ip route get 100.100.100.100": "100.100.100.100 dev tailscale0\n",
		},
		Err: map[string]error{
			"resolvectl status": stderrTextError{resolvedSocketMissingDiagnostic + "\nAdditional diagnostic"},
		},
	}
	var result Result
	got := checkDNS(context.Background(), runner, path, map[string]string{"tailscale0": "Tailscale"}, &result)
	if got != nil {
		t.Fatalf("DNS containment = %v, want unknown for non-exact socket error text", got)
	}
	if len(result.Coverage) == 0 {
		t.Fatal("missing resolver status failure coverage note")
	}
}

type stderrTextError struct{ stderr string }

func (e stderrTextError) Error() string { return "status command failed" }

func (e stderrTextError) Unwrap() error {
	return &exec.ExitError{Stderr: []byte(e.stderr)}
}

func TestNoNetworkIsUnknown(t *testing.T) {
	got := UnknownResult("network checks disabled")
	if got.State != Unknown || len(got.Coverage) == 0 {
		t.Fatalf("no-network result = %+v, want explicit unknown coverage", got)
	}
}
