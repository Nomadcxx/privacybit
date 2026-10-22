// Package render turns catalogue installation metadata into safe instructions
// for a known distribution family. It never runs those instructions.
package render

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Nomadcxx/privacybit/internal/catalog"
)

// Instruction is a copyable command or a vetted upstream URL. Available is
// false when the catalogue has no usable route for this family.
type Instruction struct {
	Command   string
	URL       string
	Available bool
	Reason    string
}

// Install returns the catalogue's route for family, or an honest explanation
// when no route can be rendered.
func Install(family string, entry *catalog.Entry) Instruction {
	if entry == nil {
		return Instruction{Reason: "no catalogue entry was provided"}
	}
	var packages map[string]string
	var upstream string
	if entry.Install != nil {
		packages = entry.Install.Package
		upstream = entry.Install.Upstream
	}
	if pkg := packages[family]; pkg != "" {
		if !safePackageName(pkg) {
			return upstreamInstruction(entry.ID, family, upstream, "catalogue package name is not safe to print")
		}
		if manager := packageManager(family); manager != "" {
			return Instruction{Command: manager + " " + pkg, Available: true}
		}
	}
	if isHTTPS(upstream) {
		reason := fmt.Sprintf("%s is not packaged for %s; use its upstream source", entry.ID, family)
		return Instruction{URL: upstream, Available: true, Reason: reason}
	}
	if upstream != "" {
		return Instruction{Reason: "catalogue upstream URL is not a valid HTTPS URL"}
	}
	return Instruction{Reason: fmt.Sprintf("no install route is known for %s on %s", entry.ID, family)}
}

func upstreamInstruction(id, family, upstream, reason string) Instruction {
	if isHTTPS(upstream) {
		return Instruction{
			URL: upstream, Available: true,
			Reason: fmt.Sprintf("%s has no safe package command for %s; use its upstream source", id, family),
		}
	}
	return Instruction{Reason: reason}
}

func packageManager(family string) string {
	switch family {
	case "arch":
		return "pacman -S"
	case "debian":
		return "apt install"
	case "fedora":
		return "dnf install"
	case "suse":
		return "zypper install"
	case "alpine":
		return "apk add"
	default:
		return ""
	}
}

// ponytail: package names are rendered from the catalogue without querying a
// live repository; a package-source availability check can replace this when
// those sources expose repository metadata.
func safePackageName(name string) bool {
	if len(name) == 0 || len(name) > 128 || !asciiAlphaNum(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !asciiAlphaNum(c) && !strings.ContainsRune("+._:@-", rune(c)) {
			return false
		}
	}
	return true
}

func asciiAlphaNum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func isHTTPS(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}
