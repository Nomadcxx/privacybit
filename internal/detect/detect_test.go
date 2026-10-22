package detect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestZeroSignalIsNotObserved is the property the whole detection layer exists
// to guarantee. A Signal that was never populated has a zero state, and the
// zero state must not compare equal to an observed value. If it did, every
// source that forgot to set a field would be reporting that the field is
// clean, which is the one answer this project refuses to give.
func TestZeroSignalIsNotObserved(t *testing.T) {
	var s Signal[string]
	if s.State == StateObserved {
		t.Fatal("the zero state of Signal must not be StateObserved")
	}
	if s.IsObserved() {
		t.Fatal("a zero Signal reported itself as observed")
	}
}

func TestSignalStatesAreDistinct(t *testing.T) {
	seen := map[SignalState]string{}
	for _, s := range []SignalState{StateZero, StateObserved, StateUnavailable, StateContradicted} {
		if prev, dup := seen[s]; dup {
			t.Fatalf("state %d is shared by %s and another case", s, prev)
		}
		seen[s] = "case"
	}
	if Observed("x").IsObserved() != true {
		t.Fatal("Observed must report itself observed")
	}
	if Unavailable[string]("no licence metadata").IsObserved() {
		t.Fatal("Unavailable must not report itself observed")
	}
	// ValueOr must never surface the value of a signal that was not measured,
	// or a caller that skipped the state check still gets the fallback.
	if got := Unavailable[string]("nothing").ValueOr("fallback"); got != "fallback" {
		t.Fatalf("Unavailable.ValueOr returned %q, want the fallback", got)
	}
}

func TestContradictedSignalIsMeasuredEvidence(t *testing.T) {
	if !Observed("expected value").IsMeasured() {
		t.Fatal("an observed signal must count as measured evidence")
	}
	s := Contradicted("unknown license", "catalogue expects a FOSS license")
	if !s.IsMeasured() {
		t.Fatal("a contradicted signal contains a measured fact")
	}
	if s.IsObserved() {
		t.Fatal("a contradicted signal must remain distinct from an ordinary observation")
	}
	if got := s.ValueOr("unavailable"); got != "unknown license" {
		t.Fatalf("ValueOr returned %q, want the measured contradictory value", got)
	}
	if Unavailable[string]("not supported").IsMeasured() {
		t.Fatal("an unavailable signal must not count as measured evidence")
	}
}

// TestUnfixturedCommandNeverLooksEmpty pins the guard on MapRunner. Returning
// empty output for a command a test did not record would make an unfixtured
// source indistinguishable from a source that looked and found nothing.
func TestUnfixturedCommandNeverLooksEmpty(t *testing.T) {
	r := MapRunner{}
	out, err := r.Run(context.Background(), "pacman", "-Qq")
	if err == nil {
		t.Fatal("an unfixtured command must return an error")
	}
	if out != "" {
		t.Fatalf("an unfixtured command returned output %q; it must return none", out)
	}
}

// TestAvailabilitySeparatesEmptyFromMissing is D6 §3.3 in executable form: a
// package manager that is installed and manages nothing is Ready and Empty,
// while one that is not installed is NotReady with a reason. Neither is an
// error, and collapsing them would report a coverage gap where there is none.
func TestAvailabilitySeparatesEmptyFromMissing(t *testing.T) {
	r := MapRunner{Output: map[string]string{"pacman -Qq": ""}}
	a := (&Pacman{Runner: r}).Availability()
	if !a.Ready {
		t.Fatal("an installed manager that reports nothing must be Ready")
	}
	if !a.Empty {
		t.Fatal("an installed manager with no packages must report Empty")
	}

	missing := MapRunner{Missing: map[string]bool{"pacman": true}}
	b := (&Pacman{Runner: missing}).Availability()
	if b.Ready {
		t.Fatal("a missing manager must not report Ready")
	}
	if b.Reason == "" {
		t.Fatal("a missing manager must say why")
	}
}

