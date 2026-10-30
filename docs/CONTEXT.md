# privacybit — background and intent

**Written:** 2026-10-20
**This file's absolute path:** `/home/nomadx/privacy-bit/docs/CONTEXT.md`
**Pairs with:** `/home/nomadx/privacy-bit/docs/HANDOVER.md` (technical state, file
manifest, how to verify the work)

**Reader:** an engineer or agent with **no prior context on this project.**

---

## Why this document exists

`HANDOVER.md` answers *what exists and how to check it*. It cannot answer *why
this was built, what the owner actually asked for, or what they intend to do
next* — because that knowledge lives in a conversation, and much of it never
reached a design document.

Read this first. Read `HANDOVER.md` second. The seven design documents in
`/home/nomadx/privacy-bit/docs/plans/` third.

This file records the owner's original request, approved grading rules, and
the v1 panel boundary. The earlier disagreement about panel controls is
resolved in §6 and D5.

---

## 1. Where this came from

The owner (Nomadxx) maintains **sysc-shell**, a Wayland shell written in Go
with a custom `ui.Node` renderer — not Qt, not QML.

sysc-shell's Control Centre has a **privacy panel**. As it stands, that panel
answers exactly one question:

> *Is something capturing my microphone, camera or screen right now?*

It works by polling PipeWire (`pw-dump`, 500 ms) and cross-referencing niri's
screen-share casts. It is good at that question and it does not attempt any
other.

The owner then asked for the panel to be expanded into a **privacy audit** —
one that assesses the machine, not just the moment. In their own words, the
audit should tell users:

- whether a **VPN is active**;
- whether **key privacy-preserving applications are installed** — they named
  `freetube`, `mullvadvpn`, `protonvpn`, `opentubex`;
- a **breakdown by category**, each rated **red / orange / green**;
- **toggles** that resolve open privacy issues;
- and whether **key privacy-preserving applications are present at all**.

Their category table, as they gave it:

| Category | Red (proprietary / unsafe) | Orange (middling) | Green (best) |
|---|---|---|---|
| Browser | Edge, Chrome | `brave-bin` | `brave-origin`, Waterfox, Mullvad Browser, LibreWolf |
| Password manager | 1Password | Proton Pass, KeePassX | Bitwarden |
| Chat | Slack, and other Chromium-based social/chat software | — | IRC-based clients |
| Work | Slack, WPS Office (proprietary) | — | — |
| PDF | Foxit Reader (proprietary) | — | — |
| Email | — | — | — |

They also posed five open questions, which are effectively the specification:

1. What the categories and applications are.
2. Where those applications live, and where a scanner can find them.
3. *(renumbered by them; the numbering as given was 1, 2, 4, 5)*
4. How to code them "from a privacy perspective".
5. **What are we missing that they have not thought about yet.**

They raised the architectural question themselves, offering three options: a
standalone project wired into the panel later; a sysc-shell plugin built from
scratch; or keeping it inside the Control Centre privacy panel only. **They
leaned towards a mix of the first and third.** That is what was built.

### 1.1 The correction that shaped the design

The owner corrected a claim that `brave-bin` and `brave-origin` were "the same
browser from different repositories".

> **They are different products.**

This mattered more than it looked. A flat list of package names cannot
represent Brave, Brave Origin, Brave Beta and Brave Nightly as distinct things
with distinct privacy postures — nor can it represent the Chrome → Chromium →
Ungoogled Chromium lineage, where each fork removes something different.

It forced an **identity model** before any data could be authored: one
catalogue entry is one concrete *product-variant* and is the only thing that
carries a grade. `brave` and `brave-origin` are two entries with two grades,
not one product with a variant flag. Retrofitting this later would have meant
re-authoring the entire table, so it was settled before any YAML was written.

**If you are ever tempted to flatten the catalogue back into one row per
program name, this is why not to.**

---

## 2. Why a standalone project with a plugin

The decisive architectural finding was this: **a plugin cannot render its view
inside a Control Centre section.**

The Control Centre is host-rendered. `ccPage` in
`internal/shell/popout_controlcenter.go` switches over host-side functions; it
does not render a plugin view inside a Control Centre section. A plugin can
publish bounded JSON into its own state, and the host renders it natively —
`org.sysc.calendar` writing to state key `control_center` is the existing,
working precedent. The shell can also open a plugin-owned panel as a separate
surface; D5 §11 records the approved route.

The implementation keeps the audit engine in privacybit and gives the plugin
its own panel. The plugin also publishes a bounded snapshot for the Control
Centre summary. The bar widget and the Control Centre summary both open the
same plugin panel.

So the split is:

