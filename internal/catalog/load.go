package catalog

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// shippedFS is the embedded catalogue.
//
// The files live beside this source rather than at the repository root because
// go:embed can only reach its own package directory and below. That is the one
// deviation from D1 §5.1's layout, and it is forced by the toolchain.
//
//go:embed data/*.yaml
var shippedFS embed.FS

// ConfigDirName is the per-user configuration directory name.
const ConfigDirName = "privacybit"

// UserFragmentDir returns the directory holding user catalogue overrides.
func UserFragmentDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, ConfigDirName, "catalog.d"), nil
}

// LoadOptions controls a load. The zero value loads only the shipped
// catalogue, which is what the helper must do (D4 §4: user fragments may
// override grades in the CLI and may never widen the helper's power).
type LoadOptions struct {
	// UserFragments enables ~/.config/privacybit/catalog.d/*.yaml.
	UserFragments bool
	// Dir overrides the fragment directory, for tests.
	Dir string
}

// Load reads, merges and validates the catalogue.
//
// A malformed user fragment is rejected whole with a readable error, because
// applying half a fragment would produce grades nobody authored.
//
// A malformed *shipped* file is also returned as an error rather than
// panicking. It is a build defect either way, but the shipped table is
// hand-authored, so failing the whole process on the first bad line would hide
// every other problem in the other nine files. Problems.Err() aggregates them
// all into one report instead.
func Load(opts LoadOptions) (*Set, error) {
	s := &Set{
		byID:     map[string]*Entry{},
		byDetect: map[string][]*Entry{},
	}

	shipped, err := loadShipped()
	if err != nil {
		return nil, fmt.Errorf("privacybit: embedded catalogue is invalid: %w", err)
	}
	for _, c := range shipped {
		s.Categories = append(s.Categories, c)
	}

	if opts.UserFragments {
		s.fragments++
		dir := opts.Dir
		if dir == "" {
			dir, err = UserFragmentDir()
			if err != nil {
				return nil, err
			}
		}
		if err := s.mergeFragments(dir); err != nil {
			return nil, err
		}
	}

	s.Revision = s.revision(shipped)
	if err := s.validate(); err != nil {
		return nil, err
	}
	s.index()
	return s, nil
}

// loadShipped parses the embedded documents in filename order, which fixes the
// order `privacybit list` prints.
func loadShipped() ([]*Category, error) {
	names, err := fs.Glob(shippedFS, "data/*.yaml")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	out := make([]*Category, 0, len(names))
	seen := map[string]string{}
	for _, name := range names {
		raw, err := shippedFS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		f, err := parse(raw, name)
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[f.Category.ID]; dup {
			return nil, fmt.Errorf("%s: category %q already defined by %s", name, f.Category.ID, prev)
		}
		seen[f.Category.ID] = name
		c := f.Category
		for i := range c.Entries {
			c.Entries[i].Category = c.ID
		}
		out = append(out, &c)
	}
	return out, nil
}

// parse decodes one YAML document strictly. An unknown field is an error
// rather than a shrug, for the same reason D4 §5 makes unknown JSON fields an
// error: a typo that is silently ignored becomes a rule that never matches,
// which reads as "not installed".
func parse(raw []byte, name string) (file, error) {
	var f file
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return f, fmt.Errorf("%s: %w", name, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, fs.ErrClosed) && err == nil {
		return f, fmt.Errorf("%s: file must contain exactly one document", name)
	}
	if f.Category.ID == "" {
		return f, fmt.Errorf("%s: category id is required", name)
	}
	if f.Category.Name == "" {
		return f, fmt.Errorf("%s: category name is required", name)
	}
	return f, nil
}

// mergeFragments applies user fragments in filename order. An entry whose id
// already exists replaces it in place, preserving its position so output order
// is stable and an override does not silently reorder the table.
func (s *Set) mergeFragments(dir string) error {
	names, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	// Copy the file paths out of Glob's backing array so the documented
	// "must not retain" behaviour of filepath.Glob cannot bite us.
	sort.Strings(names)
	for _, name := range names {
		raw, err := os.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read fragment: %w", err)
		}
		f, err := parse(raw, filepath.Base(name))
		if err != nil {
			return err
		}
		cat := s.Category(f.Category.ID)
		if cat == nil {
			c := f.Category
			for i := range c.Entries {
				c.Entries[i].Category = c.ID
			}
			s.Categories = append(s.Categories, &c)
			continue
		}
		if f.Category.Summary != "" {
			cat.Summary = f.Category.Summary
		}
		for _, want := range f.Category.Expectations {
			replaced := false
			for i := range cat.Expectations {
				if cat.Expectations[i].ID == want.ID {
					cat.Expectations[i] = want
					replaced = true
					break
				}
			}
			if !replaced {
				cat.Expectations = append(cat.Expectations, want)
			}
		}
		for i := range f.Category.Entries {
			e := f.Category.Entries[i]
			e.Category = cat.ID
			if at := indexOfEntry(cat, e.ID); at >= 0 {
				e.Overridden = true
				cat.Entries[at] = e
				continue
			}
			cat.Entries = append(cat.Entries, e)
		}
	}
	return nil
}

func indexOfEntry(c *Category, id string) int {
	for i := range c.Entries {
		if c.Entries[i].ID == id {
			return i
		}
	}
	return -1
}

// revision fingerprints the shipped table plus the number of fragments applied,
// so a stored snapshot can be traced to the table that produced it (D2 §5).
func (s *Set) revision(shipped []*Category) string {
	h := sha256.New()
	for _, c := range shipped {
		fmt.Fprintf(h, "%s\x00", c.ID)
		for i := range c.Entries {
			e := &c.Entries[i]
			fmt.Fprintf(h, "%s\x00%s\x00", e.ID, e.Grade)
		}
	}
	sum := hex.EncodeToString(h.Sum(nil))[:12]
	var frag string
	if n := s.fragmentCount(); n > 0 {
		frag = fmt.Sprintf("+%d", n)
	}
	return "cat-" + sum + frag
}

func (s *Set) fragmentCount() int { return s.fragments }

// index builds the lookup maps. It runs after validation, so it can assume
// every detect string is unique and every grade is valid.
func (s *Set) index() {
	s.byID = map[string]*Entry{}
	s.byDetect = map[string][]*Entry{}
	type cand struct {
		e    *Entry
		conf int
	}
	best := map[string]cand{}
	for _, c := range s.Categories {
		for i := range c.Entries {
			e := &c.Entries[i]
			s.byID[e.ID] = e
			for _, d := range e.Detects {
				k := detectKey(d)
				conf := d.Type.Confidence()
				if prev, ok := best[k]; !ok || conf > prev.conf {
					best[k] = cand{e, conf}
				}
			}
		}
	}
	keys := make([]string, 0, len(best))
	for k := range best {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s.byDetect[k] = []*Entry{best[k].e}
	}
}

func detectKey(d Detect) string {
	return string(d.Type) + "\x00" + strings.ToLower(d.Name)
}