func TestPacmanKeepsPackageWhenEnrichmentFails(t *testing.T) {
	r := MapRunner{Output: map[string]string{"pacman -Qq": "brave\n"}}
	p := NewPacman(r, func(name string) bool { return name == "brave" })
	obs, err := p.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].Name != "brave" {
		t.Fatalf("inventory = %+v, want brave retained after failed -Qi", obs)
	}
	if obs[0].Version.IsObserved() || obs[0].Facts.Licence.IsObserved() {
		t.Fatal("failed enrichment must leave unavailable facts, not observed values")
	}
	if !strings.Contains(obs[0].Facts.Licence.Why, "pacman -Qi failed") {
		t.Fatalf("unavailable licence reason = %q, want the failed detail query", obs[0].Facts.Licence.Why)
	}
}

// TestDpkgReportsNoFactsItCannotSupply is the reference case for the invariant.
// dpkg lists versions and whether a package was explicitly requested, and
// nothing else. Every other fact must be unavailable rather than empty, so the
// reason a licence cannot be cross-checked reaches the person reading it.
func TestDpkgReportsNoFactsItCannotSupply(t *testing.T) {
	out := "bash\t5.2\tsomething-installed\ncoreutils\t9.1\tinstall\n"
	r := MapRunner{Output: map[string]string{
		"dpkg-query -W -f=${Package}\t${Version}\t${db:Status-Status}\n": out,
	}}
	obs, err := NewDpkg(r).Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 2 {
		t.Fatalf("got %d observations, want 2", len(obs))
	}
	if !obs[0].Version.IsObserved() {
		t.Fatal("dpkg does report versions and they must be observed")
	}
	facts := obs[0].Facts
	if facts.Licence.IsObserved() || facts.Vendor.IsObserved() || facts.Signed.IsObserved() {
		t.Fatal("dpkg reported a fact it cannot supply")
	}
	if obs[0].Origin.IsObserved() {
		t.Fatal("dpkg records no repository, so origin cannot be observed")
	}
	if facts.Licence.Why == "" {
		t.Fatal("an unavailable fact must carry a reason the reader can see")
	}
}

func TestDpkgAvailabilityIsNotTheSameAsTheBinaryExisting(t *testing.T) {
	// dpkg-query is installed on this machine's Arch system and manages
	// nothing. Reporting that as a coverage gap would be wrong.
	empty := MapRunner{Output: map[string]string{
		// Availability probes with a bare query; Inventory asks for the
		// formatted listing. They are two calls and need two fixtures.
		"dpkg-query -W": "",
		"dpkg-query -W -f=${Package}\t${Version}\t${db:Status-Status}\n": "",
	}}
	a := NewDpkg(empty).Availability()
	if !a.Ready || !a.Empty {
		t.Fatalf("an installed dpkg-query with no packages must be Ready+Empty, got %+v", a)
	}

	gone := MapRunner{Missing: map[string]bool{"dpkg-query": true}}
	b := NewDpkg(gone).Availability()
	if b.Ready {
		t.Fatal("a missing dpkg-query must not be Ready")
	}
}

func TestFlatpakInventoryPreservesInstallationScope(t *testing.T) {
	listing := "com.example.App\t1.0\tstable\tflathub\tsystem\n" +
		"com.example.App\t1.0\tstable\tflathub\tuser\n"
	r := MapRunner{Output: map[string]string{
		"flatpak list --app --columns=application,version,branch,origin,installation": listing,
	}}
	obs, err := NewFlatpak(r).Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 2 {
		t.Fatalf("got %d observations, want both installation scopes", len(obs))
	}
	if obs[0].Name != obs[1].Name {
		t.Fatalf("duplicate application ids were changed: %q, %q", obs[0].Name, obs[1].Name)
	}
	if obs[0].Scope != "system" || obs[1].Scope != "user" {
		t.Fatalf("installation scopes = %q, %q, want system, user", obs[0].Scope, obs[1].Scope)
	}
}

