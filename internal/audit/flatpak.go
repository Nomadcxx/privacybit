package audit

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/detect"
)

// SandboxApp is one installed Flatpak copy graded on its effective permissions.
type SandboxApp struct {
	ID      string
	Scope   string
	Grade   catalog.Grade
	Grants  []string
	Unknown []string
}

// SandboxResult keeps sandbox posture separate from catalogue app grades.
type SandboxResult struct {
	Applicable      bool
	Grade           catalog.Grade
	Apps            []SandboxApp
	AppsTotal       int
	AppsChecked     int
	AppsIncomplete  int
	PartialCoverage bool
	Reason          string
}

// sandboxPermissions grades the effective context reported by Flatpak.
// ponytail: Keep the supported permission allowlist narrow; add a permission only with an explicit risk band, leaving new grants unknown.
func sandboxPermissions(observation detect.Observation) SandboxApp {
	app := SandboxApp{ID: observation.Name, Scope: observation.Scope, Grade: catalog.GradeUnknown}
	permission := observation.SandboxPermissions
	if permission.State != detect.StateObserved {
		why := strings.Join(strings.Fields(permission.Why), " ")
		if why == "" {
			why = "effective permissions were not available"
		}
		app.Unknown = []string{boundedSandboxText(why)}
		return app
	}

	sections, valid := parseFlatpakPermissionOutput(permission.Value)
	if !valid {
		app.Unknown = []string{"permission output could not be parsed"}
		return app
	}
	context, ok := sections["Context"]
	if !ok {
		app.Unknown = []string{"permission output has no Context section"}
		return app
	}
	for section, entries := range sections {
		if section == "Environment" || len(entries) == 0 {
			continue
		}
		if section != "Context" {
			app.Unknown = appendUnique(app.Unknown, "unclassified permission section")
		}
	}

	red, amber := false, false
	for key, value := range context {
		values, valid := flatpakValues(value)
		if !valid {
			app.Unknown = appendUnique(app.Unknown, "permission output contains an unparsed value")
			continue
		}
		if len(values) == 0 {
			continue
		}
		for _, value := range values {
			grade, label, known := classifyFlatpakPermission(key, value)
			if label != "" {
				app.Grants = appendUnique(app.Grants, label)
			}
			if !known {
				app.Unknown = appendUnique(app.Unknown, "unclassified "+flatpakPermissionName(key)+" permission")
				continue
			}
			switch grade {
			case catalog.GradeRed:
				red = true
			case catalog.GradeAmber:
				amber = true
			}
		}
	}

	switch {
	case len(app.Unknown) > 0:
		app.Grade = catalog.GradeUnknown
	case red:
		app.Grade = catalog.GradeRed
	case amber:
		app.Grade = catalog.GradeAmber
	default:
		app.Grade = catalog.GradeGreen
	}
	sort.Strings(app.Grants)
	sort.Strings(app.Unknown)
	return app
}

func evaluateFlatpakSandbox(observations []detect.Observation, coverage []SourceCoverage) SandboxResult {
	result := SandboxResult{Grade: catalog.GradeUnknown}
	for _, observation := range observations {
		if observation.Kind != detect.KindFlatpak {
			continue
		}
		app := sandboxPermissions(observation)
		result.Applicable = true
		result.AppsTotal++
		if observation.SandboxPermissions.State == detect.StateObserved {
			result.AppsChecked++
		}
		if app.Grade == catalog.GradeUnknown || len(app.Unknown) > 0 {
			result.AppsIncomplete++
			result.PartialCoverage = true
		}
		result.Apps = append(result.Apps, app)
	}
	if result.Applicable {
		result.Grade = catalog.GradeGreen
		for _, app := range result.Apps {
			result.Grade = catalog.Worse(result.Grade, app.Grade)
		}
		sort.Slice(result.Apps, func(i, j int) bool {
			if result.Apps[i].Grade.Risk() != result.Apps[j].Grade.Risk() {
				return result.Apps[i].Grade.Risk() > result.Apps[j].Grade.Risk()
			}
			if result.Apps[i].ID != result.Apps[j].ID {
				return result.Apps[i].ID < result.Apps[j].ID
			}
			return result.Apps[i].Scope < result.Apps[j].Scope
		})
		return result
	}

	for _, source := range coverage {
		if source.Type != catalog.DetectFlatpak {
			continue
		}
		switch {
		case source.Availability.Ready && (source.Availability.Empty || source.Observed == 0):
			result.Reason = "No Flatpak applications are installed."
		case source.Availability.Reason == "flatpak is not installed":
			result.Reason = "Flatpak is not installed."
		default:
			result.Applicable = true
			result.PartialCoverage = true
			result.Reason = boundedSandboxText(source.Availability.Reason)
			if result.Reason == "" {
				result.Reason = "Flatpak inventory could not be checked."
			}
		}
		return result
	}
	result.Applicable = true
	result.PartialCoverage = true
	result.Reason = "Flatpak inventory coverage was not reported."
	return result
}

