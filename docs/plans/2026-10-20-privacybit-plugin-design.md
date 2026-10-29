# D5 — the plugin and the host

**Date:** 2026-10-20
**Status:** approved
**Touches:** `/home/nomadx/sysc-plugins` and `/home/nomadx/sysc-shell`
**Closes:** the user's original request — the Control Centre privacy panel

> **Amended 2026-10-23:** §11 records the current owner-approved panel design.
> It supersedes the original state-only boundary, host-owned scan and
> `ToneWarning` grade treatment. The no-root boundary in §4.2 remains in force.

---

## 1. Scope

This is the design that makes the whole project exist for the user. It is also
the smallest, because it moves no audit logic.

Two parts:

- **Plugin** — `plugins/privacy` in sysc-plugins. Runs an audit on a tick,
  publishes a bounded JSON snapshot, declares a bar widget and audit panel,
  and offers the read-only `Scan now` action.
- **Host** — keeps the existing capture activity card, renders a compact audit
  summary, and opens the plugin panel from `Open full audit`.

Out of scope: any privileged operation from the panel (§4), any change to the
capture panel that exists today, or enforcement of audit findings.

## 2. The plugin panel is a separate surface

The Control Centre remains host-rendered: `ccPage` in
`internal/shell/popout_controlcenter.go` switches over host functions and does
not render plugin views inside a Control Centre section. The shell can open a
separate plugin-owned panel through the existing `panels` capability. The bar
widget and `Open full audit` action both open that panel.

The plugin also publishes bounded state for the compact Control Centre summary.
That state follows the existing calendar plugin precedent.

## 3. Existing warning wire tone

sysc-shell `main` accepts `plugin/v1.ToneWarning` (`warning`) at protocol minor
18. `internal/plugin/view.go` maps it to `ui.ToneActivity`, which paints the
theme's Tertiary role and appears in monitor and process panels. Privacy grade
chips do not use this tone: the laptop palette made it appear green. §11.2
defines the current grade treatment. The existing wire mapping remains
available to other plugin views and needs no protocol change.

D5's earlier draft called the wire value `activity`. That name duplicated the
host's existing `warning` value, so the integration now reuses `warning`.

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

**The panel performs no privileged operation.** It may request a read-only scan
as the current user. The TUI and helper handle state-changing actions.

- A panel has no TTY, so every sudo action is a password prompt in a place
  designed for reading. moonbit does this because cleaning is its entire point.
  For a privacy audit the actions are mostly informational.
- The genuinely destructive action — purging a red app — is exactly the one a
  user should take deliberately, with the full plan on screen.
- Every finding already carries a tier 3 action that opens the TUI or prints
  the command. Nothing is lost.

The panel can show results and request a read-only scan. It cannot remove
software or apply a finding. **A control centre that can remove software with
one click is a bad control centre**, and D4's token exists to make that
impossible, not merely hard.

## 5. The plugin

### 5.1 Identity

| Field | Value | Why |
|---|---|---|
| `id` | `org.sysc.privacy` | reverse-domain, matches `org.sysc.calendar` |
| `capabilities` | `["panels", "state"]` | it declares a bar widget and panel, and publishes a snapshot |
| `exec` | `bin/sysc-plugin-privacy` | |
| `requires.commands` | `[]` | must not refuse to start — §5.4 |
| `version` | `0.1.0` | |
| `protocol` | `{major: 1, minor: 8}` | minimum minor used by the panel and state calls |

The plugin needs `panels` to open its declared audit panel and `state` to publish
the bounded summary. It does not need `settings` or `open-url`.

The privacy plugin's source and package use GPL-3.0-or-later. Its Go files
carry SPDX headers, `plugins/privacy/LICENSE` ships with the package, and the
catalog metadata publishes that license. The sysc-plugins repository license
continues to apply to its other plugins.

### 5.2 What it does, once per tick

```go
data, runErr := run("privacybit", "audit", "--snapshot")
snapshot, parseErr := privacy.ParseAuditOutput(data)
if parseErr != nil {
    snapshot = privacy.UnavailableSnapshot(firstError(runErr, parseErr), time.Now())
}
publishWithStateSet(snapshot)
```

- **`audit --snapshot`** emits one `done` event with the schema-1 snapshot.
  A valid unknown snapshot is accepted even when the CLI exits with code 2
  because every detection source was unavailable.
- **Missing, malformed, oversized, or unsupported output** becomes an explicit
  unavailable snapshot. A future schema is never decoded as today's data.
- A rejected state write is logged and retried on the next tick. A broken
  protocol transport ends the plugin process and is handled by its supervisor.

### 5.3 The projection

