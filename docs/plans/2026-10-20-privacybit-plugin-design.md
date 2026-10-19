# D5 — the plugin and the host

**Date:** 2026-10-20
**Status:** approved
**Touches:** `/home/nomadx/sysc-plugins` and `/home/nomadx/sysc-shell`
**Closes:** the user's original request — the Control Centre privacy panel

---

## 1. Scope

This is the design that makes the whole project exist for the user. It is also
the smallest, because it moves no audit logic.

Two halves:

- **Plugin** — `plugins/privacy` in sysc-plugins. Runs an audit on a tick and
  publishes a bounded JSON snapshot into its own state.
- **Host** — five edits in sysc-shell that render that snapshot in the existing
  Control Centre privacy panel, and one wire fix that was already needed
  regardless of privacybit.

Out of scope: any privileged operation from the panel (§4), any change to the
capture panel that exists today, any plugin-provided UI (§2).

## 2. The plugin still cannot render UI

Unchanged from D1 §2.2. The Control Centre is host-rendered: `ccPage` in
`internal/shell/popout_controlcenter.go` is a `switch h.section` over host
functions with no plugin branch. The plugin's only power is `CallStateSet`.

The calendar plugin is the precedent, and D5 is a close copy of it.

## 3. Correction: the host already has a warning tone

**D1 §8.1 is wrong.** It states that plugin/v1 tones are only
`normal`/`error`/`subtle`/`accent` and that a warning tone must be added to the
host. The host has had one for some time.

`internal/ui/tree.go`:

```go
type Tone uint8
const (
	ToneNormal   Tone = iota
	ToneError
	ToneAccent
	ToneSubtle
	ToneActivity  // paints the theme's Tertiary role
)
```

`ToneActivity` is documented as *"a reading past its activity threshold but
short of critical"* and is already used in `controlcenter_monitor.go`,
`popout_process.go`, and — in `monitorscale_test.go` — with exactly the
normal → activity → error threshold progression privacybit's grades want.

Its comment also explains why it exists: *"the theme has no warning token, and
Tertiary exists on every palette."* So the visual layer is done and needs no
theme work.

**What is missing is only the wire.** plugin/v1 defines four tones and validates
against `knownTones`, so `"activity"` is currently rejected. Two edits close it.

**The trap:** `ui.Tone` is an iota enum. A new constant must be **appended**,
never inserted, or every subsequent tone value shifts and anything persisting a
numeric tone silently changes meaning. D1 §8.1 should be amended to record that
the renderer already had the tone and only the plugin protocol lacked it.

## 4. Two revisions to earlier decisions

Both are late corrections with concrete reasons, recorded here rather than
quietly applied.

### 4.1 The plugin shells out; it does not import the module

D1 §2.1 chose a Go module dependency so the panel and the CLI would run the same
audit. Shelling out to the same binary satisfies that just as well — there is
still only one implementation — and it is better for three reasons:

1. **`internal/` is not importable across modules.** The audit engine lives at
   `github.com/Nomadcxx/privacybit/internal/...`. sysc-plugins is a different
   Go module, so it cannot import any of it without moving the engine to a
   public path.
2. **No cross-module API to version.** A public package becomes a compatibility
   commitment between two repositories, for a surface that is one function call.
3. **moonbit already does this.** `plugins/moonbit` shells out to the `moonbit`
   binary. It is the house pattern, and the audit cost is irrelevant at a
   once-per-minute poll (~400 ms, 0.07% duty cycle).

```go
// Path resolution, mirroring moonbit's Runner.
type Runner struct {
    Privacybit string // empty means PATH
}
```

### 4.2 The panel never reaches root

D4 §7.2 establishes that the helper enforces confirmation at the privileged
boundary, because the panel bypasses the TUI. It does not follow that the
panel should be *able* to take privileged actions.

**In v1 the panel performs no privileged operation at all.** The panel is a
viewer; the TUI is the operator.

