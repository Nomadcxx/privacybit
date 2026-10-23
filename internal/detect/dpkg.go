package detect

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Dpkg is the Debian backend, and it is the reference case for the invariant
// that shapes this package.
//
// dpkg records no licence, no vendor, and no signature state. A naive reader
// treats "the licence field is absent" as "no licence found", and then a
// package the catalogue calls FOSS passes its cross-check and reads as clean.
// That would hand a Debian user a green browser because the backend had
// nothing to say, which is exactly the under-grade this project refuses to
// produce. So dpkg declares Licence, Vendor and Signed unavailable, always,
// with the reason attached, and the cross-check simply does not run there.
type Dpkg struct {
	Runner Runner
}

// NewDpkg returns a dpkg source using r.
//
// Unlike Pacman there is no matches callback. dpkg-query reports versions in
// one listing call, and apt-mark reports explicit-install state in one bulk
// query; neither needs per-package enrichment.
func NewDpkg(r Runner) *Dpkg {
	return &Dpkg{Runner: r}
}

// Name identifies the source in coverage reporting.
func (d *Dpkg) Name() string { return "dpkg" }

// Capabilities declares what dpkg and apt-mark can supply. Explicit-install
// state is unavailable on systems where apt-mark is missing or cannot run.
func (d *Dpkg) Capabilities() Capabilities {
	return Capabilities{
		Version:     true,
		Explicit:    true, // from apt-mark showmanual when available
		Origin:      false,
		Licence:     false,
		Vendor:      false,
		ReverseDeps: false,
		Signed:      false,
	}
}

// Availability is the three-state check, and this backend is why it exists.
//
// dpkg-query is installed on the development machine, works correctly, and
// manages no packages at all, because it is not a Debian system. A boolean
// presence check would report it as a working source contributing zero facts,
// which reads identically to "we looked and there is nothing installed".
func (d *Dpkg) Availability() Availability {
	if _, err := d.Runner.LookPath("dpkg-query"); err != nil {
		return NotReady("dpkg-query is not installed")
	}
	out, err := d.Runner.Run(context.Background(), "dpkg-query", "-W")
	if err != nil {
		// dpkg-query exits 1 when no packages match the selection. On a
		// real Debian host that is not possible, but on a foreign host it
		// is exactly the empty-database case, and reporting it as empty is
		// more accurate than reporting it as broken.
		if isNoPackages(err) || strings.Contains(err.Error(), "no packages found") {
			return ReadyEmpty()
		}
		return NotReady("dpkg-query -W failed: " + err.Error())
	}
	if len(lines(out)) == 0 {
		return ReadyEmpty()
	}
	return Available()
}

// Inventory lists installed packages.
//
// apt-mark showmanual distinguishes packages selected by the user from
// automatic dependencies. dpkg's binary:Package field retains architecture
// qualifiers for exact apt-mark matching while Name stays catalogue-friendly.
// If that query fails, package inventory remains useful and explicit-install
// state stays unavailable.
func (d *Dpkg) Inventory(ctx context.Context) ([]Observation, error) {
	out, err := d.Runner.Run(ctx, "dpkg-query", "-W",
		"-f=${Package}\t${binary:Package}\t${Version}\n")
	if err != nil {
		if isNoPackages(err) || strings.Contains(err.Error(), "no packages found") {
			return nil, nil
		}
		return nil, err
	}
	manualOutput, manualErr := d.Runner.Run(ctx, "apt-mark", "showmanual")
	manual := make(map[string]struct{})
	if manualErr == nil {
		for _, name := range lines(manualOutput) {
			manual[name] = struct{}{}
		}
	}
	explicitWhy := "apt-mark showmanual failed"
	if manualErr != nil {
		explicitWhy += ": " + manualErr.Error()
	}
	const (
		licenceWhy = "dpkg records no licence; cross-checking the catalogue is impossible here"
		vendorWhy  = "dpkg records no vendor or packager"
		signedWhy  = "dpkg -W does not report signature validation state"
	)
	var obs []Observation
	for _, l := range lines(out) {
		fields := strings.Split(l, "\t")
		if len(fields) < 3 {
			continue
		}
		name, binaryName, version := fields[0], fields[1], fields[2]
		explicit := Unavailable[bool](explicitWhy)
		if manualErr == nil {
			_, isManual := manual[binaryName]
			explicit = Observed(isManual)
		}
		o := Observation{
			Name:    name,
			Kind:    KindPackage,
			Version: Observed(version),
			Origin:  Unavailable[Origin]("dpkg -W does not report the source archive"),
			Facts: Facts{
				Licence:     Unavailable[string](licenceWhy),
				Vendor:      Unavailable[string](vendorWhy),
				Explicit:    explicit,
				ReverseDeps: Unavailable[[]string]("dpkg -W does not report reverse dependencies"),
				Signed:      Unavailable[bool](signedWhy),
			},
		}
		obs = append(obs, o)
	}
	return obs, nil
}

// isNoPackages reports whether err is dpkg's "nothing matched" exit rather
// than a real failure. dpkg signals an empty selection with a non-zero status,
// so the exit code alone cannot distinguish the two.
func isNoPackages(err error) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 1
	}
	return false
}
