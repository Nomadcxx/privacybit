// Package catalog holds the authored table of known product-variants and the
// grades attached to them. The catalogue is data, not code: adding an entry
// must never require a recompile, because a privacy table that rots is worse
// than no privacy table.
//
// The identity model is deliberately flat. One Entry is one concrete
// product-variant and is the only thing that carries a grade, so "brave" and
// "brave-origin" are two entries with two grades rather than one product with a
// variant flag. Family groups variants for display only and never grades.
// Channel and distribution source feed maturity, never the grade.
package catalog

import "fmt"

// Grade is the verdict on a product-variant. The vocabulary is deliberately
// not named after a hue: grade meaning must not live in colour, because a
// colour-only encoding fails a substantial minority of users. The UI renders
// colour, a distinct glyph shape, and a word together.
type Grade string

const (
	// GradeGreen is privacy-respecting by default.
	GradeGreen Grade = "green"
	// GradeAmber is acceptable with caveats, or configurable into good shape.
	// It needs a decision from the user.
	GradeAmber Grade = "amber"
	// GradeRed is privacy-hostile by default, or collects data that cannot
	// practically be avoided.
	GradeRed Grade = "red"
	// GradeUnknown is installed but absent from the catalogue. It is not a
	// penalty and is deliberately not a grade the catalogue may assign.
	GradeUnknown Grade = "unknown"
)

// Valid reports whether g is a grade the catalogue may assign.
func (g Grade) Valid() bool {
	switch g {
	case GradeGreen, GradeAmber, GradeRed:
		return true
	}
	return false
}

// Risk orders grades so that a comparison answers "which is worse". Unknown
// sorts above red because an unrated item has not been shown to be safe, and
// treating it as better would be the under-grade this project treats as its one
// unacceptable error.
func (g Grade) Risk() int {
	switch g {
	case GradeGreen:
		return 0
	case GradeAmber:
		return 1
	case GradeRed:
		return 2
	case GradeUnknown:
		return 3
	}
	return 3
}

// Worse returns the grade with the higher risk.
func Worse(a, b Grade) Grade {
	if b.Risk() > a.Risk() {
		return b
	}
	return a
}

// Word is the text channel of the three-channel grade display. Callers must
// render this even when colour and glyph are available, because those two are
// the channels a colourblind reader cannot use.
func (g Grade) Word() string {
	switch g {
	case GradeGreen:
		return "Good"
	case GradeAmber:
		return "Review"
	case GradeRed:
		return "Risky"
	}
	return "Unrated"
}

// Glyph is the shape channel, chosen to stay distinguishable in greyscale.
func (g Grade) Glyph() string {
	switch g {
	case GradeGreen:
		return "●" // filled circle
	case GradeAmber:
		return "▲" // filled triangle
	case GradeRed:
		return "■" // filled square
	}
	return "?"
}

// ASCII is the fallback for terminals without the glyph shapes. It loses no
// meaning because Word is always rendered alongside it.
func (g Grade) ASCII() string {
	switch g {
	case GradeGreen:
		return "o"
	case GradeAmber:
		return "^"
	case GradeRed:
		return "x"
	}
	return "?"
}

// DetectType names the source a detect rule matches against. It is a closed
// set; an unknown type is a validation error rather than a silently ignored
// rule, because an ignored rule is a false negative that reads as "not
// installed".
type DetectType string

const (
	DetectPackage DetectType = "package" // pacman or dpkg package name, exact
	DetectBinary  DetectType = "binary"  // basename on PATH and the scan roots
	DetectFlatpak DetectType = "flatpak" // flatpak application id
	DetectDesktop DetectType = "desktop" // .desktop basename stem
)

// Valid reports whether t is a detect type the loader understands.
func (t DetectType) Valid() bool {
	switch t {
	case DetectPackage, DetectBinary, DetectFlatpak, DetectDesktop:
		return true
	}
	return false
}

// Confidence orders detect types for the resolution tie-break in Match. Package
// and flatpak are primary evidence: they are named by the distribution and mean
// what they say. Binary and desktop are corroboration, because a binary named
// "signal" is not necessarily Signal Desktop.
func (t DetectType) Confidence() int {
	switch t {
	case DetectPackage, DetectFlatpak:
		return 2
	case DetectBinary, DetectDesktop:
		return 1
	}
	return 0
}