The full snapshot carries the installed-app inventory, findings, and coverage
information. It does not contain live telemetry connections because endpoint
attribution is not implemented. The host summary has limited space and plugin
state has an explicit size cap, so the plugin **projects** the report — using
the same bounded-state pattern as calendar's `BuildControlCenterSnapshot`.

The projection is bounded by explicit named caps, and **every capped list
carries its total and a truncation flag**. This is the one place privacybit
could quietly under-report, so it is stated in the wire contract:

```jsonc
{
  "schema": 1,
  "available": true,
  "generated_at": "2026-10-20T12:00:00Z",
  "catalog_rev": "2026.10.1",
  "overall": { "grade": "red", "score": 62, "findings": 1, "high": 1 },
  "network": {
    "vpn": { "state": "connected", "client": "Mullvad VPN",
             "default_route_via_tunnel": true, "dns_contained": false,
             "ipv6_contained": true,
             "kill_switch_note": "Firewall rules were not verified.",
             "defects": [{"id":"vpn.dns-leak",
               "title":"DNS is outside the tunnel",
               "detail":"The configured resolver is outside the tunnel."}],
             "defects_shown": 1, "defects_total": 1,
             "defects_truncated": false },
    "telemetry_connections_unknown": true
  },
  "categories": [
    { "id": "browser", "name": "Browser", "grade": "red",
      "installed": [{"id":"chrome", "name":"Google Chrome", "grade":"red"}],
      "installed_shown": 1, "installed_total": 4,
      "installed_truncated": true,
      "worst": { "id": "chrome", "name": "Google Chrome", "grade": "red" } }
  ],
  "categories_shown": 1, "categories_total": 10,
  "categories_truncated": true,
  "findings": [
    { "id": "vpn.dns-leak", "severity": "high", "grade": "red",
      "category": "network", "title": "DNS queries are leaving the tunnel",
      "summary": "Resolver 192.168.1.1 is outside the Mullvad tunnel.",
      "actions": [{"tier":3,"kind":"launch","label":"Show the removal plan"}],
      "actions_shown": 1, "actions_total": 1,
      "actions_truncated": false }
  ],
  "findings_shown": 1, "findings_total": 1,
  "findings_truncated": false,
  "reduced": [
    { "fact": "network.telemetry",
      "effect": "Live endpoint attribution is unavailable." }
  ],
  "reduced_shown": 1, "reduced_total": 1, "reduced_truncated": false
}
```

Each category carries its grade, counts, truncation state, and highest-risk
entry. The scrollable plugin panel renders the bounded app, network, finding,
and coverage details. It labels capped lists with their shown and total counts;
the CLI and TUI retain the complete report. The snapshot carries action
suggestions, but neither the panel nor the Control Centre executes them.

Category grades use the worst known installed-entry grade and unmet
expectations; the overall grade is the worst category grade. Unknown category
grades propagate to the overall grade so incomplete judgement cannot appear as
a pass. This follows D1 §3.7; the score is a separate progress indicator.

`reduced` (D6 §10) is carried with total and truncation fields. Both views state
that telemetry attribution is unavailable and show the reported coverage
caveats. Neither view presents unmeasured telemetry as clean.

### 5.4 When privacybit is not installed

The plugin must start anyway. A missing `requires.commands` entry means the
host refuses to launch the plugin, and a plugin that refuses to launch cannot
tell the user anything useful.

The plugin resolves `privacybit` from `PATH`, then `~/.local/bin/privacybit`,
matching the user-local installer target. If neither location contains an
executable, the plugin publishes an explicit unavailable snapshot:

```json
{
  "schema": 1, "available": false,
  "reason": "privacybit was not found on PATH or as an executable at ~/.local/bin/privacybit",
  "hint": "Install privacybit and make sure it is on PATH or in ~/.local/bin.",
  "generated_at": "2026-10-20T12:00:00Z",
  "overall": {"grade": "unknown"},
  "network": {"vpn": {"state": "unknown", "defects": []},
              "telemetry_connections_unknown": true},
  "categories": [], "categories_shown": 0, "categories_total": 0,
  "categories_truncated": false,
  "findings": [], "findings_shown": 0, "findings_total": 0,
  "findings_truncated": false,
  "reduced": [], "reduced_shown": 0, "reduced_total": 0,
  "reduced_truncated": false
}
```

The panel then renders *"Privacy audit unavailable — privacybit not found"* with
the hint, rather than an empty card. **An empty card is indistinguishable from
"you are fine"**, which is the unacceptable under-grade in its purest form.

## 6. The host

The Control Centre keeps the existing capture activity card and adds a compact
audit summary with an `Open full audit` action. The plugin panel owns the
detailed audit view and the `Scan now` action.

### 6.1 `internal/ui` — existing tone

