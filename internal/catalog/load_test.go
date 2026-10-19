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