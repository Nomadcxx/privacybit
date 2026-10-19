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

Three things in here will not be findable anywhere else:

1. The original request, in the owner's own terms.
2. Decisions that were made in dialogue and never written down.
3. **An open tension between what the owner wants next and what D5 currently
   specifies** (§6). Read that before building phase 10.

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

## 2. Why a standalone project rather than a plugin

The decisive architectural finding was this: **a plugin cannot render UI inside
a Control Centre section.**

The Control Centre is entirely host-rendered. `ccPage` in
`internal/shell/popout_controlcenter.go` is a `switch h.section` over host-side
functions, and **there is no plugin branch**. A plugin can publish bounded JSON
into its own state, and the host renders it natively — `org.sysc.calendar`
writing to state key `control_center` is the existing, working precedent.

So the split is:

```
privacybit (library + CLI)  →  plugin publishes a JSON snapshot  →  host renders it
```

This gives the plugin-only benefits (crash isolation, catalogue distribution,
independent versioning) while keeping the UI where the user expects it. And it
forces the engine to be a library rather than shell glue, which is what makes
the CLI, the TUI and the panel run *one* implementation instead of three.

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

Four derived signals were accepted as reliable enough to grade on: live network
connections to known telemetry endpoints, packaging origin, sandbox posture,
and catalogue/package metadata mismatch. Four were explicitly rejected as too
noisy: licence string alone, grepping binaries for "telemetry", package
descriptions, and "is it popular".

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
| Continuous disclosure detection (PID→exe→known endpoint) | **kept** |
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

### 3.7 A correction about the host: the amber tone already exists

Every design document originally claimed sysc-shell's plugin protocol had no
amber/warning tone, and that adding one would require theme work. **That was
wrong.** `internal/ui/tree.go` has defined `ToneActivity` for some time,
painting the theme's **Tertiary** role, and it is already used by the monitor
page — whose tests assert exactly the normal→activity→error progression
privacybit grades want.

Only the *wire* is missing: two edits in `plugin/v1/node.go`. No theme change
is needed, because Tertiary exists on every palette. **Trap:** `ui.Tone` is an
iota enum — a new tone must be **appended, never inserted**, or every later
value shifts.

### 3.8 The repo owner forbids AI attribution in commits

Not a preference — a global commit hook enforces it, and it has **deliberately
not** been narrowed even when offered. The rejection pattern includes `bot`
without word boundaries, which matches inside **"Both"**. Four legitimate commit
messages have been rejected this way, including one that documented this very
trap. Reword; never weaken the hook. Full detail in `HANDOVER.md` §7.2.

---

## 4. What is built, in one paragraph

Seven design documents, complete. Two implementation phases: the catalogue
(entry model, loader, validator, ten categories of YAML) and the detection
layer (pacman, dpkg, flatpak, `.desktop` binaries and desktop entries, each
behind one interface with three-state availability). Build, vet, gofmt and tests
are green. **The grader does not exist yet**, so privacybit today reads its
catalogue and describes what it found, but does not yet assign grades. See
`HANDOVER.md` §10.1.

---

## 5. What comes next

The owner's stated direction, in order:

1. **Finish privacybit** — grader, findings and actions, network checks, the
   `audit` command.
2. **Redesign the privacy panel inside the sysc-shell Control Centre.** It is
   currently one small card showing three capture rows. The redesign adds the
   audit: categories, grades, the achievable wins, coverage honesty.
3. **Build a comprehensive sysc-shell plugin** that wires privacybit into that
   panel and can **read and control** it.

The panel has room: the Control Centre body resolves to roughly 596 × 480
inside a scroll viewport, and the existing privacy page uses a single card
with no scroll and no controls. Adding a second card is additive, not a
restructure.

---

## 6. An open tension — resolve this before building phase 10

The owner asks for a plugin that can **read and control** privacybit.

**D5 §4.2 currently specifies the opposite**: that the panel performs *no*
privileged operation, and only two verbs — `privacybit:refresh` and
`privacybit:explain`. Its reasoning was recorded at the time:

> *A control centre that can remove software with one click is a bad control
> centre.*

That reasoning still holds for **tier 4**. But the owner has since asked for
control, so the next agent must not silently re-derive "read-only" from the
design doc and assume it is settled. **Ask.**

There is a coherent middle position, and it is probably what is wanted:

| Capability | Tier | Root? | Reasonable in a panel? |
|---|---|---|---|
| Show grades, findings, coverage | read | no | Yes — the core |
| Re-run the audit on demand | 0 | no | Yes |
| Explain a finding in place | 0 | no | Yes |
| Toggle a preference-backed entry | 2 | no | Yes, with an undo affordance |
| Open the TUI, or print the command | 3 | no | Yes |
| Trigger the scheduled scan now | 0 | no | Yes |
| **Purge a package** | **4** | **yes** | **Not without a real confirm and D4's review token** |

Everything above the line is achievable without root and without weakening
invariant I4. Below it, D4's token is the control and the UI confirm is only a
courtesy — and remember D5 wires `privacybit:run-action` into
`activateControlCentre`, which bypasses the TUI entirely, so **the helper's
check is the only thing standing between the panel and an unattended
`pacman -Rns`**.

---

## 7. Open questions to put to the owner

1. **Does the panel control include tier 4?** See §6. If yes, the host needs a
   second, privileged path — and D5's read-only assumption must be revised
   explicitly rather than drifted past.
2. **Are the point-in-time VPN checks still wanted?** Continuous VPN *monitoring*
   was cut and per-app history was cut, but D1 §7's point-in-time checks — is
   the default route actually going through the tunnel, is DNS leaking, is
   IPv6 escaping — were their original request and appear never to have been
   separately confirmed as cut. **Ask before building.**
3. **What does "re-design the privacy panel" mean visually?** Whether the
   capture card stays, shrinks, or becomes a summary row is unresolved and
   affects the layout budget.
4. **Monthly cadence confirmed?** The daemon design assumes one monthly scan.
5. **Licence.** The repository has no licence file yet.

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