```go
const (
	ToneNormal Tone = iota
	ToneError
	ToneAccent
	ToneSubtle
	ToneActivity // already present — nothing to do
)
```

`ToneActivity` already exists as the final iota value in `ui.Tone` and paints
the theme's tertiary role. Keep its position stable; this work does not change
the renderer enum. The privacy grade chips do not use this tone; see §11.2.

### 6.2 `plugin/v1` — the wire

`plugin/v1.ToneWarning` (`warning`) already exists at protocol minor 18.
`internal/plugin/view.go` maps it to `ui.ToneActivity`. The privacy views use
the grade-specific treatment in §11.2 instead of that warning tone. The plugin
uses the existing `panels` and `state` capabilities at protocol minor 8.

### 6.3 `controlcenter_pages.go` — the snapshot reader

Reads the running plugin's state through `stateValue`, following the calendar
pattern. The privacy reader also caps the raw state at 128 KiB, accepts schema
1 only, validates grades and bounded list totals, and requires telemetry to be
explicitly unknown. Missing or invalid state becomes an unavailable card; it
never becomes an empty result or a pass.

```go
const (
	ccPrivacyPluginID = "org.sysc.privacy"
	ccPrivacyStateKey = "control_center"
)

// ccPrivacySnapshotFor reads the running privacy plugin's bounded state.
func ccPrivacySnapshotFor(r *Registry) *ccPrivacySnapshot {
	if r == nil || r.plugins == nil {
		return nil
	}
	raw, ok := r.plugins.stateValue(ccPrivacyPluginID, ccPrivacyStateKey)
	if !ok {
		return nil
	}
	snapshot, err := parseCCPrivacySnapshot(raw)
	if err != nil {
		return nil
	}
	return snapshot
}
```

### 6.4 `ccPrivacy` — the compact summary

The page uses its existing scroll viewport. The capture card keeps its three
rows and uses its natural content height. The audit card follows it:

```
┌─ Capture activity ──────────────┐  unchanged
│ Microphone      Not recording
│ Camera          Not recording
│ Screen sharing  Inactive
└────────────────────────────────┘
┌─ Privacy audit ────────────────┐  new
│ Overall             Good
│ Updated just now
│ Coverage limits are listed in the full audit.
│ [Open full audit]
└────────────────────────────────┘
```

The capture readings retain their existing meaning and refresh path. The
summary shows the overall grade and currentness, then opens the plugin panel for
VPN checks, categories, findings, and coverage details. A snapshot older than
15 minutes or dated in the future is shown as `Unrated`; the plugin panel also
hides stale category and finding details. Both surfaces keep grade words
visible without relying on colour; §11.2 defines their visual treatment.

### 6.5 Scan ownership

The first design put `Run scan` in the Control Centre and read the saved
`privacybit scan` report. The owner superseded that design in §11 on
2026-10-23. The Control Centre now opens the plugin panel; it does not run a
scan or read the saved report. The plugin runs `privacybit audit --snapshot`
as the current user with a timeout. It does not write a saved report or invoke
the daemon or root helper. The CLI and TUI retain their saved-report workflows.

## 7. Failure matrix

Every row degrades rather than breaks. The panel is a display; it may never be
the thing that takes the desktop down.

| Failure | Panel shows | Plugin state |
|---|---|---|
| Plugin not installed or not running | audit card says the snapshot is unavailable; capture card remains | no running state |
| privacybit binary missing | unavailable reason and install/PATH hint | `available:false`, grade `unknown` |
| Audit timeout or invalid CLI output | explicit unavailable/unknown card | `available:false` |
| Malformed, oversized, or unsupported snapshot | unavailable/unknown card | invalid state is ignored |
| Last valid state is over 15 minutes old | `Unrated`, historical grade subdued, details hidden | age checked from `generated_at` |
| Future `generated_at` | `Unrated` with clock-skew note | state is not treated as current |
| All category sources unavailable | categories remain unknown, coverage is visible | `available:true`, overall `unknown` |

The last row matters most. **An audit that could not see anything must not
report good.** It reports `unknown`, and the panel says so in words.

## 8. Testing

| Property | Method |
|---|---|
| Grade presentation | Plugin and shell tests pin visible grade words and distinct fills/icons for Good, Review, Risky, and Unrated; amber does not depend on `ToneWarning`. |
| Bounded plugin projection | `plugins/privacy` tests pin list caps, total/truncation fields, malformed/future-schema rejection, and telemetry remaining unknown. |
| Safe snapshot reader | Shell tests cover absent state, malformed JSON, future schema, invalid grade/counts, oversized state, and available/unavailable snapshots. |
| Currentness | Shell tests verify stale and future timestamps render unrated and hide stale details. |
| Honest surfaces | Shell tests check the capture rows and compact audit summary; plugin tests check panel details, coverage, and explicit unavailable states. |
| Plugin protocol | Plugin tests prove `audit --snapshot` invocation and state publication over the JSON-lines protocol. |