```
privacybit (CLI/TUI + internal engine)
                ▲
                │ `audit --snapshot`
org.sysc.privacy plugin
   ├── bar widget ────────────────→ audit panel
   ├── audit panel (`Scan now`) ───→ privacybit
   └── bounded snapshot ──────────→ Control Centre summary
                                       └── `Open full audit` → audit panel
```

The plugin provides crash isolation, catalogue distribution, independent
versioning, and a place for the full audit UI. The Control Centre keeps the
existing capture activity card and a compact audit summary. The CLI, TUI, and
plugin call the same binary; Go's `internal/` import rule keeps the audit API
from becoming a cross-module compatibility commitment.

---

## 3. Decisions made in dialogue that the design documents do not record

These were reached through discussion, not derivation. A future reader will not
find them in `docs/plans/`.

### 3.1 The rating model is a hybrid, deliberately

The owner chose a **hybrid** of an authored catalogue (static reputation) and
**measured live system state**, over either alone.

The governing rule, which became invariant I1: **derived signals are monotone
risk evidence.** A measurement may push a grade toward more risk or raise a
flag. It may never improve one. There is no runtime path from red to amber.

The owner's reasoning was direct: **under-grading is the one unacceptable
error.** Everything else in this project is recoverable; a false green is not
detectable by the user and destroys trust in every other claim.

Three evidence families were accepted as grade inputs: live connections to
known telemetry endpoints, sandbox posture, and catalogue/package metadata
mismatch. Packaging origin is retained as provenance context. In issue #22 the
owner chose to show its source name and whether it matches a recognized
official label without letting that status change any grade. Name matches do
not authenticate configured sources. Four signals were rejected as too noisy:
licence string alone, grepping binaries for "telemetry", package descriptions,
and popularity.

The v1 implementation does not deliver every accepted family. D7 §5.2 defers
live endpoint matching because there is no reviewed endpoint catalogue or
reliable process attribution; the report says telemetry attribution is
unknown. privacybit now measures resolved Flatpak sandbox permissions per
installed copy and grades that axis separately from the catalogue. Unsupported
permission groups or missing coverage stay unknown. Reports expose origin facts
from sources that can provide them, with source names and recognized official
labels as context. A recognized name does not authenticate the configured
source. Origin never affects grades. Contradictions in other grade-bearing
metadata can worsen catalogue entry grades; sandbox evidence contributes
separately to the overall grade.

### 3.2 Actions escalate to real privilege — and the owner insisted on it

The action model has five tiers (inform → acknowledge → configure → guided flow
→ **root helper**). The owner initially accepted a stub at the top tier, saw
the tier list, and escalated: moonbit has a real root helper, so privacybit
would too.

**Two hard limits were then imposed, both at the owner's direction:**

- **Nothing that impacts encryption. Ever.** Their words: *"we want to be
  paranoid about user data just like moonbit. That means anything that impacts
  encryption is off the table."* Full-disk encryption was removed from the
  helper entirely. It is also a security rather than privacy property, so the
  project's own framing already excluded it.
- **No install verb.** Installing introduces new code that will later execute,
  possibly as root through a unit the package ships — and no review step can
  bound that future crossing. moonbit has never had a `pacman -S` in its
  codebase; adding one would have made privacybit less safe than the project it
  is modelled on. Installing an alternative is tier 3: the tool prints the
  command and the user runs it.

They endorsed the existing complexity when told about it: *"moonbit priv
escalation is quite complex for very good reasons."*

### 3.3 Reuse from moonbit is unrestricted, and deliberately not abstracted

The owner clarified that **moonbit is entirely theirs**: *"if it makes sense to
re-use or copy whatever we like we can, we have 100% ownership of it."*

That removed licensing as a constraint. What it did **not** justify was a shared
moonbit/privacybit library. The genuinely-shared surface is ~900 lines, and
every file in it needs editing on arrival — moonbit-specific strings in
`errors.go`, the lock path in `oplock.go`, unit names in `units.go`. A shared
module would have been a merge queue, not reusable code. **Copy the leaves,
lift the patterns, do not build a library.**

### 3.4 The daemon started as a "shield" and was cut back to an event watcher

The owner proposed that the daemon be **active** — not merely periodic, but
continuously defending. The idea was explored seriously and **rejected on
evidence**, which is recorded here because the reasoning is reusable:

- IP-based blocklists break real services by default. Google, Meta,
  Cloudflare and Microsoft serve mail, login, docs and video from the same
  frontends as their telemetry.
