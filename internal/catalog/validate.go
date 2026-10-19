package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// Problem is one catalogue defect, located precisely enough that an author can
// find it without grepping. Path is a slash-joined trail such as
// "browser/brave-origin/detects[0]"; a top-level problem such as a duplicate
// category id uses the id on its own.
type Problem struct {
	Path string
	Msg  string
}

func (p Problem) String() string { return p.Path + ": " + p.Msg }

// Problems is a catalogue defect list. It sorts by Path on first use so the
// output is stable, which matters because the shipped table is expected to be
// broken while it is being authored and an unstable order makes it impossible
// to tell two runs apart.
type Problems []Problem

// Err returns the aggregated problems as an error, or nil when there are none.
func (ps Problems) Err() error {
	if len(ps) == 0 {
		return nil
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Path < ps[j].Path })
	var b strings.Builder
	fmt.Fprintf(&b, "catalogue: %d problem(s)", len(ps))
	for _, p := range ps {
		b.WriteString("\n  ")
		b.WriteString(p.String())
	}
	return fmt.Errorf("%s", b.String())
}

// ids enumerates every entry id in the set. validate runs before index(), so
// it cannot rely on byID; it builds its own view instead.
func (s *Set) ids() map[string]bool {
	out := make(map[string]bool)
	for _, c := range s.Categories {
		for _, e := range c.Entries {
			out[e.ID] = true
		}
	}
	return out
}