- A panel has no TTY, so every sudo action is a password prompt in a place
  designed for reading. moonbit does this because cleaning is its entire point.
  For a privacy audit the actions are mostly informational.
- The genuinely destructive action — purging a red app — is exactly the one a
  user should take deliberately, with the full plan on screen.
- Every finding already carries a tier 3 action that opens the TUI or prints
  the command. Nothing is lost.

This can be revisited once the panel is proven, but it is the right default:
**a control centre that can remove software with one click is a bad control
centre**, and D4's token exists to make that impossible, not merely hard.

## 5. The plugin

### 5.1 Identity

| Field | Value | Why |
|---|---|---|
| `id` | `org.sysc.privacy` | reverse-domain, matches `org.sysc.calendar` |
| `capabilities` | `["state"]` | it publishes a snapshot and renders nothing |
| `exec` | `bin/sysc-plugin-privacy` | |
| `requires.commands` | `[]` | must not refuse to start — §5.4 |
| `version` | `0.1.0` | |
| `protocol` | `{major: 1, minor: 8}` | matches the calendar plugin |

`capabilities` is just `["state"]`. It does **not** need `panels`, `settings`,
or `open-url`. The plugin opens no surface of its own.

### 5.2 What it does, once per tick

```go
func (p *plugin) refresh(ctx context.Context) {
    raw, err := p.runAudit(ctx)          // privacybit audit --json --no-progress
    if err != nil {
        p.publish(unavailableSnapshot(err))
        return
    }
    snap, err := parseSnapshot(raw)      // bounded; rejects a newer schema major
    if err != nil {
        p.publish(unavailableSnapshot(err))
        return
    }
    p.publish(snap)
}
```

- **`--json --no-progress`** so the NDJSON contract yields the document and not
  an event stream (D2 §4).
- **A newer `schema` major is rejected to no data** (D2 §6). Parsing a future
  snapshot with today's struct reads zero for unknown fields and would report
  *"no browsers found"* — the under-grade arriving through the back door.
- **Publishing is best-effort and non-fatal** (D2 §9). A failed `CallStateSet`
  is logged and retried on the next tick; it never takes the plugin down.

### 5.3 The projection

The full snapshot carries every installed app, every finding, every telemetry
connection. That is the right size for a terminal and far too large for a
596-pixel panel, so the plugin **projects** it — exactly as calendar does with
`BuildControlCenterSnapshot`.

The projection is bounded by explicit named caps, and **every capped list
carries its total and a truncation flag**. This is the one place privacybit
could quietly under-report, so it is stated in the wire contract:

```jsonc
{
  "schema": 1,
  "available": true,
  "generated_at": "2026-10-20T12:00:00Z",
  "catalog_rev": "2026.10.1",
  "overall": { "grade": "amber", "score": 62, "findings": 14, "high": 3 },
  "network": {
    "vpn": { "state": "connected", "client": "mullvad",
             "default_route_via_tunnel": true,
             "dns_contained": false, "ipv6_contained": false,
             "defects": ["dns-leak"] },
    "telemetry_now": 3
  },
  "categories": [
    { "id": "browser", "name": "Browser", "grade": "red",
      "installed": 4, "installed_shown": 3,
      "installed_truncated": true, "installed_total": 4,
      "worst": { "id": "chrome", "name": "Google Chrome", "grade": "red" } }
  ],
  "findings": [
    { "id": "vpn.dns-leak", "severity": "high", "grade": "red",
      "category": "network", "title": "DNS queries are leaving the tunnel",
      "summary": "Resolver 192.168.1.1 is outside the Mullvad tunnel.",
      "action": { "kind": "launch", "label": "How to fix" } }
  ],
  "reduced": [
    { "fact": "licence", "effect": "flatpak sandbox permissions could not be checked" }
  ],
  "entry_point": "privacybit"
}
```