- Blocking at DNS fights whatever owns the resolver. **On this machine
  `resolvectl status` reports `resolv.conf mode: foreign`** — Tailscale
  MagicDNS owns `/etc/resolv.conf`, so a DNS shield would insert itself into a
  path another client already holds.
- DoH defeats a DNS layer entirely — in exactly the browser you would most want
  shielded.
- Per-process blocking without a kernel module is a project (OpenSnitch), not
  a feature.

**What survived**, after the owner reviewed the list:

| Capability | Outcome |
|---|---|
| Continuous disclosure detection (PID→exe→known endpoint) | Kept as a design direction; deferred from v1 by D7 §5.2 until endpoint data and attribution are reviewed. The report remains unknown. |
| New-application alert with its grade | **kept** |
| VPN leak **monitoring** | cut — *"complex and not something we want"* |
| Per-application network history | cut — *"should be managed at the firewall level, not via a privacy daemon"* |

Their firewall argument sharpened the whole design: **a firewall logs
destinations, it does not know `metrics.tiktok.com` is a tracker.** You could
log every byte and still not know. privacybit knows because the catalogue says
so. *Logging is the firewall's job; judging against known-bad destinations is
the catalogue's job.*

**The daemon therefore justifies itself on exactly one thing: event watching.**
Periodic scans are a timer. Reports are on-demand. moonbit already makes its
daemon `Conflicts=` its timers so only one mode runs. **If event watching is
cut, the daemon should not exist** — that was stated plainly and accepted.

Instant alerts via a pacman `PostInstall` hook were considered and deferred: it
runs as root and is permanent attack surface on every future package
transaction, for a latency improvement. Polling (`pacman -Qq` ~100 ms) ships
first.

### 3.5 The daemon is a *user* daemon, not a root daemon

moonbit's daemon runs as root. **privacybit's must not**, because a root
daemon's "current user" is `/root`, which breaks per-user scoping on a
multi-user machine — and all the unattended work is user-scoped anyway
(`~/.mozilla`, `~/.config`, `~/.local/share/flatpak`).

moonbit needs root at 3am only for `pacman -Rns`, which is Class C here. Apply
the direction rule to scheduling and the root requirement disappears.

It also improves the panel socket: moonbit's is `0666` and must answer only
ping/status as *policy*. A user socket at `0600` in `$XDG_RUNTIME_DIR` makes
privileged reach impossible **by construction**.

### 3.6 Unattended fixes — the Class A/B/C model

| Class | What | Unattended? |
|---|---|---|
| **A** | Scan, report, coverage | Always |
| **B** | Preference toggles — Firefox/Thunderbird telemetry prefs, flatpak override tightening, disabling a *timer* (not a service) | **Yes, only these** |
| **C** | Purge a package, enable anything, touch disk encryption, act on a cached scan | **Never** |

The admission test for Class B:

> **An auto-fix must be monotone toward privacy and reversible in one command.**

`fwupd` is the discriminating case. Disabling `fwupd.service` stops
phone-home **and** firmware updates — a security regression dressed as a
privacy win, so it **fails**. `fwupd-refresh.timer` only refreshes the metadata
list, leaving manual updates working, so it **passes**. Same vendor, same
intent, different class, decided entirely by what else the switch turned off.

**Cadence is a safety parameter here, not a performance one.** Frequent
unattended action on privacy posture means acting on stale knowledge. The
timer therefore re-scans in the same run and never auto-fixes from a cache.

### 3.7 The host maps warning to activity

sysc-shell `main` accepts `plugin/v1.ToneWarning` (`warning`) at protocol minor
18 and maps it to `ui.ToneActivity`, which paints the theme's **Tertiary** role.
The privacy panel reuses that wire value for amber. The integration needs no
new tone or protocol bump. `ui.Tone` is an iota enum; any future tone must be
**appended, never inserted**, or every later value shifts.

### 3.8 The repo owner forbids AI attribution in commits

Not a preference — a global commit hook enforces it, and it has **deliberately
not** been narrowed even when offered. The rejection pattern includes `bot`
without word boundaries, which matches inside **"Both"**. Four legitimate commit
messages have been rejected this way, including one that documented this very
trap. Reword; never weaken the hook. Full detail in `HANDOVER.md` §7.2.

### 3.9 Phase 4 findings and actions

On 2026-10-20, the owner approved a conservative Phase 4 scope. Known installed
entries graded red or amber and judged unmet expectations become findings.
Red maps to high severity; amber maps to medium. Findings expose tier 0
information and tier 3 alternative instructions. Tier 1 waits for dismissal
storage, and tier 2 waits for modeled settings. The builder never runs a
command. Unmatched entries and expectations blocked by coverage stay in their
existing result fields.

---

## 4. What is built