func TestFlatpakInventoryDoesNotResolveProduct(t *testing.T) {
	r := MapRunner{Output: map[string]string{
		"flatpak list --app --columns=application,version,branch,origin,installation": "com.example.App\t1.0\tstable\tflathub\tuser\n",
	}}
	obs, err := NewFlatpak(r).Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 {
		t.Fatalf("got %d observations, want 1", len(obs))
	}
	if obs[0].Product != "" {
		t.Fatalf("flatpak source assigned catalogue Product %q; matching belongs to the audit", obs[0].Product)
	}
}

// TestDesktopSkipsTombstonesAndNonApplications covers the two desktop entry
// states that mean something other than "this program is installed".
func TestDesktopSkipsTombstonesAndNonApplications(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("real.desktop", "[Desktop Entry]\nType=Application\nName=Real\n")
	write("gone.desktop", "[Desktop Entry]\nType=Application\nHidden=true\n")
	write("link.desktop", "[Desktop Entry]\nType=Link\n")
	write("hidden.desktop", "[Desktop Entry]\nType=Application\nNoDisplay=true\n")

	obs, err := (&Desktop{Roots: []string{dir}}).Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, o := range obs {
		got[o.Name] = true
	}
	if !got["real"] {
		t.Error("an ordinary application entry must be reported")
	}
	// NoDisplay means installed but kept out of menus, which is how a
	// background agent advertises itself, so it still counts.
	if !got["hidden"] {
		t.Error("NoDisplay entries are installed and must still be reported")
	}
	if got["gone"] {
		t.Error("Hidden=true is a tombstone for a deleted application")
	}
	if got["link"] {
		t.Error("a Type=Link entry is not an application")
	}
}

// TestDesktopVersionIsUnavailable is a small honesty check. The Version key in
// a desktop entry is the Desktop Entry specification version, so reporting
// "1.5" as the application's version would be a confidently wrong fact.
func TestDesktopVersionIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.desktop"),
		[]byte("[Desktop Entry]\nType=Application\nVersion=1.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	obs, err := (&Desktop{Roots: []string{dir}}).Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 {
		t.Fatalf("got %d observations, want 1", len(obs))
	}
	if obs[0].Version.IsObserved() {
		t.Fatal("a desktop entry's Version key is not the application version")
	}
	if !strings.Contains(obs[0].Version.Why, "specification") {
		t.Errorf("the reason should say what the Version key actually is, got %q", obs[0].Version.Why)
	}
}

func TestBinaryExcludesNixStoreAndSharedLibraries(t *testing.T) {
	dir := t.TempDir()
	makeExec := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	makeExec("freedoom")
	makeExec("libnotreal.so")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	obs, err := (&Binary{Dirs: []string{dir}}).Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, o := range obs {
		got[o.Name] = true
	}
	if !got["freedoom"] {
		t.Error("an executable must be reported")
	}
	if got["libnotreal.so"] {
		t.Error("a shared library is not an application")
	}
	if got["notes.txt"] {
		t.Error("a non-executable file is not an application")
	}
}

// TestSourcesNeverSetProduct checks that no source assigns itself authority to
// name a catalogue entry. Product is a grading decision; a source that fills it
// in would be reporting a conclusion instead of an observation, and would do it
// with less evidence than the grader has.
func TestSourcesNeverSetProduct(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "brave.desktop"),
		[]byte("[Desktop Entry]\nType=Application\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "brave"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	obs, err := (&Desktop{Roots: []string{dir}}).Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	obs2, err := (&Binary{Dirs: []string{dir}}).Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range append(obs, obs2...) {
		if o.Product != "" {
			t.Errorf("%s source set Product to %q; sources report, they do not grade", o.Kind, o.Product)
		}
	}
}
