// Package network performs point-in-time VPN routing checks. It never sends
// packets; route lookups inspect the kernel's existing routing decision.
package network

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/Nomadcxx/privacybit/internal/detect"
)

type CheckState string

const (
	Unknown   CheckState = "unknown"
	Connected CheckState = "connected"
)

// Defect is a supported negative finding. Unknown observations are kept as
// coverage notes and never converted into a pass or a defect.
type Defect struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Result is intentionally explicit about absent evidence. Nil booleans mean
// that the check could not judge the condition.
type Result struct {
	State                 CheckState `json:"state"`
	Client                string     `json:"client,omitempty"`
	DefaultRouteViaTunnel *bool      `json:"default_route_via_tunnel,omitempty"`
	DNSContained          *bool      `json:"dns_contained,omitempty"`
	IPv6Contained         *bool      `json:"ipv6_contained,omitempty"`
	KillSwitchNote        string     `json:"kill_switch_note,omitempty"`
	Defects               []Defect   `json:"defects,omitempty"`
	Coverage              []string   `json:"coverage,omitempty"`
}

// Inspect reads routes and resolver configuration without making network
// requests. Unsupported or ambiguous evidence stays unknown.
func Inspect(ctx context.Context, runner detect.Runner) Result {
	return inspect(ctx, runner, "/etc/resolv.conf", processNames())
}

func UnknownResult(reason string) Result {
	return Result{State: Unknown, Coverage: []string{reason}}
}

func inspect(ctx context.Context, runner detect.Runner, resolvConf string, processes []string) Result {
	result := Result{State: Unknown}
	if runner == nil {
		return UnknownResult("network command runner is unavailable")
	}

	linksOut, err := runner.Run(ctx, "ip", "-o", "link")
	if err != nil {
		result.Coverage = append(result.Coverage, "network interfaces could not be read: "+err.Error())
		return result
	}
	links := parseLinks(linksOut)
	wgOut, _ := runner.Run(ctx, "wg", "show", "interfaces")
	wgInterfaces := make(map[string]bool)
	for _, name := range strings.Fields(wgOut) {
		wgInterfaces[name] = true
	}
	tunnel, client := verifiedTunnel(links, wgInterfaces, processes)
	if tunnel == "" {
		result.Coverage = append(result.Coverage, "no tunnel could be verified from an interface and client or WireGuard state")
		result.KillSwitchNote = "Tunnel and kill-switch state could not be verified."
		return result
	}
	result.State = Connected
	result.Client = client

	route4, route4Err := routeInterface(ctx, runner, "-4", "1.1.1.1")
	if route4Err != nil {
		result.Coverage = append(result.Coverage, "IPv4 default route could not be judged: "+route4Err.Error())
	} else {
		result.DefaultRouteViaTunnel = boolPointer(route4 == tunnel)
		if route4 != tunnel {
			result.Defects = append(result.Defects, Defect{
				ID: "vpn.route-bypass", Title: "Default IPv4 route is outside the tunnel",
				Detail: fmt.Sprintf("The route to 1.1.1.1 uses %s, while the verified tunnel is %s.", route4, tunnel),
			})
		}
	}

	result.DNSContained = checkDNS(ctx, runner, resolvConf, tunnel, &result)
	route6, route6Err := routeInterface(ctx, runner, "-6", "2606:4700:4700::1111")
	if route6Err != nil {
		if routeUnreachable(route6Err) {
			result.IPv6Contained = boolPointer(true)
			result.Coverage = append(result.Coverage, "no IPv6 route is configured")
		} else {
			result.Coverage = append(result.Coverage, "IPv6 default route could not be judged: "+route6Err.Error())
		}
	} else {
		result.IPv6Contained = boolPointer(route6 == tunnel)
		if route6 != tunnel {
			result.Defects = append(result.Defects, Defect{
				ID: "vpn.ipv6-bypass", Title: "IPv6 route is outside the tunnel",
				Detail: fmt.Sprintf("The route to 2606:4700:4700::1111 uses %s, while the verified tunnel is %s.", route6, tunnel),
			})
		}
	}

	defaults, defaultErr := runner.Run(ctx, "ip", "route", "show", "default")
	defaults6, default6Err := runner.Run(ctx, "ip", "-6", "route", "show", "default")
	if defaultErr == nil && default6Err == nil {
		outside := false
		for _, line := range append(strings.Split(defaults, "\n"), strings.Split(defaults6, "\n")...) {
			iface := routeDev(line)
			if iface != "" && iface != tunnel {
				outside = true
			}
		}
		if outside {
			result.KillSwitchNote = "An outside default route exists; this route-table check cannot confirm a kill switch."
		} else {
			result.KillSwitchNote = "No outside default route was observed; firewall rules and all traffic paths were not verified."
		}
	} else {
		result.KillSwitchNote = "Kill-switch state could not be judged from the routing table."
	}
	return result
}