Per category: a grade, counts, a truncation pair, and the single worst entry.
Not the whole list. The panel is a summary; `privacybit explain` is the detail,
and every finding names its `id` so the TUI can jump straight there.

`reduced` (D6 §10) is carried verbatim. A non-empty `reduced` puts a visible
caveat on the overall grade, because the panel is the one surface where the
user will *not* run `--json` and read the coverage section.

### 5.4 When privacybit is not installed

The plugin must start anyway. A missing `requires.commands` entry means the
host refuses to launch the plugin, and a plugin that refuses to launch cannot
tell the user anything useful.

So the plugin publishes an explicit unavailable snapshot:

```json
{ "schema": 1, "available": false,
  "reason": "the privacybit binary was not found on PATH",
  "hint": "install it, or set the plugin's binary path" }
```

The panel then renders *"Privacy audit unavailable — privacybit not found"* with
the hint, rather than an empty card. **An empty card is indistinguishable from
"you are fine"**, which is the unacceptable under-grade in its purest form.

## 6. The host

Five edits to `/home/nomadx/sysc-shell`, all additive.

### 6.1 `internal/ui` — append, do not insert

```go
const (
	ToneNormal Tone = iota
	ToneError
	ToneAccent
	ToneSubtle
	ToneActivity // already present — nothing to do
)
```

Nothing to change here. Recorded because the append rule is load-bearing for
whoever adds the next tone.

### 6.2 `plugin/v1` — the wire

```go
// node.go, alongside the existing tones
ToneActivity Tone = "activity"

// and
var knownTones = map[Tone]bool{
	ToneNormal: true, ToneError: true,
	ToneSubtle: true, ToneAccent: true,
	ToneActivity: true, // new
}
```

Plus one case in the v1→ui tone conversion. The protocol minor version goes
up, because a host that does not know `"activity"` must reject the node rather
than silently render it unstyled.

### 6.3 `controlcenter_pages.go` — the snapshot reader

Mirrors `ccCalendarSnapshotFor` exactly, including its contract:

```go
const (
	ccPrivacyPluginID = "org.sysc.privacy"
	ccPrivacyStateKey = "control_center"
)

// ccPrivacySnapshotFor reads the privacy plugin's published snapshot from the
// running plugin's in-memory store. No plugin, no key or malformed state
// simply means "no audit"; it never breaks the page.
func ccPrivacySnapshotFor(r *Registry) *ccPrivacySnapshot {
	if r == nil {
		return nil
	}
	raw, ok := r.plugins.stateValue(ccPrivacyPluginID, ccPrivacyStateKey)
	if !ok {
		return nil
	}
	var snapshot ccPrivacySnapshot
	if json.Unmarshal(raw, &snapshot) != nil {
		return nil
	}
	return &snapshot
}
```

### 6.4 `ccPrivacy` — the second card

Today: one `monitorCard` titled "Capture activity", height `ccPageH`, three
rows. The audit becomes a second card below it, inside a `KindScroll`:

```
┌─ Capture activity ──────────────┐  unchanged
│ Microphone      Not recording
│ Camera          Not recording
│ Screen sharing  Inactive
└────────────────────────────────┘
┌─ Privacy audit ── ▲ Review ────┐  new
│ DNS is leaking outside your VPN
│ Browser  Google Chrome    ■
│ Password Bitwarden        ●
│ ▸ 12 findings · 3 high   Run audit
└────────────────────────────────┘
```

The capture panel is untouched. It answers a different question — *is something
recording right now* — and conflating it with *is your setup reasonable* would
make both less useful.

**Grade rendering uses all three channels** (D2 §9, D3 §5). Colour is mapped to
the tone that already exists:

| Grade | Tone | Glyph | Word |
|---|---|---|---|
| `green` | `ToneNormal` | ● | Good |
| `amber` | **`ToneActivity`** | ▲ | Review |
| `red` | `ToneError` | ■ | Risky |
| `unknown` | `ToneSubtle` | ? | Unrated |

