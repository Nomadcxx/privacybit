package detect

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
)

// desktopRoots are searched in order. The flatpak export directories come
// after the system and user ones so a system entry is reported first when the
// same application is visible through both.
func desktopRoots() []string {
	roots := []string{
		"/usr/share/applications",
		"/usr/local/share/applications",
		"/var/lib/flatpak/exports/share/applications",
		"/var/lib/snapd/desktop/applications",
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots,
			filepath.Join(home, ".local/share/applications"),
			filepath.Join(home, ".local/share/flatpak/exports/share/applications"),
		)
	}
	return roots
}

// Desktop reads *.desktop files. It is the one source that works the same way
// on every distribution, which is why it is the fallback when no native
// package backend has been written for the machine it is running on.
type Desktop struct {
	Roots []string
}

// NewDesktop returns a Desktop using the standard search roots.
func NewDesktop() *Desktop { return &Desktop{Roots: desktopRoots()} }

func (d *Desktop) Name() string { return "desktop" }

// Availability reports whether any search root exists at all. An absent root is
// not an error: most of these directories do not exist on most machines, and a
// machine with no user-installed launchers is not a coverage gap.
func (d *Desktop) Availability() Availability {
	roots := d.Roots
	if roots == nil {
		roots = desktopRoots()
	}
	found := false
	for _, root := range roots {
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			found = true
			break
		}
	}
	if !found {
		return NotReady("no application directory exists on this machine")
	}
	if n := countDesktopFiles(roots); n == 0 {
		return ReadyEmpty()
	}
	return Available()
}

func (d *Desktop) Capabilities() Capabilities {
	// A desktop entry names an application and nothing else. It carries no
	// licence, no vendor, no repository and no install reason, and it does not
	// say whether the package is installed or only a launcher left behind.
	return Capabilities{}
}

// Inventory reads every visible desktop entry.
//
// Product is left empty on purpose. The entry's basename is a hint, not a
// match: a launcher file called signal.desktop is a reasonable inference, but
// resolving it to a catalogue entry is the grader's job, and doing it here
// would make this source authoritative about something it only guesses at.
func (d *Desktop) Inventory(ctx context.Context) ([]Observation, error) {
	roots := d.Roots
	if roots == nil {
		roots = desktopRoots()
	}
	var out []Observation
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".desktop") {
				continue
			}
			if !d.visible(filepath.Join(root, name)) {
				continue
			}
			stem := strings.TrimSuffix(name, ".desktop")
			// The flatpak export directories repeat applications that the
			// flatpak source already reports, with the application id as the
			// stem. Reporting them here would double every flatpak app.
			if isFlatpakExportRoot(root) && strings.Count(stem, ".") >= 2 {
				continue
			}
			out = append(out, Observation{
				Name:    stem,
				Kind:    KindDesktop,
				Version: Unavailable[string]("the Version key in a desktop entry is the Desktop Entry specification version, not the application's version"),
				Origin:  Unavailable[Origin]("a desktop entry records no repository or remote"),
				Facts:   desktopFacts(),
			})
		}
	}
	return out, nil
}

func isFlatpakExportRoot(root string) bool {
	return strings.Contains(root, "flatpak/exports")
}

// visible reports whether an entry should count as installed. Hidden=true means
// the file is a tombstone for a deleted application. NoDisplay=true means the
// application is installed but deliberately kept out of menus, which is still
// installed and is the more common way a background agent advertises itself.
func (d *Desktop) visible(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Hidden":
			if strings.EqualFold(strings.TrimSpace(value), "true") {
				return false
			}
		case "Type":
			// Only applications are applications. A .desktop describing a
			// link or a directory is not a running program.
			if v := strings.TrimSpace(value); v != "" && v != "Application" {
				return false
			}
		}
	}
	return true
}

func desktopFacts() Facts {
	why := "a desktop entry records none of this"
	return Facts{
		Licence:     Unavailable[string](why),
		Vendor:      Unavailable[string](why),
		Explicit:    Unavailable[bool](why),
		ReverseDeps: Unavailable[[]string](why),
		Signed:      Unavailable[bool](why),
	}
}

func countDesktopFiles(roots []string) int {
	n := 0
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".desktop") {
				n++
			}
		}
	}
	return n
}
