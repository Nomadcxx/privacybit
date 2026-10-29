package detect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// binaryDirs returns the directories scanned for standalone executables, most
// specific first. It is deliberately conservative: the system directories hold
// thousands of files that are packages, and finding them again here would
// report the same software several times with less evidence than the package
// manager has.
func binaryDirs() []string {
	dirs := []string{
		"/usr/local/bin",
		"/opt",
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local/bin"))
	}
	return dirs
}

// Binary finds executables installed outside any package manager: tarball
// installs, AppImages, cargo and go install, and manual copies into /opt.
type Binary struct {
	Dirs []string
	// PATH is scanned only for the current user's additions, not the whole
	// system PATH, because the system directories are pacman's or dpkg's to
	// report and this source adds no evidence to them.
	PathDirs []string
}

// NewBinary returns a Binary with the standard search set.
func NewBinary() *Binary {
	return &Binary{
		Dirs:     binaryDirs(),
		PathDirs: userPathDirs(),
	}
}

func (b *Binary) Name() string { return "binary" }

func (b *Binary) Availability() Availability {
	dirs := b.dirs()
	existing := 0
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			existing++
		}
	}
	switch {
	case existing == 0:
		return NotReady("none of the scanned executable directories exist")
	case countExecutables(b.dirs()) == 0:
		return ReadyEmpty()
	default:
		return Available()
	}
}

func (b *Binary) dirs() []string {
	if len(b.Dirs) > 0 || len(b.PathDirs) > 0 {
		out := append([]string{}, b.Dirs...)
		return append(out, b.PathDirs...)
	}
	return binaryDirs()
}

func (b *Binary) Capabilities() Capabilities {
	// Finding a file proves it exists. It says nothing about where it came
	// from, who wrote it, or whether anyone signed it.
	return Capabilities{}
}

// Inventory lists executables, deduplicated by resolved target.
//
// Product is left empty for the same reason as in the desktop source: the
// filename is a hint, and turning a hint into a grade is the grader's job.
func (b *Binary) Inventory(ctx context.Context) ([]Observation, error) {
	seen := map[string]bool{}
	var out []Observation
	for _, dir := range b.dirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			name := e.Name()
			if !isExecutableCandidate(name, dir, e) {
				continue
			}
			key := resolveTarget(filepath.Join(dir, name))
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, Observation{
				Name:    name,
				Kind:    KindBinary,
				Version: Unavailable[string]("an executable file records no version of its own"),
				Origin:  Unavailable[Origin]("no package manager claims this file, so no repository or remote is known"),
				Facts:   binaryFacts(),
			})
		}
	}
	return out, nil
}

func isExecutableCandidate(name, dir string, e os.DirEntry) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	// Skip AppImages here. They register a desktop entry, which the desktop
	// source already reports, and reporting them twice would let one program
	// count as two.
	if strings.HasSuffix(strings.ToLower(name), ".appimage") {
		return false
	}
	path := filepath.Join(dir, name)
	if e.IsDir() {
		// An AppImage can be run in place as a directory on some systems.
		if strings.HasSuffix(strings.ToLower(name), ".appimage") {
			return stExecutable(path)
		}
		return false
	}
	if strings.HasPrefix(name, "lib") || strings.HasSuffix(name, ".so") {
		return false
	}
	return stExecutable(path)
}

func binaryFacts() Facts {
	why := "a file on disk records none of this"
	return Facts{
		Licence:     Unavailable[string](why),
		Vendor:      Unavailable[string](why),
		Explicit:    Unavailable[bool](why),
		ReverseDeps: Unavailable[[]string](why),
		Signed:      Unavailable[bool](why),
	}
}

// resolveTarget follows symlinks so that a package-managed binary appearing in
// /usr/local/bin through a symlink is only counted once. The nix store is
// excluded: it holds thousands of content-addressed paths that are not
// applications a user chose to install, and counting them would bury the
// handful of things that are.
func resolveTarget(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if strings.HasPrefix(path, "/nix/store/") {
		return "nix:" + filepath.Base(path)
	}
	return path
}

func stExecutable(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	return st.Mode()&0o111 != 0
}

// userPathDirs returns the writable additions to PATH, dropping the system
// directories that a package manager already covers.
func userPathDirs() []string {
	var out []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		switch {
		case pathWithinRoot(dir, "/usr/bin"),
			pathWithinRoot(dir, "/bin"),
			pathWithinRoot(dir, "/usr/sbin"),
			pathWithinRoot(dir, "/sbin"),
			pathWithinRoot(dir, "/usr/local/bin"),
			pathWithinRoot(dir, "/nix/store"),
			pathWithinRoot(dir, "/usr/lib"):
			continue
		}
		out = append(out, dir)
	}
	return out
}

// ponytail: This compares lexical paths; resolve PATH symlinks if aliases to system roots cause duplicate scans.
func pathWithinRoot(path, root string) bool {
	path, root = filepath.Clean(path), filepath.Clean(root)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func countExecutables(dirs []string) int {
	n := 0
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if stExecutable(filepath.Join(dir, e.Name())) {
				n++
			}
		}
	}
	return n
}
