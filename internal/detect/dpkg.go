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
// Unlike Pacman there is no matches callback, because dpkg has no enrich pass
// to narrow: Version and Explicit are the only facts dpkg -W reports, and both
// are in the one listing call. Carrying a callback here would imply dpkg can be
// asked to widen its own coverage, which it cannot.
func NewDpkg(r Runner) *Dpkg {
	return &Dpkg{Runner: r}
}

// Name identifies the source in coverage reporting.
func (d *Dpkg) Name() string { return "dpkg" }

// Capabilities declares what dpkg can actually supply. Everything absent from
// the -W format is false here, which is what makes the unavailable states
// honest rather than a matter of remembering to fill them in.
func (d *Dpkg) Capabilities() Capabilities {
	return Capabilities{
		Version:     true,
		Explicit:    true, // from the "deinstall" and Section fields
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
// The one fact dpkg can supply beyond the name is whether the package was
// explicitly requested or pulled in as a dependency, which it infers from the
// status field's request state. Licence, vendor and signature state are
// unavailable by construction rather than by omission.
func (d *Dpkg) Inventory(ctx context.Context) ([]Observation, error) {
	out, err := d.Runner.Run(ctx, "dpkg-query", "-W",
		"-f=${Package}\t${Version}\t${db:Status-Status}\n")
	if err != nil {
		if isNoPackages(err) || strings.Contains(err.Error(), "no packages found") {
			return nil, nil
		}
		return nil, err
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
		name, version, status := fields[0], fields[1], fields[2]
		o := Observation{
			Name:    name,
			Kind:    KindPackage,
			Version: Observed(version),
			Origin:  Unavailable[Origin]("dpkg -W does not report the source archive"),
			Facts: Facts{
				Licence:     Unavailable[string](licenceWhy),
				Vendor:      Unavailable[string](vendorWhy),
				Explicit:    Observed(status == "installed"),
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