// Detect is one match rule. Name is matched exactly; a substring rule is not
// supported, because substring matching is where name collisions turn into
// confidently wrong grades.
type Detect struct {
	Type DetectType `yaml:"type" json:"type"`
	Name string     `yaml:"name" json:"name"`
}

func (d Detect) String() string { return fmt.Sprintf("%s:%s", d.Type, d.Name) }

// Install describes how to obtain an entry on a given distribution family.
// Package is keyed by D6 distribution id_like values ("arch", "debian",
// "fedora", "suse", "alpine"). An absent key is meaningful: brave-origin is
// genuinely not packaged for Debian, and the renderer must say so rather than
// emit a command that cannot work.
type Install struct {
	Package  map[string]string `yaml:"package,omitempty" json:"package,omitempty"`
	Upstream string            `yaml:"upstream,omitempty" json:"upstream,omitempty"`
}

// Entry is one product-variant and the unit of grading.
type Entry struct {
	ID           string   `yaml:"id" json:"id"`
	Name         string   `yaml:"name" json:"name"`
	Grade        Grade    `yaml:"grade" json:"grade"`
	Family       string   `yaml:"family,omitempty" json:"family,omitempty"`
	Summary      string   `yaml:"summary" json:"summary"`
	Reasons      []string `yaml:"reasons,omitempty" json:"reasons,omitempty"`
	Caveats      []string `yaml:"caveats,omitempty" json:"caveats,omitempty"`
	Alternatives []string `yaml:"alternatives,omitempty" json:"alternatives,omitempty"`
	Detects      []Detect `yaml:"detects" json:"detects"`
	Install      *Install `yaml:"install,omitempty" json:"install,omitempty"`

	// Category is assigned by the loader from the containing file and is not
	// part of the authored YAML. Repeating it per entry would be a chance for
	// the data and its location to disagree.
	Category string `yaml:"-" json:"category"`

	// Overridden records that a user fragment replaced this entry. The UI must
	// show the catalogue grade alongside an override, so that a user changing a
	// grade never does it silently (D1 §5.1).
	Overridden bool `yaml:"-" json:"overridden,omitempty"`

	// CatalogueGrade is the shipped grade, retained after an override.
	CatalogueGrade Grade `yaml:"-" json:"catalogue_grade,omitempty"`
}

// Expectation is a condition a category wants satisfied. An unmet expectation
// is a finding in its own right: absence is a finding, not a pass. A machine
// with no password manager is red, not empty.
type Expectation struct {
	ID       string   `yaml:"id" json:"id"`
	AnyOf    []string `yaml:"any_of" json:"any_of"`
	Severity string   `yaml:"severity" json:"severity"`
	Why      string   `yaml:"why" json:"why"`
}

// Category groups entries and declares what it expects to find.
type Category struct {
	ID           string        `yaml:"id" json:"id"`
	Name         string        `yaml:"name" json:"name"`
	Summary      string        `yaml:"summary,omitempty" json:"summary,omitempty"`
	Expectations []Expectation `yaml:"expectations,omitempty" json:"expectations,omitempty"`
	Entries      []Entry       `yaml:"entries" json:"entries"`
}

// file is the on-disk shape of one catalogue document.
type file struct {
	Category Category `yaml:"category"`
}

// Set is a loaded, merged, validated catalogue.
type Set struct {
	Revision   string
	Categories []*Category

	byID     map[string]*Entry
	byDetect map[string][]*Entry

	// fragments counts user fragments merged into this set, for the revision.
	fragments int
}

// Entry returns the entry with the given id, or nil.
func (s *Set) Entry(id string) *Entry {
	if s == nil {
		return nil
	}
	return s.byID[id]
}

// Category returns the category with the given id, or nil.
func (s *Set) Category(id string) *Category {
	if s == nil {
		return nil
	}
	for _, c := range s.Categories {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// Entries returns every entry in catalogue order.
func (s *Set) Entries() []*Entry {
	if s == nil {
		return nil
	}
	out := make([]*Entry, 0, len(s.byID))
	for _, c := range s.Categories {
		for i := range c.Entries {
			out = append(out, &c.Entries[i])
		}
	}
	return out
}

// Candidates returns every entry that claims the given detect string, in
// confidence order, highest first. More than one result is a catalogue defect
// rather than a user problem; the validator rejects it at load time so it can
// still be reported if a fragment slips through.
func (s *Set) Candidates(key string) []*Entry {
	if s == nil {
		return nil
	}
	return s.byDetect[key]
}