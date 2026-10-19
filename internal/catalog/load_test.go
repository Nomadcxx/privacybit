package catalog

import (
	"strings"
	"testing"
)

// TestShippedCatalogueLoads is the gate for every data file under data/.
//
// The shipped catalogue is hand-authored, so this is expected to fail while it
// is being written. It is deliberately a single test rather than a table: the
// useful signal is the complete problem list in one run, and Problems.Err()
// aggregates rather than stopping at the first.
func TestShippedCatalogueLoads(t *testing.T) {
	set, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("shipped catalogue does not validate:\n%v", err)
	}
	if got, want := len(set.Categories), 10; got != want {
		t.Errorf("category count = %d, want %d", got, want)
	}
	if set.Revision == "" {
		t.Error("revision is empty; a stored snapshot could not be traced to the table that produced it")
	}
	for _, c := range set.Categories {
		if len(c.Entries) == 0 {
			t.Errorf("category %q has no entries", c.ID)
		}
		for _, e := range c.Entries {
			if e.Category != c.ID {
				t.Errorf("entry %q: Category = %q, want %q", e.ID, e.Category, c.ID)
			}
		}
	}
}

// TestEveryDetectKeyBelongsToOneEntry is D6 §9's rule asserted directly rather
// than only through validate(), so a regression names the duplicate key.
func TestEveryDetectKeyBelongsToOneEntry(t *testing.T) {
	set, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	seen := map[string]string{}
	for _, c := range set.Categories {
		for _, e := range c.Entries {
			for _, d := range e.Detects {
				k := detectKey(d)
				if other, dup := seen[k]; dup {
					t.Errorf("detect key %q claimed by both %q and %q", k, other, e.ID)
					continue
				}
				seen[k] = e.ID
			}
		}
	}
}

// TestAnyOfToleratesUngradedPackages pins the deliberate decision in
// validate.go: an expectation's any_of may name a bare package that the
// catalogue has no entry for.
//
// The failure this refuses is a user with Epiphany installed — a browser
// privacybit has no opinion about — being told they have no browser at all,
// because "browser" only resolves to browsers the table happens to grade.
// Inventing a deficiency is the direction this project will not fail in; the
// table may be over-broad about what counts, never under-broad.
//
// The reverse is the real cost, and it is accepted: a typo becomes a token
// nothing matches, which fails permissively. So this test asserts two things —
// ungraded tokens load clean, and the ones we know about still resolve to
// something real rather than silently matching nothing.
func TestAnyOfToleratesUngradedPackages(t *testing.T) {
	set, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("ungraded any_of tokens must not fail validation: %v", err)
	}

	var tokens, resolved int
	for _, c := range set.Categories {
		for _, ex := range c.Expectations {
			for _, token := range ex.AnyOf {
				tokens++
				if set.Entry(token) != nil {
					resolved++
				}
			}
		}
	}
	if tokens == 0 {
		t.Fatal("no any_of tokens at all; the expectations are not testing anything")
	}
	// The shipped table must still lean on real entries. If this drifts to
	// zero the expectations have stopped referring to the catalogue and are
	// just prose, which is a different (and weaker) thing than it looks.
	if resolved == 0 {
		t.Error("no any_of token resolves to an entry; expectations no longer reference the catalogue")
	}
	t.Logf("any_of tokens: %d total, %d resolve to an entry, %d name ungraded packages", tokens, resolved, tokens-resolved)
}

// TestRedEntriesOfferAnAlternative is the wall-of-shame rule: a red entry with
// no way forward is a scolding, not a to-do list.
func TestRedEntriesOfferAnAlternative(t *testing.T) {
	set, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var missing []string
	for _, c := range set.Categories {
		for _, e := range c.Entries {
			if e.Grade == GradeRed && len(e.Alternatives) == 0 {
				missing = append(missing, e.ID)
			}
		}
	}
	if len(missing) > 0 {
		t.Errorf("red entries with no alternative: %s", strings.Join(missing, ", "))
	}
}
