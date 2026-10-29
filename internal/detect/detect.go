// Package detect enumerates the software installed on a machine, through
// several independent sources, each declaring up front which facts it can
// support.
//
// The design constraint that shapes everything here is that a fact which nobody
// looked at must never be read as a fact which was looked at and found clean.
// dpkg-query is installed on Arch and manages nothing; a source that reports
// "no licences found" would let a Debian user's FOSS browser read as green
// because the backend had nothing to say. So every fact is a three-state
// Signal, and unavailable reduces reported coverage without ever moving a
// grade in either direction.
package detect

import "context"

// SignalState is whether a fact was observed, could not be looked at, or was
// looked at and contradicted the catalogue.
type SignalState uint8

const (
	// StateZero is the zero value of Signal and is deliberately not
	// StateObserved. A Signal that was never populated must not read as a
	// passing observation, so every constructor sets the state explicitly.
	StateZero SignalState = iota

	// StateObserved means the source looked and has a value.
	StateObserved

	// StateUnavailable means the source cannot support this fact here. It
	// reduces coverage. It is never a pass.
	StateUnavailable

	// StateContradicted means the source looked and disagreed with the
	// catalogue, which is evidence toward risk and nothing else.
	StateContradicted
)

func (s SignalState) String() string {
	switch s {
	case StateObserved:
		return "observed"
	case StateUnavailable:
		return "unavailable"
	case StateContradicted:
		return "contradicted"
	default:
		return "unset"
	}
}

// Signal is a fact that may be unavailable. The zero value is not false and
// not empty: it is nobody looked, and the audit must never read it as a pass.
type Signal[T any] struct {
	State SignalState
	Value T
	// Why is populated whenever State is StateUnavailable, so coverage can
	// explain itself rather than merely report a hole.
	Why string
}

// Observed returns an observed signal carrying v.
func Observed[T any](v T) Signal[T] {
	return Signal[T]{State: StateObserved, Value: v}
}

// Unavailable returns a signal this source cannot support, and why.
func Unavailable[T any](why string) Signal[T] {
	return Signal[T]{State: StateUnavailable, Why: why}
}

// Contradicted returns a signal that was looked at and disagreed with the
// catalogue.
func Contradicted[T any](v T, why string) Signal[T] {
	return Signal[T]{State: StateContradicted, Value: v, Why: why}
}

// IsObserved reports whether the signal is an ordinary observation. A
// contradicted signal is also measured evidence, with its risk direction
// carried by its state.
func (s Signal[T]) IsObserved() bool { return s.State == StateObserved }

// IsMeasured reports whether the source observed a value, either ordinarily or
// in contradiction with the catalogue. Unavailable and unset signals are not
// evidence and cannot move a grade.
func (s Signal[T]) IsMeasured() bool {
	return s.State == StateObserved || s.State == StateContradicted
}

// ValueOr returns the measured value, or def when the signal is unavailable or
// unset.
func (s Signal[T]) ValueOr(def T) T {
	if s.IsMeasured() {
		return s.Value
	}
	return def
}

// Availability reports whether a source can run here. Availability means data,
// not a binary on $PATH: dpkg-query is installed on Arch and manages nothing.
type Availability struct {
	Ready bool
	// Reason explains why not Ready.
	Reason string
	// Empty is installed and working with nothing to report. It is not a
	// coverage gap: a machine can legitimately have no flatpaks.
	Empty bool
}

// Ready returns an available, non-empty source.
func Available() Availability { return Availability{Ready: true} }

// ReadyEmpty returns an available source with nothing to report.
func ReadyEmpty() Availability { return Availability{Ready: true, Empty: true} }

// NotReady returns an unavailable source and the reason.
func NotReady(why string) Availability { return Availability{Reason: why} }

// Capabilities declares which facts a source can support. A source that lists
// packages but cannot report licences says so here, so the audit treats
// licences as unavailable rather than absent.
type Capabilities struct {
	Version     bool
	Licence     bool
	Vendor      bool
	Explicit    bool
	Origin      bool
	ReverseDeps bool
	Signed      bool
}

// FactCapability names one fact a source may or may not provide.
type FactCapability struct {
	Name      string
	Supported bool
}

// Facts returns the source's supported facts in a stable order.
func (c Capabilities) Facts() []FactCapability {
	return []FactCapability{
		{Name: "explicit", Supported: c.Explicit},
		{Name: "licence", Supported: c.Licence},
		{Name: "origin", Supported: c.Origin},
		{Name: "reverse_deps", Supported: c.ReverseDeps},
		{Name: "signed", Supported: c.Signed},
		{Name: "vendor", Supported: c.Vendor},
		{Name: "version", Supported: c.Version},
	}
}

// Origin is provenance: which repository, remote, or nowhere.
type Origin struct {
	// Source is the distribution or remote name, e.g. "core", "extra",
	// "aur", "flathub".
	Source string
	// Official is true when Source matches a recognized official label. The
	// detector does not authenticate the configured repository or remote.
	// Origin is context only and never changes a privacy grade.
	Official bool
}

// Facts are the per-observation attributes a source may supply. Every field is
// a Signal because every field may be unavailable.
type Facts struct {
	Licence     Signal[string]
	Vendor      Signal[string]
	Explicit    Signal[bool]
	ReverseDeps Signal[[]string]
	Signed      Signal[bool]
}

// Observation is one piece of software as one source saw it.
type Observation struct {
	// Name is the source-native identifier: an arch package name, a deb, a
	// flatpak application id.
	Name string

	// Kind is which sort of thing this is, for display and for choosing the
	// right renderer.
	Kind Kind

	// Version is where the source has one.
	Version Signal[string]

	// Product is the catalogue entry this resolves to. Empty means unknown,
	// never a bad grade: software the table has no opinion about is
	// unrated, not bad.
	Product string

	// Origin is provenance.
	Origin Signal[Origin]

	// Branch is the channel a packaged app came from: a flatpak branch, a
	// desktop entry's type. Empty where the source has no such concept.
	Branch string

	// Scope is where an application is installed, such as system or user.
	// Empty where the source has no installation scopes.
	Scope string

	// SandboxPermissions carries the effective Flatpak context returned for this
	// installed copy. Unavailable means the query failed or was not run.
	SandboxPermissions Signal[string]

	// NetworkObservable reports whether this source can attribute a process's
	// outbound connections to it at all. A false value means the source cannot
	// see: an app that unshares its network namespace may be sending anything,
	// and observing nothing from it is evidence about this source rather than
	// about the app.
	//
	// The zero value is false, so a source that forgets to set it produces the
	// conservative answer: absence is not evidence. That is the direction this
	// project refuses to get wrong.
	NetworkObservable bool

	Facts Facts
}

// Kind classifies an observation.
type Kind uint8

const (
	KindPackage Kind = iota
	KindFlatpak
	KindDesktop
	KindBinary
)

func (k Kind) String() string {
	switch k {
	case KindFlatpak:
		return "flatpak"
	case KindDesktop:
		return "desktop"
	case KindBinary:
		return "binary"
	default:
		return "package"
	}
}

// Source is one way of seeing what is installed. It reports observations and
// declares what it can support; it does not answer questions and it does not
// grade.
type Source interface {
	// Name identifies the source in coverage reporting.
	Name() string

	// Availability reports whether this source can run here, and if not why.
	Availability() Availability

	// Capabilities declares which facts this source can supply.
	Capabilities() Capabilities

	// Inventory enumerates what this source currently sees.
	Inventory(ctx context.Context) ([]Observation, error)
}