func parseFlatpakPermissionOutput(output string) (map[string]map[string]string, bool) {
	sections := make(map[string]map[string]string)
	var section string
	var scanner bufio.Scanner
	scanner = *bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") || len(line) < 3 {
				return nil, false
			}
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section == "" || sections[section] != nil {
				return nil, false
			}
			sections[section] = make(map[string]string)
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || section == "" || key == "" {
			return nil, false
		}
		if _, exists := sections[section][key]; exists {
			return nil, false
		}
		sections[section][key] = strings.TrimSpace(value)
	}
	if scanner.Err() != nil {
		return nil, false
	}
	return sections, true
}

func flatpakValues(value string) ([]string, bool) {
	if strings.ContainsRune(value, '\\') {
		return nil, false
	}
	var values []string
	for _, item := range strings.Split(value, ";") {
		item = strings.TrimSpace(item)
		if item != "" {
			values = append(values, item)
		}
	}
	return values, true
}

func classifyFlatpakPermission(key, value string) (catalog.Grade, string, bool) {
	switch key {
	case "shared":
		switch value {
		case "network":
			return catalog.GradeRed, "shared network", true
		case "ipc":
			return catalog.GradeGreen, "", true
		}
	case "devices":
		switch value {
		case "all":
			return catalog.GradeRed, "all devices", true
		case "usb":
			return catalog.GradeAmber, "USB access", true
		case "dri", "shm":
			return catalog.GradeGreen, "", true
		}
	case "sockets":
		switch value {
		case "x11", "fallback-x11":
			return catalog.GradeAmber, "X11 access", true
		case "wayland":
			return catalog.GradeGreen, "", true
		}
	case "filesystems":
		return classifyFlatpakFilesystem(value)
	}
	return catalog.GradeUnknown, "", false
}

func classifyFlatpakFilesystem(value string) (catalog.Grade, string, bool) {
	path, mode, hasMode := strings.Cut(value, ":")
	grade, label, known := classifyFlatpakFilesystemPath(path)
	if hasMode {
		for _, flag := range strings.Split(mode, ":") {
			switch flag {
			case "ro", "rw", "create", "noatime", "xattr":
			default:
				return catalog.GradeUnknown, label, false
			}
		}
	}
	if path == "/var/lib/flatpak" {
		if mode == "ro" {
			return catalog.GradeGreen, "", true
		}
		return catalog.GradeUnknown, "", false
	}
	return grade, label, known
}

func classifyFlatpakFilesystemPath(path string) (catalog.Grade, string, bool) {
	switch path {
	case "host":
		return catalog.GradeRed, "host filesystem", true
	case "home", "~":
		return catalog.GradeAmber, "home files", true
	case "host-etc":
		return catalog.GradeAmber, "host configuration files", true
	}
	if grade, label, known := classifyXDGFilesystem(path); known {
		return grade, label, true
	}
	if home, err := os.UserHomeDir(); err == nil {
		if relative, ok := pathRelativeToHome(path, home); ok {
			return classifyHomeFilesystem(relative)
		}
	}
	if strings.HasPrefix(path, "~/") {
		return classifyHomeFilesystem(filepath.Clean(strings.TrimPrefix(path, "~/")))
	}
	return catalog.GradeUnknown, "", false
}

func classifyXDGFilesystem(path string) (catalog.Grade, string, bool) {
	switch path {
	case "xdg-desktop", "xdg-documents", "xdg-download", "xdg-music", "xdg-pictures", "xdg-public-share", "xdg-templates", "xdg-videos", "xdg-config", "xdg-data", "xdg-cache":
		return catalog.GradeAmber, "user files", true
	default:
		return catalog.GradeUnknown, "", false
	}
}

func pathRelativeToHome(path, home string) (string, bool) {
	if !filepath.IsAbs(path) {
		return "", false
	}
	relative, err := filepath.Rel(filepath.Clean(home), filepath.Clean(path))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", false
	}
	return relative, true
}

func classifyHomeFilesystem(relative string) (catalog.Grade, string, bool) {
	switch filepath.Clean(relative) {
	case ".themes", ".icons", ".local/share/themes", ".local/share/icons", ".local/share/flatpak":
		return catalog.GradeGreen, "", true
	case ".", ".var/app", ".config", ".local/share", "Documents", "Downloads", "Desktop", "Music", "Pictures", "Public", "Templates", "Videos":
		return catalog.GradeAmber, "home files", true
	default:
		return catalog.GradeUnknown, "", false
	}
}

func flatpakPermissionName(key string) string {
	switch key {
	case "filesystems":
		return "filesystem"
	case "sockets":
		return "socket"
	case "devices":
		return "device"
	case "shared":
		return "shared"
	default:
		return "Context"
	}
}

func boundedSandboxText(value string) string {
	var clean strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) {
			clean.WriteByte(' ')
		} else {
			clean.WriteRune(r)
		}
	}
	value = strings.Join(strings.Fields(clean.String()), " ")
	runes := []rune(value)
	if len(runes) > 240 {
		return string(runes[:240])
	}
	return value
}
