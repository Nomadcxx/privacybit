package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/engine"
	"github.com/Nomadcxx/privacybit/internal/report"
	"github.com/Nomadcxx/privacybit/internal/tui"
)

func scheduledSnapshot(at time.Time, grade catalog.Grade) report.Snapshot {
	return report.Snapshot{
		Schema: report.Schema, Available: true, GeneratedAt: at,
		Overall:  report.Overall{Grade: grade},
		Coverage: []report.Coverage{{Name: "test", Type: "package", Ready: true}},
	}
}

func TestScanCommandStoresUnreadSnapshot(t *testing.T) {
	store := report.Store{Dir: t.TempDir()}
	want := scheduledSnapshot(time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC), catalog.GradeAmber)
	scan := func(_ context.Context, set *catalog.Set, options engine.Options) (report.Snapshot, error) {
		assertReportLockHeld(t, store)
		if set == nil || !options.Network {
			t.Fatalf("scan inputs: catalogue=%v options=%+v", set, options)
		}
		return want, nil
	}
	userFragments := false
	command := newScanCommand(&userFragments, scan, func() (report.Store, error) { return store, nil })
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(io.Discard)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.GeneratedAt.Equal(want.GeneratedAt) || got.Overall.Grade != want.Overall.Grade {
		t.Fatalf("stored snapshot = %+v, want %+v", got, want)
	}
	unread, err := store.IsUnread()
	if err != nil || !unread {
		t.Fatalf("unread = %v, error = %v; want true", unread, err)
	}
}

func TestReportCommandOpensStoredSnapshotAndRefreshUsesFreshScan(t *testing.T) {
	store := report.Store{Dir: t.TempDir()}
	initial := scheduledSnapshot(time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC), catalog.GradeAmber)
	if err := store.Save(initial, true); err != nil {
		t.Fatal(err)
	}
	fresh := scheduledSnapshot(time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC), catalog.GradeRed)
	scanCalls := 0
	scan := func(_ context.Context, _ *catalog.Set, options engine.Options) (report.Snapshot, error) {
		scanCalls++
		assertReportLockHeld(t, store)
		if !options.Network {
			t.Fatal("refresh scan disabled network checks")
		}
		return fresh, nil
	}
	userFragments := false
	runUI := func(_ context.Context, _ io.Reader, _ io.Writer, opened report.Snapshot, refresh tui.ScanFunc) error {
		if !opened.GeneratedAt.Equal(initial.GeneratedAt) {
			t.Fatalf("opened snapshot at %s, want %s", opened.GeneratedAt, initial.GeneratedAt)
		}
		if scanCalls != 0 {
			t.Fatal("opening the stored report ran a scan")
		}
		updated, err := refresh(context.Background())
		if err != nil {
			return err
		}
		if !updated.GeneratedAt.Equal(fresh.GeneratedAt) {
			t.Fatalf("refreshed snapshot at %s, want %s", updated.GeneratedAt, fresh.GeneratedAt)
		}
		return nil
	}
	command := newReportCommand(&userFragments, scan,
		func() (report.Store, error) { return store, nil }, runUI)
	command.SetIn(bytes.NewReader(nil))
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.GeneratedAt.Equal(fresh.GeneratedAt) || got.Overall.Grade != catalog.GradeRed {
		t.Fatalf("stored refresh = %+v, want %+v", got, fresh)
	}
	unread, err := store.IsUnread()
	if err != nil || unread {
		t.Fatalf("unread after opening report = %v, error = %v; want false", unread, err)
	}
	if scanCalls != 1 {
		t.Fatalf("refresh scan count = %d, want 1", scanCalls)
	}
}

func assertReportLockHeld(t *testing.T, store report.Store) {
	t.Helper()
	fd, err := syscall.Open(filepath.Join(store.Dir, ".scan.lock"), syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatalf("open report lock during scan: %v", err)
	}
	defer syscall.Close(fd)
	err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		t.Fatal("scan ran without holding the report lock")
	}
	if !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("probe report lock: %v, want EWOULDBLOCK", err)
	}
}

func TestReportCommandLeavesUnreadMarkerWhenOpenFails(t *testing.T) {
	store := report.Store{Dir: t.TempDir()}
	initial := scheduledSnapshot(time.Now().UTC(), catalog.GradeUnknown)
	if err := store.Save(initial, true); err != nil {
		t.Fatal(err)
	}
	userFragments := false
	command := newReportCommand(&userFragments, nil,
		func() (report.Store, error) { return store, nil },
		func(context.Context, io.Reader, io.Writer, report.Snapshot, tui.ScanFunc) error {
			return context.Canceled
		})
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	if err := command.Execute(); err == nil {
		t.Fatal("failed TUI open was hidden")
	}
	unread, err := store.IsUnread()
	if err != nil || !unread {
		t.Fatalf("unread after failed open = %v, error = %v; want true", unread, err)
	}
}