## 9. Ordering and dependencies

D5 is **phase 10** of D1 §15, after the CLI exists, because the plugin shells
out to it.

1. The plugin cannot be written meaningfully before `privacybit audit --snapshot`
   produces the document it publishes.
2. The host already supports the plugin's `panels` and `state` capabilities;
   the integration needs no protocol or shell panel framework change.
3. `ccPrivacySnapshotFor` and the compact summary can use a fixture snapshot.
   Plugin tests cover its panel and `privacybit audit --snapshot` call.

The host summary can be reviewed against a JSON fixture before the CLI snapshot
exists. The plugin panel still needs a valid snapshot to verify its audit
details and scan action.

## 10. What this closes

With the approved design, the Control Centre keeps current capture status
separate from a compact audit summary and opens the plugin-owned panel. The
panel shows VPN checks, category grades, bounded findings, and coverage
caveats; its `Scan now` action requests a read-only snapshot. The CLI and TUI
provide the complete report and action detail.

What is deliberately *not* delivered: one-click remediation from the panel
(§4.2), and any enforcement. D1 §1.1's framing holds — this grades privacy and
reports it. Acting on it is the TUI's job, and the destructive part of that is
the helper's.

## 11. Owner-approved amendment — plugin bar widget and audit panel

**Date:** 2026-10-23
**Approval:** owner approved the audited direction after reviewing the
Control Centre, plugin examples and live laptop rendering.

This amendment replaces the original state-only and host-owned scan model and
sets the current grade treatment. The host already supports declared plugin
bar widgets, plugin-owned panels, view input events and bounded panel
placement; no protocol or shell panel framework change is needed.

### 11.1 Ownership and entry points

- `org.sysc.privacy` declares one user-placeable `bar` widget and one `panel`.
- Activating the bar widget requests the declared panel through the existing
  `panels` capability. The shell's Bar Editor persists its placement.
- The Control Centre keeps its existing microphone, camera and screen-sharing
  activity card. Its audit area becomes a compact grade/currentness summary
  with an `Open full audit` action that opens the same plugin panel.
- The plugin panel owns the audit display and the single `Scan now` action.
  Both the bar and Control Centre lead to that panel, so there is one refresh
  path and one current snapshot. The old host-owned `privacybit scan` card and
  its separate saved-report read are removed from the Control Centre.

The plugin continues to publish its bounded state snapshot for the Control
Centre. The panel renders the same in-memory snapshot and refreshes it by
running `privacybit audit --snapshot`; the refresh runs as the current user,
with a timeout, and does not write a saved report. It has no privileged host
calls and cannot invoke helper actions.

### 11.2 Visual treatment and honest states

Use the existing SYSC card spacing, type roles and theme-derived surfaces.
The panel header uses the `privacybit` wordmark in host-rendered text; the
plugin view protocol does not expose the user's light/dark theme variant, so a
fixed light or dark logo raster could become unreadable. Grade chips use
distinct shared theme styling: accent for Good, outlined surface with a warning
icon for Review, error container for Risky, and soft neutral for Unrated. The
shell's embedded icon set does not include a generic warning glyph, so Review
uses its supported `flag` caution icon. Grade words remain visible and
accessible without colour. The sticky-note mint/sun/rose fills are rejected
for panels by the protocol and are not used here.

The panel uses the plugin protocol's scrollable list node as its bounded
viewport. Native Control Centre grade capsules keep their icon and label in one
row child, matching the host renderer's capsule layout rule. The scan button
uses the bundled `restart_alt` icon.

The snapshot's 15-minute freshness boundary applies in both surfaces. A stale
or future timestamp yields Unrated and hides category/finding details; partial
coverage and every capped list remain explicitly labelled. A missing plugin
state, missing `privacybit`, failed scan or invalid state never produces a
green grade. Amber must not rely on `ToneWarning`, because that maps to the
theme's tertiary token and appeared green on the laptop palette.

When a schema-1 snapshot contains the optional `sandbox` object, the panel adds
a separate **Flatpak sandbox** section after app categories. It shows the
aggregate grade, permission-query coverage, and bounded per-copy grades,
recognized grants, and unclassified evidence. When the visible app list is
capped, the highest-risk copies appear first, with ID and scope as stable
tie-breakers, so the evidence behind the aggregate grade stays visible. Older
snapshots without the field omit the section and remain readable.

The plugin still runs with the same user privileges as sysc-shell. The panel
cannot reach root, remove software, execute suggested remediation or act on
findings. GPL-3.0-or-later applies to the plugin's source, UI and packaged
assets.