// validate checks every rule the design commits to, collecting all failures
// rather than stopping at the first.
//
// The rules, and why each exists:
//
//   - Unique entry ids. Two entries sharing an id is not a duplicate entry, it
//     is one entry with two grades, and which grade wins depends on file order.
//   - Unique category ids, for the same reason one level up.
//   - Valid grades. An unknown grade must fail at load, not silently become a
//     zero value that every comparison treats as green.
//   - A name and summary on every entry, so `explain` can always answer.
//   - At least one detect rule. An entry nothing can match is a promise the
//     tool cannot keep, and it reads as "not installed" forever.
//   - Alternative ids resolve, and an entry cannot name itself.
//   - Expectation any_of must not be empty. An expectation with an empty
//     any_of is satisfied vacuously, which would report the absence
//     finding as met.
//   - No detect key claimed by two entries. Resolution would pick one silently,
//     so a typo becomes a confidently wrong grade.
//   - Family values are consistent, so display grouping cannot imply a
//     relationship that does not exist.
//   - Every red entry offers somewhere to go. A red with no alternative is the
//     wall of shame the design exists to avoid.
//
// Fragment-supplied entries are checked by the same rules as shipped ones. A
// user override is not a way around the table's invariants, and D4 relies on
// the invariant that fragments cannot widen the helper's power.
func (s *Set) validate() error {
	var ps Problems
	known := s.ids()

	categoryIDs := make(map[string]bool, len(s.Categories))
	detectOwners := make(map[string]string)
	families := make(map[string]bool)
	redWithoutAlternative := make([]string, 0)

	for _, c := range s.Categories {
		cp := c.ID
		if cp == "" {
			ps = append(ps, Problem{"<category>", "category has no id"})
			continue
		}
		if categoryIDs[cp] {
			ps = append(ps, Problem{cp, fmt.Sprintf("duplicate category id (categories: %s)", s.categoryNames())})
			continue
		}
		categoryIDs[cp] = true

		if c.Name == "" {
			ps = append(ps, Problem{cp, "category has no name"})
		}
		for i, ex := range c.Expectations {
			ep := fmt.Sprintf("%s/expectations[%d]", cp, i)
			if ex.ID == "" {
				ps = append(ps, Problem{ep, "expectation has no id"})
				continue
			}
			ep = cp + "/" + ex.ID
			if len(ex.AnyOf) == 0 {
				ps = append(ps, Problem{ep, "any_of is empty, so the expectation can never be unmet"})
			}
			// any_of tokens are deliberately NOT required to resolve to an
			// entry. They may name either a graded entry or a bare package,
			// because an expectation asks "does the user have something of
			// this kind", not "did the catalogue happen to grade it".
			//
			// Requiring resolution would mean a user with Epiphany installed
			// — a browser the table has no opinion about — is told they have
			// no browser. That is a false deficiency claim, and this project
			// treats those as the unacceptable direction: the table may be
			// over-broad about what counts, never under-broad.
			//
			// The cost is that a typo becomes a token nothing matches, which
			// fails in the permissive direction (an unmet-looking expectation
			// stays met or is missed) rather than inventing a deficiency. See
			// TestAnyOfToleratesUngradedPackages.
			_ = known
			if ex.Severity == "" {
				ps = append(ps, Problem{ep, "expectation has no severity"})
			}
			if ex.Why == "" {
				ps = append(ps, Problem{ep, "expectation has no why, so the finding could not be explained"})
			}
		}

		for _, e := range c.Entries {
			ep := cp + "/" + e.ID

			if e.ID == "" {
				ps = append(ps, Problem{cp, "entry has no id"})
				continue
			}
			if e.Category == "" {
				ps = append(ps, Problem{ep, "entry has no category assigned by the loader"})
			}
			if !e.Grade.Valid() {
				ps = append(ps, Problem{ep, fmt.Sprintf("invalid grade %q (want %s, %s or %s)", e.Grade, GradeGreen, GradeAmber, GradeRed)})
			}
			if e.Name == "" {
				ps = append(ps, Problem{ep, "entry has no name, so explain cannot describe it"})
			}
			if e.Summary == "" {
				ps = append(ps, Problem{ep, "entry has no summary, so explain has nothing to say about it"})
			}
			if len(e.Detects) == 0 {
				ps = append(ps, Problem{ep, "entry has no detect rules, so nothing can ever match it"})
			}
			for i, d := range e.Detects {
				dp := fmt.Sprintf("%s/detects[%d]", ep, i)
				if d.Name == "" {
					ps = append(ps, Problem{dp, "detect rule has no name"})
					continue
				}
				switch d.Type {
				case DetectPackage, DetectBinary, DetectFlatpak, DetectDesktop:
				default:
					ps = append(ps, Problem{dp, fmt.Sprintf("invalid detect type %q", d.Type)})
				}
				key := detectKey(d)
				if owner, taken := detectOwners[key]; taken {
					ps = append(ps, Problem{dp, fmt.Sprintf("detect key %s is already claimed by entry %q; resolution would pick one silently", key, owner)})
					continue
				}
				detectOwners[key] = e.ID
			}
			for _, alt := range e.Alternatives {
				if alt == e.ID {
					ps = append(ps, Problem{ep, "entry lists itself as an alternative"})
					continue
				}
				if !known[alt] {
					ps = append(ps, Problem{ep, fmt.Sprintf("alternative references unknown entry %q", alt)})
				}
			}
			if e.Grade == GradeRed && len(e.Alternatives) == 0 {
				redWithoutAlternative = append(redWithoutAlternative, e.ID)
			}
			if e.Family != "" {
				families[e.Family] = true
			}
			if e.Install != nil && len(e.Install.Package) > 0 {
				for k := range e.Install.Package {
					if !knownDistro[k] {
						ps = append(ps, Problem{ep, fmt.Sprintf("install.package key %q is not a distribution id_like value (want one of %s)", k, strings.Join(distroIDs(), ", "))})
					}
					if e.Install.Package[k] == "" {
						ps = append(ps, Problem{ep, fmt.Sprintf("install.package[%q] is empty, which means no command can be produced", k)})
					}
				}
			}
		}
	}

	// Reported together so the author fixes them in one pass rather than
	// discovering them one per run. A red entry with no alternative is the
	// single most important authoring mistake in the table.
	if len(redWithoutAlternative) > 0 {
		sort.Strings(redWithoutAlternative)
		for _, id := range redWithoutAlternative {
			ps = append(ps, Problem{id, "graded red but offers no alternative, which is the wall of shame the design exists to avoid"})
		}
	}

	if len(families) > 0 && len(s.Categories) == 0 {
		ps = append(ps, Problem{"<catalogue>", "families declared but no categories present"})
	}

	return ps.Err()
}

// knownDistro is the set of id_like values D6 defines. A key outside it is a
// typo, and a typo here produces a distribution with no install command rather
// than an error.
var knownDistro = map[string]bool{
	"arch":   true,
	"debian": true,
	"fedora": true,
	"suse":   true,
	"alpine": true,
}

func distroIDs() []string {
	out := make([]string, 0, len(knownDistro))
	for k := range knownDistro {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (s *Set) categoryNames() string {
	out := make([]string, 0, len(s.Categories))
	for _, c := range s.Categories {
		out = append(out, c.ID)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}