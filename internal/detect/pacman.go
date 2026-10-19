package detect

import (
	"context"
	"strings"
)

// Pacman is the Arch backend and the richest of them: licence, vendor,
// install reason, reverse dependencies and signature-validation state are all
// available, and RequiredBy gives D4's plan-purge the edge it needs.
//
// It is also the design's reference for how to keep the slow tier affordable.
// `pacman -Qq` lists everything in ~100ms; `pacman -Qi` is orders slower and is
// only ever asked about packages the catalogue actually matched. Spending the
// expensive query on all 2,882 packages to learn facts about software the
// table has no opinion about is the whole cost this avoids.
type Pacman struct {
	Runner Runner
	// Matches reports whether the catalogue claims a package.
	Matches func(pkgName string) bool
}

// NewPacman returns a pacman source using r.
//
// matches reports whether the catalogue claims a package. It is consulted once
// per package, and only packages it claims pay for the expensive enrich query.
// A nil matches means no catalogue is available, which degrades to the fast
// tier rather than failing: an unrated machine is a coverage fact, not a
// failure.
func NewPacman(r Runner, matches func(pkgName string) bool) *Pacman {
	return &Pacman{Runner: r, Matches: matches}
}

// Name identifies the source in coverage reporting.
func (p *Pacman) Name() string { return "pacman" }

// Capabilities declares everything pacman can supply.
func (p *Pacman) Capabilities() Capabilities {
	return Capabilities{
		Version: true, Licence: true, Vendor: true,
		Explicit: true, Origin: true, ReverseDeps: true, Signed: true,
	}
}

// Availability reports whether there is a pacman database to read.
//
// This is a data check, not a binary check. dpkg-query's three-state handling
// is the same shape of problem in reverse: a binary that exists is not a
// package database with anything in it.
func (p *Pacman) Availability() Availability {
	if _, err := p.Runner.LookPath("pacman"); err != nil {
		return NotReady("pacman is not installed")
	}
	out, err := p.Runner.Run(context.Background(), "pacman", "-Qq")
	if err != nil {
		// A pacman that cannot list is not a gap in our coverage so much as
		// a broken package manager, and saying so is more useful than
		// claiming no software is installed.
		return NotReady("pacman -Qq failed: " + err.Error())
	}
	if len(lines(out)) == 0 {
		return ReadyEmpty()
	}
	return Available()
}

// Inventory lists installed packages, enriching only catalogue matches.
//
// p.Matches decides who pays for the expensive query.
func (p *Pacman) Inventory(ctx context.Context) ([]Observation, error) {
	out, err := p.Runner.Run(ctx, "pacman", "-Qq")
	if err != nil {
		return nil, err
	}
	var obs []Observation
	for _, name := range lines(out) {
		o := Observation{
			Name:    name,
			Kind:    KindPackage,
			Version: Unavailable[string]("version needs the enrich pass"),
			Origin:  Unavailable[Origin]("origin needs the enrich pass"),
			Facts: Facts{
				Licence:     Unavailable[string]("licence is not in the fast listing"),
				Vendor:      Unavailable[string]("vendor is not in the fast listing"),
				Explicit:    Unavailable[bool]("install reason is not in the fast listing"),
				ReverseDeps: Unavailable[[]string]("reverse dependencies need the enrich pass"),
				Signed:      Unavailable[bool]("signature state is not in the fast listing"),
			},
		}
		if p.Matches != nil && p.Matches(name) {
			if err := p.enrich(ctx, &o); err != nil {
				// One unqueryable package must not abandon the other 2,881.
				// It stays in the inventory with its fast-tier facts and
				// its unfilled ones marked unavailable.
				continue
			}
		}
		obs = append(obs, o)
	}
	return obs, nil
}

// enrich fills in the facts that cost a query, for one package.
//
// Field labels come from pacman's own output: Name, Version, Licenses,
// Packager, RequiredBy, InstallReason, ValidatedBy. Note ValidatedBy is
// pacman-specific signature-validation state with no equivalent elsewhere; it
// is the reason Signed is a per-source fact rather than a universal one.
func (p *Pacman) enrich(ctx context.Context, o *Observation) error {
	out, err := p.Runner.Run(ctx, "pacman", "-Qi", o.Name)
	if err != nil {
		return err
	}
	f := fields(out)
	if v, ok := f["Version"]; ok {
		o.Version = Observed(v)
	}
	if v, ok := f["Licenses"]; ok {
		o.Facts.Licence = Observed(v)
	}
	if v, ok := f["Packager"]; ok {
		o.Facts.Vendor = Observed(v)
	}
	if v, ok := f["RequiredBy"]; ok {
		o.Facts.ReverseDeps = Observed(splitList(v))
	} else {
		o.Facts.ReverseDeps = Observed([]string(nil))
	}
	if v, ok := f["InstallReason"]; ok {
		o.Facts.Explicit = Observed(v == "Explicitly installed")
	}
	// ValidatedBy is present only when a signature check has run. Absent is
	// unavailable rather than false: pacman has not said unsigned, it has
	// not said anything.
	if v, ok := f["ValidatedBy"]; ok {
		o.Facts.Signed = Observed(!strings.HasPrefix(v, "None"))
	} else {
		o.Facts.Signed = Unavailable[bool]("no ValidatedBy field in pacman -Qi output")
	}
	o.Origin = Observed(pacmanOrigin(f))
	return nil
}

// pacmanOrigin classifies the repository field.
//
// AUR is the third-party case on Arch and is a supply-chain finding feeding
// maturity. The official repositories carry numeric prefixes in pacman's
// output, so "extra" is official and "aur" is not.
func pacmanOrigin(f map[string]string) Origin {
	repo, ok := f["Repository"]
	if !ok || repo == "" {
		return Origin{Source: "none", Official: false}
	}
	o := Origin{Source: repo}
	if repo == "aur" || strings.HasPrefix(repo, "aur/") {
		o.Official = false
		return o
	}
	o.Official = true
	return o
}

// fields parses pacman -Qi output into a label to value map. Records are
// blank-line separated with "Label : value" lines.
func fields(out string) map[string]string {
	f := make(map[string]string)
	for _, l := range lines(out) {
		i := strings.Index(l, " :")
		if i < 0 {
			continue
		}
		f[strings.TrimSpace(l[:i])] = strings.TrimSpace(l[i+2:])
	}
	return f
}

// splitList splits a pacman list field on commas, dropping empties. A field of
// "None" means an empty list rather than a package called None.
func splitList(v string) []string {
	if v == "" || v == "None" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" && p != "None" {
			out = append(out, p)
		}
	}
	return out
}