This is precisely the threshold progression `monitorscale_test.go` already
asserts, which is reassuring rather than coincidental: the host's own monitor
panel treats a mid-range reading as an attention state, and a grade is a
mid-range reading.

The word always renders, so the glyph and colour are redundant rather than
load-bearing — which is what makes D2's ASCII fallback cost nothing.

### 6.5 Action verbs

Two, both non-privileged (§4.2):

| Verb | Effect |
|---|---|
| `privacybit:refresh` | drop the cached snapshot so the next read rescans |
| `privacybit:explain <finding-id>` | open the TUI at that finding |

Both route through `activateControlCentre` alongside the existing
`panel:privacy`. Neither takes an argument that reaches a package name, a path,
or a unit — the panel's power over the system ends at "show me more".

## 7. Failure matrix

Every row degrades rather than breaks. The panel is a display; it may never be
the thing that takes the desktop down.

| Failure | Panel shows | Plugin state |
|---|---|---|
| Plugin not installed | audit card absent; capture panel normal | — |
| privacybit binary missing | "Privacy audit unavailable" + hint | `available:false` |
| Audit timed out | "Audit did not finish" + last grade, timestamped | `available:true`, stale |
| Malformed snapshot | audit card absent | unchanged |
| Snapshot from a future `schema` | audit card absent | unchanged |
| Plugin crashed | audit card absent; capture panel unaffected | — |
| All category sources unavailable | categories empty, `reduced` non-empty, overall `unknown` | published |

The last row matters most. **An audit that could not see anything must not
report good.** It reports `unknown`, and the panel says so in words.

## 8. Testing

| Property | Method |
|---|---|
| Tone accepted and converted | `plugin/v1` test that `"activity"` passes `knownTones` and converts to `ui.ToneActivity`; the existing unknown-tone test keeps rejecting typos. |
| Enum stability | A test asserting `ToneActivity` is the highest `ui.Tone` value, so a future insertion fails CI. |
| Nil-safe reader | `ccPrivacySnapshotFor` against nil registry, absent key, malformed JSON, and a valid snapshot. Mirrors the calendar test. |
| Projection bounds | A synthetic 10,000-app snapshot asserting every list respects its cap **and** that `installed_truncated`/`installed_total` are correct. |
| No privileged action | A table asserting every `privacybit:*` verb maps to a non-privileged operation, and that none accepts a package name, path, or unit. |
| Missing binary | Plugin test with `PATH` stripped; asserts `available:false` is published, not a crash and not silence. |
| Future schema | A fixture with `"schema": 2` must produce no snapshot, never a partial parse. |
| Capture panel unaffected | The existing privacy panel tests pass unmodified. |
| Rendering | `shelllint.Tree` against `panelTargetSize(PanelControlCentre)` so a tree that validates but refuses at render is caught in CI. |

## 9. Ordering and dependencies

D5 is **phase 10** of D1 §15, after the CLI exists, because the plugin shells
out to it.

1. The plugin cannot be written meaningfully before `privacybit audit --json`
   produces the document it publishes.
2. The host tone wire (`§6.2`) is **independent of privacybit entirely**. It is
   a small fix the plugin protocol wanted regardless, and it can land first.
3. `ccPrivacySnapshotFor` plus the second card can land against a hand-written
   fixture snapshot, so the host work is not blocked on the plugin at all.

That last point is worth taking: **the panel can be built and reviewed before
the engine exists**, against a JSON file. It is the only part of this project
where the user-visible outcome is testable today.

## 10. What this closes

With D5, the original request is answerable end to end: open the Control Centre,
see whether a VPN is actually protecting you, see every installed application
graded by category with a reason, and get a concrete next step for each finding.

What is deliberately *not* delivered: one-click remediation from the panel
(§4.2), and any enforcement. D1 §1.1's framing holds — this grades privacy and
reports it. Acting on it is the TUI's job, and the destructive part of that is
the helper's.