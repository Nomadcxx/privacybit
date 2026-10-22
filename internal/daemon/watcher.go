// Package daemon watches fast software inventories for newly installed
// catalogue entries. It does not enforce network or package policy.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/detect"
)

const pollInterval = 3 * time.Minute

type SourceSpec struct {
	Type   catalog.DetectType
	Source detect.Source
}

type Event struct {
	Source  string
	EntryID string
	Name    string
	Scope   string
}

type NotifyFunc func(context.Context, Event) error

type inventoryKey struct {
	name  string
	scope string
}

type sourceState struct {
	complete bool
	items    map[inventoryKey]struct{}
}

// Watcher keeps one last-complete inventory per source. Failed reads never
// replace that source's baseline, so a temporary outage cannot look like a
// wave of removals followed by false new-install events.
type Watcher struct {
	set     *catalog.Set
	sources []SourceSpec
	notify  NotifyFunc
	states  map[string]sourceState
	pending map[string]Event
}

func NewWatcher(set *catalog.Set, sources []SourceSpec, notify NotifyFunc) (*Watcher, error) {
	if set == nil {
		return nil, errors.New("catalogue is required")
	}
	if len(sources) == 0 {
		return nil, errors.New("at least one inventory source is required")
	}
	seen := make(map[string]bool, len(sources))
	for _, spec := range sources {
		if spec.Source == nil {
			return nil, errors.New("inventory source is nil")
		}
		name := spec.Source.Name()
		if name == "" || seen[name] {
			return nil, fmt.Errorf("inventory source name %q is empty or duplicated", name)
		}
		seen[name] = true
		if spec.Type != catalog.DetectPackage && spec.Type != catalog.DetectFlatpak {
			return nil, fmt.Errorf("watcher cannot observe detect type %q", spec.Type)
		}
	}
	if notify == nil {
		notify = NotifyDesktop
	}
	return &Watcher{
		set: set, sources: append([]SourceSpec(nil), sources...), notify: notify,
		states: make(map[string]sourceState), pending: make(map[string]Event),
	}, nil
}

// Poll reads each available source once. A source whose inventory fails keeps
// its prior complete set; its peers can still advance and report additions.
func (w *Watcher) Poll(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var failures []error
	for _, spec := range w.sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := spec.Source.Name()
		availability := spec.Source.Availability()
		if !availability.Ready {
			continue
		}
		var observations []detect.Observation
		if !availability.Empty {
			items, err := spec.Source.Inventory(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				failures = append(failures, fmt.Errorf("%s inventory: %w", name, err))
				continue
			}
			observations = items
		}

		current := make(map[inventoryKey]struct{}, len(observations))
		for _, item := range observations {
			key := inventoryKey{name: item.Name, scope: item.Scope}
			current[key] = struct{}{}
		}
		previous := w.states[name]
		if previous.complete {
			keys := make([]inventoryKey, 0, len(current))
			for key := range current {
				keys = append(keys, key)
			}
			sort.Slice(keys, func(i, j int) bool {
				if keys[i].name == keys[j].name {
					return keys[i].scope < keys[j].scope
				}
				return keys[i].name < keys[j].name
			})
			for _, key := range keys {
				if _, existed := previous.items[key]; existed {
					continue
				}
				entry := w.set.Match(spec.Type, key.name)
				if entry == nil || entry.Grade != catalog.GradeRed {
					continue
				}
				event := Event{Source: name, EntryID: entry.ID, Name: entry.Name, Scope: key.scope}
				w.pending[eventKey(event)] = event
			}
		}
		w.states[name] = sourceState{complete: true, items: current}
	}

	keys := make([]string, 0, len(w.pending))
	for key := range w.pending {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := w.notify(ctx, w.pending[key]); err != nil {
			failures = append(failures, fmt.Errorf("notify %s: %w", w.pending[key].EntryID, err))
			continue
		}
		delete(w.pending, key)
	}
	return errors.Join(failures...)
}

func (w *Watcher) Run(ctx context.Context) error {
	if err := w.Poll(ctx); err != nil && ctx.Err() == nil {
		log.Printf("privacybit: watcher poll incomplete: %v", err)
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.Poll(ctx); err != nil && ctx.Err() == nil {
				log.Printf("privacybit: watcher poll incomplete: %v", err)
			}
		}
	}
}

func eventKey(event Event) string {
	return event.Source + "\x00" + event.EntryID + "\x00" + event.Scope
}

func NotifyDesktop(ctx context.Context, event Event) error {
	body := event.Name
	if event.Scope != "" {
		body += " (" + event.Scope + " installation)"
	}
	command := exec.CommandContext(ctx, "notify-send", "--app-name=privacybit",
		"--urgency=normal", "--expire-time=10000", "New red-rated privacy app",
		cleanNotificationText(body, 256))
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("notify-send failed: %w: %s", err, cleanNotificationText(string(output), 256))
	}
	return nil
}

func cleanNotificationText(value string, limit int) string {
	var out strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) {
			if r == '\n' || r == '\r' || r == '\t' {
				out.WriteByte(' ')
			}
			continue
		}
		out.WriteRune(r)
		if out.Len() >= limit {
			out.WriteString("…")
			break
		}
	}
	return out.String()
}