func routeUnreachable(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && strings.Contains(strings.ToLower(string(exitErr.Stderr)), "unreachable")
}

func routeInterface(ctx context.Context, runner detect.Runner, family, address string) (string, error) {
	args := []string{"route", "get", address}
	if family != "" {
		args = append([]string{family}, args...)
	}
	out, err := runner.Run(ctx, "ip", args...)
	if err != nil {
		return "", err
	}
	iface := routeDev(out)
	if iface == "" {
		return "", fmt.Errorf("route output did not name an interface")
	}
	return iface, nil
}

func routeDev(output string) string {
	fields := strings.Fields(output)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "dev" {
			return strings.TrimSuffix(fields[i+1], ":")
		}
	}
	return ""
}

func parseLinks(output string) map[string]bool {
	links := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSuffix(fields[1], ":")
		name = strings.SplitN(name, "@", 2)[0]
		if name != "" {
			links[name] = true
		}
	}
	return links
}

func verifiedTunnel(links, wgInterfaces map[string]bool, processes []string) (string, string) {
	clients := make(map[string]string)
	for _, process := range processes {
		name := strings.ToLower(filepath.Base(strings.TrimSpace(process)))
		switch name {
		case "mullvad-daemon", "mullvad":
			clients["mullvad"] = "Mullvad"
		case "openvpn", "openvpn3":
			clients["openvpn"] = "OpenVPN"
		case "protonvpn", "protonvpn-app", "protonvpn-service":
			clients["proton"] = "Proton VPN"
		case "ivpn", "ivpn-service":
			clients["ivpn"] = "IVPN"
		case "windscribe", "windscribe-cli":
			clients["windscribe"] = "Windscribe"
		case "amnezia", "amneziawg-go":
			clients["amnezia"] = "Amnezia"
		case "tailscaled":
			clients["tailscale"] = "Tailscale"
		case "warp-svc":
			clients["warp"] = "Cloudflare WARP"
		}
	}

	for iface := range links {
		if wgInterfaces[iface] {
			return iface, "WireGuard"
		}
		prefixes := []struct{ prefix, key string }{
			{"mullvad", "mullvad"}, {"proton", "proton"}, {"ivpn", "ivpn"},
			{"windscribe", "windscribe"}, {"amnezia", "amnezia"},
			{"tailscale", "tailscale"}, {"warp", "warp"},
		}
		for _, item := range prefixes {
			if strings.HasPrefix(strings.ToLower(iface), item.prefix) && clients[item.key] != "" {
				return iface, clients[item.key]
			}
		}
		if (strings.HasPrefix(iface, "tun") || strings.HasPrefix(iface, "tap")) && clients["openvpn"] != "" {
			return iface, clients["openvpn"]
		}
	}
	return "", ""
}

type resolver struct {
	iface   string
	address net.IP
}

