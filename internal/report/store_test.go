package report

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Nomadcxx/privacybit/internal/catalog"
)

func TestStoreRoundTripsPrivateUnreadSnapshot(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	want := Snapshot{
		Schema: Schema, Available: true, GeneratedAt: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC),
		CatalogRev: "catalog-test", Overall: Overall{Grade: catalog.GradeUnknown},
		Sandbox: Sandbox{Grade: catalog.GradeUnknown, Reason: "Flatpak is not installed."},
	}
	if err := store.Save(want, true); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("stored snapshot = %#v, want %#v", *got, want)
	}
	info, err := os.Stat(filepath.Join(store.Dir, reportName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("report mode = %o, want 600", info.Mode().Perm())
	}
	unread, err := store.IsUnread()
	if err != nil || !unread {
		t.Fatalf("unread = %v, error = %v; want true", unread, err)
	}
	if err := store.MarkRead(); err != nil {
		t.Fatal(err)
	}
	unread, err = store.IsUnread()
	if err != nil || unread {
		t.Fatalf("unread after mark-read = %v, error = %v; want false", unread, err)
	}
}

func TestStoreLoadNormalizesLegacySnapshotWithoutRewritingIt(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	path := filepath.Join(store.Dir, reportName)
	data := []byte(`{"schema":1,"available":true,"generated_at":"2026-10-20T00:00:00Z","overall":{"grade":"green"}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Overall.Grade != catalog.GradeUnknown || !got.Sandbox.Applicable || got.Sandbox.Grade != catalog.GradeUnknown {
		t.Fatalf("loaded legacy snapshot = overall %q, sandbox %+v; want in-memory unknown grades", got.Overall.Grade, got.Sandbox)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, data) {
		t.Fatalf("Load rewrote the legacy report:\n got %s\nwant %s", stored, data)
	}
}

func TestStoreRejectsSymlinkAndOversizedReport(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	external := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(external, []byte(`{"schema":1,"available":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(store.Dir, reportName)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("Load accepted a symlink")
	}
	if err := os.Remove(filepath.Join(store.Dir, reportName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, reportName), make([]byte, maxSnapshotBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("Load accepted an oversized report")
	}
}

func TestStoreLockSerializesWriters(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	release, err := store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	started := make(chan struct{})
	acquired := make(chan func(), 1)
	go func() {
		close(started)
		unlock, err := store.Lock()
		if err != nil {
			return
		}
		acquired <- unlock
	}()
	<-started

	select {
	case <-acquired:
		t.Fatal("second writer acquired the report lock before the first released it")
	case <-time.After(25 * time.Millisecond):
	}

	release()
	select {
	case unlock := <-acquired:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("second writer did not acquire the report lock after release")
	}
}

func TestStoreLockRejectsSymlink(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	external := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(external, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(store.Dir, ".scan.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lock(); err == nil {
		t.Fatal("Lock accepted a symlink")
	}
}
