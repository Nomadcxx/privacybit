package detect

import (
	"context"
	"strings"
)

// Flatpak reads installed flatpak applications.
//
// The distinguishing problem with flatpak is that an application may unshare
// its network namespace, in which case its outbound connections are not
// attributable to it from outside. Observing no telemetry from such an
// application is not evidence that it sends none: it is evidence that this
// source cannot see. Both are the same thing to a naive reader, and the naive
// reading is the under-grade. So NetworkObservable is a property of the
// observation rather than a comment on it, and a caller that wants to say
// "nothing seen" has to check it first.
type Flatpak struct {
	Runner Runner
	// perms maps application id to its effective permission summary, obtained
	// from `flatpak info --show-permissions`. Absent means not checked.
	perms map[string]string
}

// NewFlatpak returns a flatpak source using r.
func NewFlatpak(r Runner) *Flatpak {
	return &Flatpak{Runner: r, perms: map[string]string{}}
}

// Name identifies the source in coverage reporting.
func (f *Flatpak) Name() string { return "flatpak" }

// Capabilities reports what flatpak can answer. It reports a version and an
// origin. It reports no licence, no vendor and no signature state, because
// flatpak records none of those in a way that maps onto a package fact.
func (f *Flatpak) Capabilities() Capabilities {
	return Capabilities{Version: true, Origin: true}
}

// Availability asks flatpak whether it manages any applications. An installed
// flatpak with no applications is Ready and Empty, which is a healthy machine
// rather than a coverage gap.
func (f *Flatpak) Availability() Availability {
	if _, err := f.Runner.LookPath("flatpak"); err != nil {
		return NotReady("flatpak is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	out, err := f.Runner.Run(ctx, "flatpak", "list", "--app",
		"--columns=application,version,branch,origin,installation")
	if err != nil {
		return NotReady("flatpak list failed: " + err.Error())
	}
	if len(lines(out)) == 0 {
		return ReadyEmpty()
	}
	return Available()
}

// parseColumns splits a tab-separated `flatpak list` row into its columns.
func parseColumns(line string) []string {
	return strings.Split(strings.TrimRight(line, "\n"), "\t")
}

// Inventory reports every installed flatpak application.
//
// Origin distinguishes a system installation (root-owned) from a user one. The
// same application id installed at both is two copies at two privilege levels,
// and the user copy shadows the system one for that user. That is a real
// finding rather than a duplicate to be silently deduplicated, so both rows are
// reported and the ids are left equal.
func (f *Flatpak) Inventory(ctx context.Context) ([]Observation, error) {
	out, err := f.Runner.Run(ctx, "flatpak", "list", "--app",
		"--columns=application,version,branch,origin,installation")
	if err != nil {
		return nil, err
	}
	var obs []Observation
	for _, line := range lines(out) {
		cols := parseColumns(line)
		if len(cols) < 5 {
			continue
		}
		id, version, branch, origin, installation := cols[0], cols[1], cols[2], cols[3], cols[4]
		scope := "system"
		if strings.HasPrefix(installation, "user") {
			scope = "user"
		}
		official := origin == "flathub" || origin == "flathub-beta"
		obs = append(obs, Observation{
			Name:       id,
			Kind:       KindFlatpak,
			Scope:      scope,
			Version:    Observed(version),
			Branch:     branch,
			Origin:     Observed(Origin{Source: origin, Official: official}),
			Permissive: f.perms[id],
			Facts: Facts{
				Licence:     Unavailable[string]("flatpak does not record a licence in its listing"),
				Vendor:      Observed(origin),
				Explicit:    Observed(true),
				ReverseDeps: Unavailable[[]string]("flatpak has no reverse dependency model"),
				Signed:      Unavailable[bool]("flatpak records signature state per remote, not per application"),
			},
		})
	}
	return obs, nil
}

// PermissionsFor fills the per-application permission map from
// `flatpak info --show-permissions`.
//
// D1 §6.4 requires effective permissions rather than declared ones, because
// `flatpak override` mutates reality and persists invisibly. No bulk command
// lists overrides, so this walks the known ids. That is O(applications) and it
// is why this is a separate call from Inventory: it is the slow part, and a
// caller that only needs presence should not pay for it.
func (f *Flatpak) PermissionsFor(ctx context.Context, ids []string) {
	for _, id := range ids {
		out, err := f.Runner.Run(ctx, "flatpak", "info", "--show-permissions", id)
		if err != nil {
			continue
		}
		f.perms[id] = strings.Join(lines(out), "; ")
	}
}
