package daemon

import (
	"context"
	"errors"
	"testing"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/detect"
)

type testSource struct {
	name         string
	availability detect.Availability
	items        []detect.Observation
	err          error
}

func (s *testSource) Name() string                      { return s.name }
func (*testSource) Capabilities() detect.Capabilities   { return detect.Capabilities{} }
func (s *testSource) Availability() detect.Availability { return s.availability }
func (s *testSource) Inventory(context.Context) ([]detect.Observation, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]detect.Observation(nil), s.items...), nil
}

func shippedCatalogue(t *testing.T) *catalog.Set {
	t.Helper()
	set, err := catalog.Load(catalog.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestWatcherBaselinesQuietlyAndReportsOnlyNewRedEntries(t *testing.T) {
	set := shippedCatalogue(t)
	red, green := detect.Observation{Name: "nordvpn"}, detect.Observation{Name: "bitwarden"}
	source := &testSource{name: "pacman", availability: detect.Available(), items: []detect.Observation{red}}
	var events []Event
	watcher, err := NewWatcher(set, []SourceSpec{{Type: catalog.DetectPackage, Source: source}}, func(_ context.Context, event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := watcher.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("baseline emitted %d notifications", len(events))
	}
	source.items = []detect.Observation{red, green, {Name: "surfshark"}}
	if err := watcher.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EntryID != "surfshark" {
		t.Fatalf("new red notifications = %+v, want only surfshark", events)
	}
	if err := watcher.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("unchanged inventory repeated notification: %+v", events)
	}
}

func TestWatcherPreservesCompleteInventoryAcrossSourceFailure(t *testing.T) {
	set := shippedCatalogue(t)
	source := &testSource{name: "flatpak", availability: detect.Available(), items: []detect.Observation{{Name: "com.nordvpn.nordvpn", Scope: "system"}}}
	var events []Event
	watcher, err := NewWatcher(set, []SourceSpec{{Type: catalog.DetectFlatpak, Source: source}}, func(_ context.Context, event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := watcher.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	source.availability = detect.NotReady("temporary database failure")
	source.items = nil
	if err := watcher.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	source.availability = detect.Available()
	source.err = errors.New("temporary failure")
	if err := watcher.Poll(ctx); err == nil {
		t.Fatal("inventory failure was hidden")
	}
	source.err = nil
	source.items = []detect.Observation{
		{Name: "com.nordvpn.nordvpn", Scope: "system"},
		{Name: "com.nordvpn.nordvpn", Scope: "user"},
	}
	if err := watcher.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Scope != "user" {
		t.Fatalf("scope additions = %+v, want only user installation", events)
	}
}

func TestWatcherRetriesFailedNotification(t *testing.T) {
	set := shippedCatalogue(t)
	source := &testSource{name: "pacman", availability: detect.Available()}
	attempts := 0
	watcher, err := NewWatcher(set, []SourceSpec{{Type: catalog.DetectPackage, Source: source}}, func(context.Context, Event) error {
		attempts++
		if attempts == 1 {
			return errors.New("notification service unavailable")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := watcher.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	source.items = []detect.Observation{{Name: "nordvpn"}}
	if err := watcher.Poll(ctx); err == nil {
		t.Fatal("notification failure was hidden")
	}
	if err := watcher.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || len(watcher.pending) != 0 {
		t.Fatalf("notification attempts=%d pending=%d, want 2 and 0", attempts, len(watcher.pending))
	}
}