Delivery phases 0–12 are implemented across privacybit, sysc-plugins, and
sysc-shell. The privacy plugin has a user-placeable bar widget and a
plugin-owned audit panel. The Control Centre shows a compact audit summary and
opens that panel. Privacybit `v0.1.2`, plugin release `privacy-v0.1.2`, and AUR
package `0.1.2-1` are published. Catalog PR #275 merged at `2f67de4`, and a
disposable sysc-shell store install verified plugin `0.1.2`. The last successful
SSH check on 2026-10-30 found privacybit `dc799e8`,
privacy plugin code `ce6ffda` with manifest version `0.1.3`, and a sysc-shell
binary built from module revision `2ed94254`. The shell service and privacy
plugin process were active, and the privacybit build metadata was clean.
GitHub reported privacybit main at `28fc591` after PR #112, with implementation
commit `cb87533`; sysc-plugins main at `e01569e`; and sysc-shell main at
`0f24b029` after PR #572 cleared a city-defined Weather place. PR #296 changed
manifest validation and publishing docs, not plugin runtime code. GitHub
history confirms testbench build `dc799e85` descends from privacybit PR #97
(`eb45f2a`) and PR #100 (`f8f093e`), so both TUI fixes are in that build. The
recorded SSH check did not exercise the TUI interactively. Build `dc799e85`
predates PR #112 (`cb87533`), so it lacks the breadcrumb change. Issue #114
tracks a rebuild and interactive TUI verification. A fresh SSH attempt during
this audit returned `No route to host`, so treat the prior testbench check as
historical and its current state as unconfirmed. See `HANDOVER.md` for revision
and verification evidence.

---

## 5. Approved panel amendment

The owner approved GPL-3.0-or-later for privacybit and, in D5 §11 on
2026-10-23, a plugin-owned bar widget and audit panel. The panel runs
`privacybit audit --snapshot` as the current user. It refreshes the bounded
snapshot without writing a saved report or invoking privileged actions. Plugin
release `privacy-v0.1.2` fixes the manifest and handshake at version `0.1.2`.
Catalog PR #275 merged at `2f67de4`, and a disposable sysc-shell store install
verified the release; the AUR package is `0.1.2-1`.

---

## 6. Panel control boundary

The first design used a host-rendered viewer because the original publisher had
no plugin-owned view. D5 §11 amended that design on 2026-10-23: the plugin now
declares a bar widget and its own audit panel. The bar widget and the Control
Centre's `Open full audit` action open the same panel. The plugin runs
`privacybit audit --snapshot` as the current user, applies a timeout, and
publishes the result to both views. The Control Centre no longer starts a scan
or reads the saved report. Its capture activity card remains unchanged.

| Capability | Current implementation |
|---|---|
| Show the overall grade and currentness in Control Centre | Compact host summary with an `Open full audit` route |
| Open the full audit from the bar | User-placeable plugin bar widget opens the plugin panel |
| Show categories, network checks, findings, coverage, and scan status | Plugin-owned panel renders the bounded snapshot and supports `Scan now` |
| Run tier 4 actions | Stays in the TUI/helper; the panel cannot invoke them |

---

## 7. Remaining non-phase work

- **Laptop testbench:** See §4 for the last successful SSH check and recorded
  revisions. Build `dc799e85` includes TUI fixes from PRs #97 and #100 but
  predates PR #112's breadcrumbs. Issue #114 tracks a rebuild and interactive
  check. A fresh SSH attempt during this audit returned `No route to host`, so
  current reachability and process state are unconfirmed.
- **Publication** is complete through privacybit `v0.1.2`, plugin release
  `privacy-v0.1.2`, catalog entry `0.1.2`, and AUR package `0.1.2-1`. A
  disposable sysc-shell store install verified the amd64 release. Coordinated
  v0.1.3 publication remains gated on owner approval in issues #41 and #280.
- Additional package-manager backends remain deferred under D6. Flatpak's
  resolved permissions are graded per installed copy; unsupported permission
  groups and missing coverage remain unknown.
- Live telemetry endpoint attribution remains deferred under D7 §5.2; v1
  reports it as unknown rather than claiming no connections were found.

---

## 8. A closing warning

This project's most common failure has been **claiming something works without
running the thing that would prove it**. The build was broken for several
turns while being reported as compiling clean, because no build had been run.
A rule that lived only in a comment shipped in the very file written to
prevent that.

Verify with a command. Every claim in `HANDOVER.md` that says the code builds
is itself subject to that rule, and one of them already was wrong.

**Do not weaken an invariant to make something work.** If I2 blocks a feature,
the feature is wrong, not the invariant.
