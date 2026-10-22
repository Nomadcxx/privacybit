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
- **Host** — renders the bounded snapshot on the existing Control Centre
  privacy page and uses its existing warning tone for amber grades.

Out of scope: any privileged operation from the panel (§4), any change to the
capture panel that exists today, any plugin-provided UI (§2).

## 2. The plugin still cannot render UI

Unchanged from D1 §2.2. The Control Centre is host-rendered: `ccPage` in
`internal/shell/popout_controlcenter.go` is a `switch h.section` over host
functions with no plugin branch. The plugin's only power is `CallStateSet`.

The calendar plugin is the precedent, and D5 is a close copy of it.

## 3. Existing warning tone

sysc-shell `main` accepts `plugin/v1.ToneWarning` (`warning`) at protocol minor
18. `internal/plugin/view.go` maps it to `ui.ToneActivity`, which paints the
theme's Tertiary role and already appears in monitor and process panels. The
privacy panel uses this wire value for amber grades. It needs no additional
tone or protocol bump.

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
| `capabilities` | `["state"]` | it publishes a snapshot and renders nothing |
| `exec` | `bin/sysc-plugin-privacy` | |
| `requires.commands` | `[]` | must not refuse to start — §5.4 |
| `version` | `0.1.0` | |
| `protocol` | `{major: 1, minor: 8}` | minimum minor for this state-only plugin; it emits no activity-tone view nodes |

`capabilities` is just `["state"]`. It does **not** need `panels`, `settings`,
or `open-url`. The plugin opens no surface of its own.

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
attribution is not implemented. The full report is still too large for a
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

Per category: a grade, counts, a truncation pair, and the single worst entry.
Not the whole list. The panel is a summary; the CLI and TUI provide the detail.
The plugin snapshot carries the action suggestions from the report, but the
Control Centre does not execute them.

Category grades use the worst known installed-entry grade and unmet
expectations; the overall grade is the worst category grade. Unknown category
grades propagate to the overall grade so incomplete judgement cannot appear as
a pass. This follows D1 §3.7; the score is a separate progress indicator.

`reduced` (D6 §10) is carried with total and truncation fields. The card always
states that telemetry attribution is unavailable, then shows the reported
coverage caveats. The panel does not imply that unmeasured telemetry is clean.

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

The host renders a bounded audit summary below the existing capture activity
card and uses the existing warning tone for amber grades.

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
the renderer enum.

### 6.2 `plugin/v1` — the wire

`plugin/v1.ToneWarning` (`warning`) already exists at protocol minor 18.
`internal/plugin/view.go` maps it to `ui.ToneActivity`. The privacy panel uses
that value for amber. The privacy plugin emits no view nodes, so its manifest
keeps minimum protocol minor 8.

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

### 6.4 `ccPrivacy` — the second card

The page uses its existing scroll viewport. The capture card keeps its three
rows and uses its natural content height. The audit card follows it:

```
┌─ Capture activity ──────────────┐  unchanged
│ Microphone      Not recording
│ Camera          Not recording
│ Screen sharing  Inactive
└────────────────────────────────┘
┌─ Privacy audit ────────────────┐  new
│ Overall             Risky
│ VPN                 Connected
│ Default route       Outside tunnel
│ Browser             Risky · 4 installed
│ Findings            1 finding · 1 high
│ Coverage limits     Telemetry attribution unavailable
└────────────────────────────────┘
```

The capture readings retain their existing meaning and refresh path. The audit
card shows the overall grade, VPN route/DNS/IPv6 checks, category grades,
bounded findings, and coverage limits. A report older than 15 minutes or with a
future timestamp is shown as `Unrated`; the last grade appears only as subdued
historical text, and stale category/finding details are hidden.

Grade rendering uses a word and the existing theme tone:

| Grade | Tone | Word |
|---|---|---|
| `green` | `ToneNormal` | Good |
| `amber` | `ToneActivity` | Review |
| `red` | `ToneError` | Risky |
| `unknown` | `ToneSubtle` | Unrated |

The grade word remains visible without colour, and the tone provides a second
channel for colour-aware themes.

### 6.5 Actions

The original Phase 11 viewer-only boundary was revised by the owner on
2026-10-20. The native Control Centre now exposes one host-owned `Run scan`
action. It invokes `privacybit scan` as the current user, outside the registry
lock. It resolves the binary from `PATH`, then `~/.local/bin/privacybit`, which
is the installer target.

The panel reads the private `$XDG_STATE_HOME/privacybit/report.json` file, or
`~/.local/state/privacybit/report.json` when `XDG_STATE_HOME` is unset. The host
validates the current user's ownership, private permissions, regular-file
type, schema, grade and timestamp, and reads no more than 4 MiB. It reads this
file when the user enters the Privacy page and after a manual scan. It does not
mark the unread report as read. A report older than 15 minutes or dated in the
future remains `Unrated`.

The panel shows scan status, the saved report's grade, and its timestamp. The
full report remains available through `privacybit report`. Manual scans,
scheduled scans and TUI refreshes share an advisory lock in the per-user state
directory from the start of scanning through the atomic report save. The panel
does not use a daemon socket, change the plugin protocol, or run a privileged
action. The plugin continues publishing its bounded audit summary on its
five-minute tick.

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
| Tone accepted and converted | Existing tests cover `plugin/v1.ToneWarning` mapping to `ui.ToneActivity`; privacy tests pin amber to the activity theme tone. |
| Bounded plugin projection | `plugins/privacy` tests pin list caps, total/truncation fields, malformed/future-schema rejection, and telemetry remaining unknown. |
| Safe snapshot reader | Shell tests cover absent state, malformed JSON, future schema, invalid grade/counts, oversized state, and available/unavailable snapshots. |
| Currentness | Shell tests verify stale and future timestamps render unrated and hide stale details. |
| Honest card | Shell tests check the capture rows, grade words/tones, VPN coverage, category/finding totals, caveats, and explicit unavailable state. |
| Plugin protocol | Plugin tests prove `audit --snapshot` invocation and state publication over the JSON-lines protocol. |

## 9. Ordering and dependencies

D5 is **phase 10** of D1 §15, after the CLI exists, because the plugin shells
out to it.

1. The plugin cannot be written meaningfully before `privacybit audit --snapshot`
   produces the document it publishes.
2. The warning wire tone is independent of privacybit and already exists in
   sysc-shell `main`.
3. `ccPrivacySnapshotFor` plus the second card can land against a hand-written
   fixture snapshot, so the host work is not blocked on the plugin at all.

That last point is worth taking: **the panel can be built and reviewed before
the privacybit CLI snapshot exists**, against a JSON file. It is the only part
of this project where the user-visible outcome is testable today.

## 10. What this closes

With D5, the Control Centre shows the current capture status separately from
the latest privacy audit, including VPN checks, category grades, bounded
findings, and coverage caveats. It can request a read-only scan and show the
saved report's grade and timestamp. The CLI and TUI provide the full report and
action detail.

What is deliberately *not* delivered: one-click remediation from the panel
(§4.2), and any enforcement. D1 §1.1's framing holds — this grades privacy and
reports it. Acting on it is the TUI's job, and the destructive part of that is
the helper's.