func checkDNS(ctx context.Context, runner detect.Runner, resolvConf, tunnel string, result *Result) *bool {
	serversOut, serversErr := runner.Run(ctx, "resolvectl", "dns")
	defaultsOut, defaultsErr := runner.Run(ctx, "resolvectl", "default-route")
	var resolvers []resolver
	if serversErr == nil && defaultsErr == nil {
		resolvers = activeResolvers(serversOut, defaultsOut)
	}
	if len(resolvers) == 0 {
		// systemd-resolved may be absent or may not own resolv.conf. In that
		// case the active nameservers are the only evidence we can use.
		var err error
		resolvers, err = resolvConfResolvers(resolvConf)
		if err != nil {
			result.Coverage = append(result.Coverage, "resolver configuration could not be read: "+err.Error())
			return nil
		}
		for i := range resolvers {
			if resolvers[i].address.IsLoopback() {
				result.Coverage = append(result.Coverage, "the local DNS stub's upstream could not be read")
				return nil
			}
			out, err := runner.Run(ctx, "ip", "route", "get", resolvers[i].address.String())
			if err != nil {
				result.Coverage = append(result.Coverage, "DNS resolver route could not be judged: "+err.Error())
				return nil
			}
			resolvers[i].iface = routeDev(out)
		}
	}
	if len(resolvers) == 0 {
		result.Coverage = append(result.Coverage, "active DNS resolvers could not be identified")
		return nil
	}
	contained := true
	for _, resolver := range resolvers {
		if resolver.address.IsLoopback() {
			result.Coverage = append(result.Coverage, "the local DNS stub's upstream could not be read")
			return nil
		}
		if resolver.iface == "" {
			iface, err := routeInterface(ctx, runner, "", resolver.address.String())
			if err != nil {
				result.Coverage = append(result.Coverage, "DNS resolver route could not be judged: "+err.Error())
				return nil
			}
			resolver.iface = iface
		}
		if resolver.iface != tunnel {
			contained = false
			result.Defects = append(result.Defects, Defect{
				ID: "vpn.dns-leak", Title: "DNS resolver is outside the tunnel",
				Detail: fmt.Sprintf("An active DNS resolver uses interface %s, outside verified tunnel %s.", resolver.iface, tunnel),
			})
			break
		}
	}
	return boolPointer(contained)
}

func activeResolvers(serversOutput, defaultsOutput string) []resolver {
	defaults := make(map[string]bool)
	globalDefault := false
	for _, line := range strings.Split(defaultsOutput, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Global:") {
			globalDefault = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "Global:")), "yes")
			continue
		}
		if !strings.HasPrefix(line, "Link ") {
			continue
		}
		start := strings.IndexByte(line, '(')
		end := strings.IndexByte(line, ')')
		if start >= 0 && end > start {
			value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line[end+1:]), ":"))
			defaults[line[start+1:end]] = strings.EqualFold(value, "yes")
		}
	}

	var out []resolver
	for _, line := range strings.Split(serversOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		iface := ""
		value := line
		if strings.HasPrefix(line, "Global:") {
			if !globalDefault {
				continue
			}
			value = strings.TrimSpace(strings.TrimPrefix(line, "Global:"))
		} else if strings.HasPrefix(line, "Link ") {
			start := strings.IndexByte(line, '(')
			end := strings.IndexByte(line, ')')
			colon := strings.Index(line, ":")
			if start < 0 || end <= start || colon < end || !defaults[line[start+1:end]] {
				continue
			}
			iface = line[start+1 : end]
			value = strings.TrimSpace(line[colon+1:])
		} else {
			continue
		}
		for _, token := range strings.Fields(value) {
			if ip := net.ParseIP(token); ip != nil {
				out = append(out, resolver{iface: iface, address: ip})
			}
		}
	}
	return out
}

func resolvConfResolvers(path string) ([]resolver, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []resolver
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(strings.SplitN(scanner.Text(), "#", 2)[0])
		if len(fields) != 2 || fields[0] != "nameserver" {
			continue
		}
		if ip := net.ParseIP(fields[1]); ip != nil {
			out = append(out, resolver{address: ip})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func processNames() []string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	uid := uint32(os.Geteuid())
	var names []string
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		path := filepath.Join("/proc", entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (stat.Uid != uid && stat.Uid != 0) {
			continue
		}
		comm, err := os.ReadFile(filepath.Join(path, "comm"))
		if err == nil {
			names = append(names, strings.TrimSpace(string(comm)))
		}
	}
	return names
}

func boolPointer(value bool) *bool { return &value }
