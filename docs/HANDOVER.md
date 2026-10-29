# privacybit — project handover

**Written:** 2026-10-30
**This file's absolute path:** `/home/nomadx/privacy-bit/docs/HANDOVER.md`
**Repository:** `https://github.com/Nomadcxx/privacybit` (branch `main`)
**Module:** `github.com/Nomadcxx/privacybit` (Go 1.26.4)

**Reader:** an engineer or agent with **no prior context on this project.**
**Purpose:** audit what has been built, audit the plan, change the plan if it is
wrong, then continue development to completion.

This document is a current status snapshot and design-audit record. Read the
seven design documents in `/home/nomadx/privacy-bit/docs/plans/` for the
authoritative product rules.

**Read `/home/nomadx/privacy-bit/docs/CONTEXT.md` first.** It carries the
project origin and the decisions the owner approved during design review.

---

## 0. How to use this document

Three different jobs, three reading orders:

| If you want to… | Read |
|---|---|
| **Continue development** | §2 (paths) → §3 (file manifest) → §4 (invariants) → §6.2 (delivery phases) → then the relevant design doc |
| **Audit the code written so far** | §8 (falsifiable claims with commands) → §10 (known defects) → §3 |
| **Audit the plan itself** | §9 (opinions, not derivations) → §4 (invariants) → §10.2 (places the docs were already wrong) |

### 0.1 Three things that are easy to get wrong

**1. Under-grading is the one unacceptable error.** Every other failure here is
recoverable: a missed app, a stale catalogue entry, a wrong install command.
Telling someone their privacy setup is fine when it is not corrodes trust in
every other claim the tool makes, and the user cannot detect it. Most of this
codebase's structure exists to make a false green difficult to write.

**2. Nothing shown may read as a pass.** An empty result, an empty card, a
silent failure, and a genuinely clean result must be visually distinct. This has
already produced three real bugs — see §10.3 — and it is the single easiest
thing for a new contributor to reintroduce.

**3. The design documents are authoritative for *why*. The code is
authoritative for *what*.** Where they disagree, that is a bug in one of them:
find it and fix it explicitly. See the corrected decisions and open conflicts in
§10.2.

---

## 1. What this project is

**privacybit** is a privacy audit for Linux desktops. It inspects what is
installed, what is running, and what is going over the network, then reports a
graded breakdown by category with a stated reason for every grade and a concrete
action for every finding.

It exists to fill a gap in the sysc-shell Control Centre privacy panel, which
today answers only *"is something capturing my microphone/camera/screen right
now"*. It does not answer *"is my setup reasonable"*. Those are different
questions and conflating them would make both worse.

### 1.1 Vocabulary — this distinction is load-bearing

- **Privacy** — who can access your information.
- **Security** — protection from unauthorised access or theft.
- **Anonymity** — your actions cannot be traced back to you.

They are **not interdependent**. Google systems are secure and unlikely to be
hacked, yet Google still has access to your personal data. Signal offers high
privacy but not anonymity, because registration requires a phone number.

privacybit grades **privacy**. Where something is really a security or anonymity
concern it is excluded or labelled. UI copy must never imply that "green" means
"private" in the strong sense. Green means: *no meaningful unwanted disclosure
was identified by the checks we actually ran* — which is why invariant I2 (§4)
matters so much.

### 1.2 The three failure modes the design exists to avoid

| Failure | Why it is fatal |
|---|---|
| **Under-grading** | Undetectable by the user; destroys trust in every other claim. |
| **Wall-of-shame** | A report opening with fourteen red items and no wins gets deleted. Lead with the summary and the achievable fixes. |
| **False positives** | One wrong accusation destroys trust faster than ten misses. When uncertain, say `unknown`. |

---

## 2. Where the work is

### 2.1 Repositories and working copies

| Absolute path | What it is | State |
|---|---|---|
| `/home/nomadx/privacy-bit` | **This project.** Canonical checkout on branch `main`. | This handover snapshot was checked against upstream commit `754114d` after PR #47. The primary checkout contains unrelated untracked `.commandcode/` data; leave it untouched and use a clean worktree from `origin/main`. Public privacybit `v0.1.2` at `7c63797` remains current; PR #42 updates the AUR recipe for `0.1.3`. Issue #20 is closed. |
| `/home/nomadx/worktrees/privacybit-origin-context` | Historical privacybit issue #22 worktree. | Branch `feat/origin-context` at `54c9fb4`; privacybit PR #23 merged the context-only origin implementation. |
| `/home/nomadx/sysc-plugins/.worktrees/privacybit-origin-context` | Historical sysc-plugins issue #22 worktree. | Branch `feat/privacybit-origin-context`; sysc-plugins PR #264 merged the panel integration and refreshed screenshot and thumbnail. |
| `/home/nomadx/privacy-bit/docs/plans/` | Seven original design documents plus follow-on designs and implementation plans. | D1–D7 record the approved design. Phase 3, 4, 8, and 9 plans remain as audit history. |
| `Nomadcxx/sysc-plugins` `main` | Upstream revision `11f0202` after PR #285. | PR #277 updated the privacy panel's origin wording; PR #279 makes the plugin wait for Docker workers on shutdown; PR #281 prepares manifest version `0.1.3`; PR #283 adds explicit first-scan progress; PR #285 aligns stale-result copy with an active refresh. Issues #276, #278, and #284 are closed. Release `privacy-v0.1.2` still points to tested commit `21b4944`. Catalog PR #275 merged at `2f67de4`; its amd64 and arm64 assets have verified SHA-256 digests, and issue #268 is closed. Release rehearsal run `40934309464` passed both architecture builds; its manual run skipped publication and catalog changes. |
| `Nomadcxx/sysc-shell` `main` | Upstream revision `2f7c2a8` after PR #559. | PRs #436 and #437 add the Control Centre summary and route `Open full audit` to the plugin panel; PR #559 repairs plugin bar drop and press states. |
| `nomadx@192.168.0.64:7777` laptop testbench | Plugin worktree `/home/nomadx/worktrees/privacybit-plugin-testbench-20261023`. | Last reachable check over SSH was 2026-10-29. The installed `privacybit` embeds privacybit `54c9fb4`; the plugin embeds sysc-plugins `9b44520`; the shell embeds `ca3c988`; `sysc-shell.service` is active and its bar placement is enabled. SSH returned `No route to host` on 2026-10-30, including this audit; the current upstream revisions have not been deployed or verified there. |
| `/home/nomadx/sysc-plugins` | Primary plugin checkout. | Branch `fix/kdeconnect-pairing-card-routing` has independent dirty changes, including an untracked `plugins/privacy`. Do not clean or repurpose it; upstream `main` is the source of truth for the published plugin. |
| `/home/nomadx/sysc-shell` | Primary shell checkout. | Branch `audit/toasthost-wayland-crash` is behind upstream and has independent dirty changes and Beads state. Do not clean or deploy it; upstream `main` is the source of truth for the merged integration. |
| `/home/nomadx/sysc-plugins/cmd/sysc-plugin-calendar/main.go` | The precedent to copy for the plugin. | Reference only. |
| `/home/nomadx/sysc-plugins/plugins/calendar/manifest.json` | Manifest shape to copy. | Reference only. |
| `/home/nomadx/moonbit` | Sibling project whose helper and daemon patterns informed this design. | Check `origin/main` before reusing its current implementation. |

### 2.2 Privacybit integration files on upstream main

sysc-plugins merged PRs #207/#208/#259/#264/#265/#267/#283/#285, and sysc-shell
merged PRs #436/#437/#559. The plugin owns the full audit panel and scan
action; sysc-shell owns the Control Centre summary and route. The testbench's
last verified revisions appear in §2.1; later upstream changes remain
undeployed there.

| File | Role |
|---|---|
| `sysc-plugins/plugins/privacy/manifest.json` | Declares the `bar` widget and `panel` entry under `org.sysc.privacy`. |
| `sysc-plugins/plugins/privacy/view.go` | Renders the overall grade in the bar and the detailed, scrollable audit panel. |
| `sysc-plugins/cmd/sysc-plugin-privacy/main.go` | Publishes bounded state, refreshes the snapshot with `privacybit audit --snapshot`, and opens the panel from the bar event. |
| `sysc-shell/internal/shell/controlcenter_pages.go` | Preserves capture activity and renders the compact audit summary with an `Open full audit` action. |
| `sysc-shell/internal/shell/popout_controlcenter.go` | Routes `Open full audit` to the plugin's declared audit panel. |
| `sysc-shell/internal/shell/controlcenter_test.go` | Covers snapshot validation, currentness, grade presentation, and the audit summary. |
| `sysc-shell/internal/shell/privacy_test.go` | Covers the existing microphone, camera, and screen-share activity shown above the audit summary. |

The full panel displays categories, network checks, findings, and coverage
limits. It treats stale and unavailable snapshots as unrated. PR #283 shows a
pending state during the first scan. PR #285 avoids asking for another scan
while a stale result is already refreshing. The Control Centre no longer
launches scans or reads the saved report; its `Open full audit` action and the
bar widget open the plugin panel. The last testbench check on 2026-10-29 found
the plugin at `9b44520` with its `bar` widget and `panel` entry enabled. SSH
attempts on 2026-10-30, including this audit, returned `No route to host`.

Privacybit PR #16 adds the per-copy Flatpak sandbox grade, and PR #23 adds
package-origin context without changing grades. sysc-plugins PR #259 displays
sandbox grades, and PR #264 displays origin context. Privacybit `v0.1.2`
includes both changes. Privacybit PR #38 clarifies that recognized source
labels are not authenticated; sysc-plugins PR #277 carries that wording into
the panel. The published `v0.1.2` CLI and plugin assets predate that correction.
Privacybit PR #42 updates the AUR recipe for `v0.1.3`; sysc-plugins PR #281
bumps the plugin manifest to `0.1.3`. Both PRs merged, but their release tags
and AUR publication remain pending owner approval. Plugin release
`privacy-v0.1.2` fixes the manifest and handshake version at `0.1.2`; catalog
PR #275 merged its URLs and checksums. Issues #20, #37, #268, and #276 are
closed. A disposable sysc-shell store install resolved the live catalog,
verified the amd64 asset and manifest, and recorded version `0.1.2`.

---

## 3. Implementation map

The early source and catalogue tables below come from the phase 1–4 audit and
do not list the later phase packages. Use `rg --files` for a complete current
inventory. The phase map in §11 lists the added subsystems.

### 3.1 Original core source inventory

| Lines | Absolute path | Purpose |
|---|---|---|
| 289 | `/home/nomadx/privacy-bit/internal/catalog/catalog.go` | `Grade`, `Risk()`/`Worse()`, `Word()`/`Glyph()`/`ASCII()`, exact detect rules and lookup, `Entry`, `Expectation`, `Category`, `Set` |
| 493 | `/home/nomadx/privacy-bit/internal/audit/audit.go` | Inventory evaluator, measured-signal grading, findings, tier 0/3 actions, expectation and coverage roll-up |
| 287 | `/home/nomadx/privacy-bit/internal/catalog/load.go` | `//go:embed data/*.yaml`, `Load()`, strict one-document parsing, fragment merge, effective-catalogue revision, index |
| 254 | `/home/nomadx/privacy-bit/internal/catalog/validate.go` | `Problem`/`Problems.Err()`, `(s *Set) validate()`, `ids()`, `categoryNames()` |
| 237 | `/home/nomadx/privacy-bit/internal/detect/detect.go` | `Signal[T]` (including measured-state helpers), `Availability`, `Capabilities`, `Origin`, `Facts`, scoped `Observation`, `Source` interface |
| 103 | `/home/nomadx/privacy-bit/internal/detect/runner.go` | `Runner` interface, `ExecRunner`, `MapRunner` for tests, `commandTimeout = 20s` |
| 199 | `/home/nomadx/privacy-bit/internal/detect/pacman.go` | `Pacman` — enriches catalogue matches only; retains a package if enrichment fails |
| 150 | `/home/nomadx/privacy-bit/internal/detect/dpkg.go` | `Dpkg` — the I2 reference case |
| 133 | `/home/nomadx/privacy-bit/internal/detect/flatpak.go` | `Flatpak` — preserves installation copies and queries resolved permissions by exact ref and scope |
| 185 | `/home/nomadx/privacy-bit/internal/detect/desktop.go` | `Desktop` — universal `.desktop` scan |
| 210 | `/home/nomadx/privacy-bit/internal/detect/binary.go` | `Binary` — universal PATH/opt scan, nix-store exclusion |
| 102 | `/home/nomadx/privacy-bit/internal/render/install.go` | D6 per-family install command and HTTPS upstream rendering |
| 119 | `/home/nomadx/privacy-bit/cmd/privacybit/main.go` | Cobra root, `loadCatalogue()`, package doc |
| 46 | `/home/nomadx/privacy-bit/cmd/privacybit/util.go` | `useColour`, `isTerminal()`, `userFragmentPath()` |
| 142 | `/home/nomadx/privacy-bit/cmd/privacybit/list.go` | `list` + `gradeCell()` |
| 201 | `/home/nomadx/privacy-bit/cmd/privacybit/explain.go` | `explain` + shared install renderer |
| 56 | `/home/nomadx/privacy-bit/cmd/privacybit/catalog_cmd.go` | `catalog validate`, `catalog path` |

### 3.2 Original core test inventory

| Lines | Absolute path | Tests |
|---|---|---|
| 207 | `/home/nomadx/privacy-bit/internal/catalog/load_test.go` | 9 — see §8 |
| 516 | `/home/nomadx/privacy-bit/internal/detect/detect_test.go` | Detection, failure, cancellation, Flatpak ref and installation coverage |
| 467 | `/home/nomadx/privacy-bit/internal/audit/audit_test.go` | Grading, coverage, findings, actions and Flatpak scope reporting |
| 85 | `/home/nomadx/privacy-bit/internal/render/install_test.go` | Distribution commands, HTTPS fallback, unsafe route rejection and shipped route checks |

Command tests now cover audit snapshots, purge plans, scheduling, and TUI startup.
The early test table above omits later phase tests.

### 3.3 Shipped catalogue data — `/home/nomadx/privacy-bit/internal/catalog/data/`

Ten files, loaded in filename order so `list` output is alphabetical.

| Lines | Absolute path | Category id |
|---|---|---|
| 191 | `…/data/addons.yaml` | `addons` |
| 453 | `…/data/browsers.yaml` | `browser` |
| 466 | `…/data/chat.yaml` | `chat` |
| 230 | `…/data/email.yaml` | `email` |
| 234 | `…/data/media.yaml` | `media` |
| 107 | `…/data/metadata.yaml` | `metadata` |
| 163 | `…/data/office.yaml` | `office` |
| 299 | `…/data/passwords.yaml` | `passwords` |
| 214 | `…/data/pdf.yaml` | `pdf` |
| 301 | `…/data/vpn.yaml` | `vpn` |

Every file is exactly one document with a single `category:` key. The YAML
schema is pinned by the struct tags in `catalog.go`; `load.go` uses
`dec.KnownFields(true)` so an unknown key is a **load error**, not a silently
ignored typo.

### 3.4 Documentation

| Lines | Absolute path |
|---|---|
| 889 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-design.md` (**D1** — engine) |
| 333 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-cli-design.md` (**D2** — CLI) |
| 283 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-tui-design.md` (**D3** — TUI) |
| 427 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-helper-design.md` (**D4** — root helper) |
| 467 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-plugin-design.md` (**D5** — plugin + host) |
| 410 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-sources-design.md` (**D6** — multi-distro sources) |
| 262 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-daemon-design.md` (**D7** — daemon) |
| 73 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-phase-3-audit-grader.md` (Phase 3 implementation plan) |
| 46 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-phase-4-findings-actions-design.md` (Phase 4 approved design) |
| 67 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-phase-4-findings-actions-plan.md` (Phase 4 implementation plan) |
| — | `/home/nomadx/privacy-bit/docs/plans/2026-10-29-origin-context-design.md` (Issue #22 approved design) |
| — | `/home/nomadx/privacy-bit/docs/plans/2026-10-29-origin-context-plan.md` (Issue #22 implementation plan) |
| 25 | `/home/nomadx/privacy-bit/docs/plans/2026-10-29-privacybit-ci-design.md` (Issue #24 CI design) |
| 36 | `/home/nomadx/privacy-bit/docs/plans/2026-10-29-privacybit-ci-plan.md` (Issue #24 implementation plan) |
| — | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-phase-8-helper-plan.md` (Phase 8 implementation plan) |
| — | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-phase-9-daemon-plan.md` (Phase 9 implementation plan) |
| 67 | `/home/nomadx/privacy-bit/docs/plans/2026-10-23-flatpak-sandbox-grade-design.md` (Issue #13 approved design) |
| 90 | `/home/nomadx/privacy-bit/docs/plans/2026-10-23-flatpak-sandbox-grade-plan.md` (Issue #13 implementation plan) |
| 100 | `/home/nomadx/privacy-bit/README.md` |
| — | `/home/nomadx/privacy-bit/docs/screenshots/privacybit-overview.png` (synthetic report view) |
| — | `/home/nomadx/privacy-bit/docs/screenshots/privacybit-categories.png` (synthetic category view) |
| — | `/home/nomadx/sysc-plugins/.worktrees/privacybit-panel-20261023/plugins/privacy/screenshot.png` (synthetic native Privacy page) |
| — | `/home/nomadx/sysc-plugins/.worktrees/privacybit-panel-20261023/plugins/privacy/thumbnail.webp` (generated catalog thumbnail) |
| — | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-installer-design.md` |
| — | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-installer.md` |
| — | `/home/nomadx/sysc-shell/docs/plans/2026-10-20-privacybit-control-centre-scan.md` (Phase 12 integration plan) |

**Reading order if you are new:** D1 §1–§5 (rating model, identity model,
catalogue), then D6 §3 (the invariant), then D2 (how the CLI surfaces it).
The rest as you reach it.

### 3.5 Other files

| Absolute path | Purpose |
|---|---|
| `/home/nomadx/privacy-bit/go.mod` | Module dependencies: Cobra, Bubble Tea, Lipgloss, YAML v3, plus transitive modules |
| `/home/nomadx/privacy-bit/.gitignore` | Excludes audit output — it fingerprints what a user runs |
| `/home/nomadx/privacy-bit/cmd/privacybit-install/` | Unprivileged TUI application installer |
| `/home/nomadx/privacy-bit/internal/installer/binary.go` | Offline build and atomic install to `~/.local/bin` |
| `/home/nomadx/privacy-bit/internal/audit/flatpak.go` | Effective permission classification, generic risk bands, and Flatpak coverage roll-up |
| `/home/nomadx/privacy-bit/packaging/aur/` | `PKGBUILD` and generated `.SRCINFO` for the first Arch package |
| `/home/nomadx/privacy-bit/.github/workflows/ci.yml` | Pull request and main-branch formatting, test, vet, and build checks |

There is **no `testdata/` directory** anywhere in the repo. `internal/detect`
fixtures are inline string literals in `detect_test.go`; `internal/catalog`
needs none because it loads the real embedded YAML.

---

## 4. The five invariants

These are the load-bearing rules. **If you change the plan, do not weaken an
invariant to make something work.** Break one and you have a tool that lies
confidently.

### I1 — Derived signals are monotone

A measured fact about the live system may only push a grade **toward more risk**,
or raise a flag. It may **never improve** a grade.

There is no runtime path from `red` to `amber`. Reduction happens only when the
user changes the system and the audit re-runs from scratch.

*Why:* the heuristic will be wrong sometimes, and the error mode of a heuristic
that can improve a grade is a grade that silently improves.

### I2 — Unavailable or unset signals cannot move a grade; contradictions can only worsen it

Four rules:
1. `Observed` and `Contradicted` signals carry measured evidence. Neither may
   improve a grade; `Contradicted` may only worsen it.
2. `Unavailable` and the zero state cannot move a grade in either direction.
3. The catalogue grade is always the floor.
4. Insufficient coverage to judge means `unknown`, never `green`.

This is the general form of D1 §6.4's flatpak rule and D6 §3.2. It is the most
important rule in the detection layer.

*Concrete bug it prevents:* `dpkg` has no licence metadata. A Debian user with
a package the catalogue calls FOSS cannot be cross-checked, so the absence of a
reported contradiction is not evidence of agreement or a clean result — the
backend had nothing to say.

*Implementation:* `SignalState` distinguishes zero, observed, unavailable and
contradicted. `IsMeasured()` accepts only observed and contradicted states.
`TestZeroSignalIsNotObserved` and `TestContradictedSignalIsMeasuredEvidence`
pin the state API. `internal/audit` preserves the effective catalogue grade,
ignores zero and unavailable signals, and moves a contradicted entry one level
worse. `TestContradictedEvidenceWorsensTheCatalogueGradeByOne` pins the grade
rule; coverage tests pin `unknown` propagation.

### I3 — Absence is a finding, not a pass

"No password manager installed" grades `red`. It does not grade empty. Every
category declares an `expectations` list; an unmet expectation is a finding in
its own right.

*Direction:* the table may be **over-broad** about what counts as satisfying an
expectation, never under-broad. 20 of 118 `any_of` tokens deliberately name
packages the catalogue does not grade.

### I4 — The privileged path removes and configures. It never introduces.

No install verb. No disk-encryption verb. Installing introduces new code that
will later execute — potentially as root, through a systemd unit the package
ships — and that is the one direction a review step cannot bound.

moonbit draws the same line and has for its whole life: its privileged verbs are
scan, clean, orphan removal, docker prune and schedule, with no `pacman -S`
anywhere in the codebase. **Adding one would make privacybit less safe than the
project it is modelled on.**

*Corollary:* the helper has no `install-entry`. Installing an alternative is a
**tier 3** action — the tool prints the exact command and the user runs it.

### I5 — Nothing shown may read as a pass

An empty result, an empty card, a silent failure, and a clean result must be
visually distinct. Enforced in the CLI, in the plugin's "unavailable" payload,
and in the validator's requirement that every entry can be explained.

---

## 5. Architecture

```
privacybit binary (CLI, TUI, and internal audit engine)
       ▲
       │ `privacybit audit --snapshot`
       │
org.sysc.privacy plugin
   ├── bar widget ────────────────┐
   ├── audit panel (`Scan now`) ───┤
   └── bounded state snapshot ──→ Control Centre summary
                                     └── `Open full audit` → audit panel
```

**The plugin shells out rather than importing the module.** The engine lives
under `internal/`, and Go forbids cross-module import of `internal/`; a module
dependency would force publishing the audit API publicly. `plugins/moonbit`
already shells out to its binary, so this is the house pattern. D1 §2.1 now
records the same architecture — see §10.2 item 2 for the correction history.

### 5.1 Package map

```
internal/catalog/     entry model, loader, validator, shipped catalogue
internal/detect/      package and universal inventory sources
internal/audit/       exact matching, grades, expectations, findings
internal/engine/      source orchestration and complete audit run
internal/network/     point-in-time VPN route, DNS, and IPv6 checks
internal/report/      shared report, NDJSON events, snapshot projection
internal/render/      distribution-aware install instructions
internal/tui/         Bubble Tea audit interface
internal/helper/      constrained root helper and action plans
internal/installer/   helper, service, and user-binary installation
internal/daemon/      scheduler, Unix socket, event watcher
cmd/privacybit/       CLI, audit, TUI, scheduled run, purge
cmd/privacybit-helper/ constrained helper executable
cmd/privacybit-installer/ installs the user watcher or timer
cmd/privacybit-install/  builds and installs privacybit for the current user
sysc-plugins/plugins/privacy   snapshot publisher, bar widget, audit panel
sysc-shell/internal/shell     snapshot reader, summary, panel route
```

`internal/report` owns shared report and grade rendering. The plugin shells
out to `privacybit audit --snapshot`; the host accepts bounded schema-1 state.
The panel reports telemetry attribution as unknown and shows stale snapshots as
unrated.

---

## 6. What is done

### 6.1 Design — complete, seven documents

See §3.4 for absolute paths. In brief:

| Doc | Covers |
|---|---|
| **D1** | Engine: rating model, identity model, categories, actions, tiers, privacy-of-tool rules |
| **D2** | CLI surface, NDJSON contract, exit codes, config |
| **D3** | Bubble Tea model, 13 screens, keybindings, navigation stack |
| **D4** | Root helper, closed verb set, review tokens, sudo dance |
| **D5** | sysc-plugins publisher and panel, plus the sysc-shell summary route |
| **D6** | Multi-distribution source layer, invariant I2 |
| **D7** | Daemon, event watcher, Class A/B/C, systemd units |

### 6.2 Delivery phases

Implementation for phases 0–12 is merged in privacybit, sysc-plugins, and
sysc-shell. Automated verification appears in §11. The privacy plugin is
published in the catalog and release. The laptop testbench last confirmed
older merged plugin and shell revisions on 2026-10-29; current upstream
deployment remains unverified because SSH returns `No route to host`.

| Phase | Deliverable | State |
|---|---|---|
| 0 | Repository and design | Complete |
| 1 | Catalogue, loader, validator, `list`, `explain` | Complete |
| 2 | Package and universal detection sources | Complete |
| 3 | Exact matching and conservative grading | Complete |
| 4 | Findings and tier 0/3 actions | Complete |
| 5 | Point-in-time VPN checks | Complete |
| 6 | Audit CLI, NDJSON, snapshot output, exit codes | Complete |
| 7 | TUI | Complete |
| 8 | Root helper and installer | Complete |
| 9 | Scheduler, daemon, Unix socket, event watcher | Complete |
| 10 | sysc-plugins privacy publisher | Complete |
| 11 | Control Centre audit summary with a route to the plugin panel | Complete |
| 12 | Plugin-owned bar, audit panel, read-only scan, and GPL-3.0-or-later licensing | Complete |

### 6.3 Phase 1 — catalogue

`/home/nomadx/privacy-bit/internal/catalog/`:

- `catalog.go` — `Grade` with `Risk()` ordering **green 0 < amber 1 < red 2 <
  unknown 3**. Unknown sorts *above* red deliberately: an unrated item has not
  been shown safe, and ranking it as better would be an under-grade. Three
  rendering channels: `Word()` (Good/Review/Risky/Unrated), `Glyph()` (●/▲/■/?),
  `ASCII()` (o/^/x/?). `DetectType` closed set (`package|binary|flatpak|desktop`)
  with `Confidence()` 2 for package/flatpak (primary evidence, named by the
  distribution) and 1 for binary/desktop (corroboration — a binary named
  `signal` is not necessarily Signal Desktop). Substring matching deliberately
  unsupported.
- `load.go` — `//go:embed data/*.yaml`; `dec.KnownFields(true)`; a malformed
  shipped file **returns an error rather than panicking** so ten files' errors
  aggregate; `revision()` produces `cat-<12hex>+<N fragments>` so a stored
  snapshot traces to the table that produced it.
- `validate.go` — aggregating `Problems.Err()` sorted by path; no detect key
  claimed by two entries; **every red entry must offer an alternative**.
- Ten data files — see §3.3.

`/home/nomadx/privacy-bit/cmd/privacybit/` — `list`, `explain`,
`catalog validate`, `catalog path`. Cobra. `--user-catalog` persistent flag.
`installCommand()` renders per distribution: arch→`pacman -S`,
debian→`apt install`, fedora→`dnf install`, suse→`zypper install`,
alpine→`apk add`, default→`""` so a missing key yields no command rather than an
unrunnable one.

### 6.4 Phase 2 — detection

`/home/nomadx/privacy-bit/internal/detect/`:

- `detect.go` — `Signal[T]{State, Value, Why}`, `Availability{Ready, Reason,
  Empty}`, `Capabilities`, `Origin{Source, Official}`, `Facts`, `Kind`,
  `Observation`, `Source` interface.
- `runner.go` — `Runner` interface so tests inject recorded output.
  `MapRunner` returns `unfixturedError` for an unfixtured command — **never
  empty output**, so an unfixtured call cannot masquerade as "a source with
  nothing to report".
- `pacman.go` — enriches **only catalogue matches**, keeping the slow tier
  affordable (`-Qq` ~100ms over 2,882 packages; `-Qi` orders slower). A missing
  `ValidatedBy` → `Unavailable`, not `false`.
- `dpkg.go` — the I2 reference case. Reports `Version` and uses
  `apt-mark showmanual` for explicit-install state, matched against
  architecture-qualified `${binary:Package}` while `Observation.Name` stays
  `${Package}` for catalogue matching. It emits only rows with
  `${db:Status-Status}=installed`; residual `config-files` records do not count
  as inventory or source availability. If apt-mark fails, installed package
  observations remain and `Explicit` is `Unavailable`; the dpkg status field
  cannot distinguish manually selected packages from dependencies. Licence,
  vendor, origin, reverse dependencies, and signature state remain unavailable
  with stated reasons.
- `flatpak.go` — `Observation.NetworkObservable` carries D1 §6.4 in the **type
  system**, not a comment. Reports each installation separately and queries
  `flatpak info --show-permissions` with its exact installation, app id, branch,
  and architecture. Flatpak resolves applicable overrides for that copy.
- `desktop.go`, `binary.go` — universal sources. `.desktop` `Version` is
  `Unavailable` because that key is the *specification* version, not the app's;
  reporting "1.5" would be a confidently wrong fact. `binary.go` excludes
  `/nix/store` and maps store paths to `nix:<base>`.

The source tests cover exact matching, unavailable capabilities, Flatpak scope
retention, and the universal scanners.

`engine.Run` carries each source's `Capabilities` into the report. Schema-1
`coverage` rows list supported facts for ready, non-empty sources;
`reduced` notes list active sources that do not support each fact. The CLI and
TUI show both views, and the privacy plugin shows the reduced source list in its
Coverage limits card. These fields do not affect grades, and older snapshots
without them remain valid.

### 6.5 Phase 3 — grading

`/home/nomadx/privacy-bit/internal/audit/` evaluates supplied observations and
source coverage without running commands:

- Matches by exact detect type and name through `catalog.Set.Match`; unknown
  observations remain `unknown`, and duplicate Flatpak scopes remain separate.
- Starts matched entries at the effective catalogue grade. A contradicted
  measured signal moves green to amber or amber to red; red stays red. Zero,
  unavailable and ordinary observed signals do not change the grade.
- Grades unmet high expectations red and unmet medium/low expectations amber.
  Ungraded expectation tokens are matched as exact package names.
- Marks a category unknown if an unmet expectation cannot be judged because a
  required detect type has no ready source. Other incomplete coverage is
  reported on the category and in the source coverage list. Category and
  overall grades use worst evidence; unknown propagates to overall.

`internal/engine` and the audit CLI connect the sources to the grader. A
contradicted measured signal can worsen an entry; zero and unavailable signals
cannot change it. Flatpak permissions are graded separately from the catalogue:
red covers host filesystem, all-device, or shared-network grants; amber covers
bounded broad access such as home files, host configuration, X11, and USB; green requires complete,
classified evidence. Failed, missing, malformed, and unclassified evidence
stays unknown. Category and overall roll-ups use worst supported evidence,
including sandbox copies.

The owner approved worst-evidence roll-up: unmet high expectations are red,
unmet medium/low expectations are amber, and an unknown category makes the
overall grade unknown. Unmatched inventory entries do not change known
categories.

### 6.6 Follow-on TUI installer

The approved installer design and implementation plan are in §3.4. The
`privacybit-install` command builds the CLI from the checkout with network
access disabled, installs it to the current user's `~/.local/bin`, and offers
no background work, the watcher, or the monthly timer. It refuses root and
requires confirmation before writing. The no-background choice disables any
existing privacybit user units. Ctrl+C cancels the active build or service
command and keeps the TUI open until it returns. Cancellation before the atomic
binary replacement preserves the current CLI; cancellation during service
setup reports that the installed CLI remains and service state may be partial.

`internal/installer/binary.go` validates ownership and file types, writes a
temporary executable in the destination directory, syncs it, and atomically
renames it into place. Service setup failure leaves the installed CLI in place
and is reported as a partial result. Full tests, build, vet, formatting,
focused race checks, and a VHS preview that quit before confirmation pass.

The README describes implemented inventory and VPN checks, notes deferred
coverage, and shows synthetic report and category screenshots. Its first-run
`go run` may resolve the installer's Go modules; the installed CLI build itself
is offline.

### 6.7 Arch package preparation

`packaging/aur/PKGBUILD` and `.SRCINFO` target `0.1.3`; PR #42 merged the change.
`makepkg --printsrcinfo | cmp - .SRCINFO` passes, and the CLI, helper, and
installer cross-build for Linux amd64 and arm64. The public AUR package remains
`privacybit 0.1.2-1` until the `v0.1.3` release. The package installs the CLI,
root helper, and user-service installer under `/usr/bin`; it does not enable
units or include the checkout-only TUI installer. GitHub release `v0.1.2` at
`7c63797` includes the Flatpak sandbox grade and package-origin context. Issue
#20 is closed. The README declares GPL-3.0-or-later, and the package installs
the verbatim `LICENSE` file at `/usr/share/licenses/privacybit/LICENSE`.

---

## 7. Commits and commit policy

### 7.1 History

```
fae52ba feat: deliver privacy audit tool
9f85b95 docs: project handover
4f6ab11 docs: handover for taking ownership of this project
1dffbac feat(detect): observation sources with honest unavailability
3e32dd7 feat(cli): list, explain and catalog validate
64ce43d feat(catalog): entry model, loader, validator and shipped table
c31fd37 docs: add the plugin and host design
8bece42 docs: add scheduler and event-watcher design (D7)
fca13f1 docs: add the multi-distribution source layer
14cf4ff docs: add the TUI design
e65fcdd docs: apply the direction rule to the helper design
8561af4 docs: add D4 root helper design
f723c26 docs: add D2 CLI design; correct D1 panel and IPC assumptions
8e67317 docs: design flatpak as a first-class detection axis
ed882f6 docs: add privacybit design and README
c3ad189 chore: initialise privacybit repository
```

### 7.2 Policy — read before committing

The repository owner **does not allow AI attribution on GitHub.** A global
commit hook enforces this and it is **deliberate. Do not modify or weaken it.**

Hooks live at `/home/nomadx/.git-hooks/`, wired via `core.hooksPath`. The
`commit-msg` hook rejects any message matching, case-insensitively: `claude`,
`anthropic`, `chatgpt`, `openai`, `copilot`, `cursor`, `cody`, `tabnine`,
`codex`, `gemini`, `bard`, `gpt-N`, `llm`, `ai assistant`, `bot`, `agent`, plus
co-author trailers and emoji.

**Known false positive: `bot` matches inside "Both"** (b-o-t-h). This has
rejected three legitimate commit messages. **The fix is to reword the message,
never to weaken the hook.** Avoid the word "both" in commit bodies.

Set the owner's GitHub identity explicitly when committing from a clean
worktree:

```console
git -c user.name=RAMA -c user.email=143774106+Nomadcxx@users.noreply.github.com commit -F <file>
```

---

## 8. How to audit the work

Specific, falsifiable claims. Each is checkable.

| # | Claim | How to check |
|---|---|---|
| 1 | **Signal states cannot silently turn unset or unavailable facts into observations or grade changes.** | `TestZeroSignalIsNotObserved`, `TestContradictedSignalIsMeasuredEvidence`, and `TestEveryCatalogueEntryAndSignalStateObeysTheGradeRule`; inspect `internal/audit/audit_test.go`. |
| 2 | **No detection source sets `Product`.** | `TestSourcesNeverSetProduct` in `internal/detect/detect_test.go`. `Product` is assigned only after catalogue matching. |
| 3 | **No duplicate detect keys.** | `TestEveryDetectKeyBelongsToOneEntry` in `…/internal/catalog/load_test.go`. A duplicate is a contradiction resolved silently into a confidently wrong grade. |
| 4 | **Every red entry offers an alternative.** | `TestRedEntriesOfferAnAlternative`. Without it, a red entry is a wall of shame rather than a to-do list. |
| 5 | **The catalogue is permissive, not strict, on `any_of`.** | `TestAnyOfToleratesUngradedPackages`. Current state: **118 tokens, 98 resolve to entries, 20 name ungraded packages (~17%)**. Deliberate — an expectation asks "does the user have something of this kind", not "did the catalogue happen to grade it". A user with Epiphany installed must not be told they have no browser. |
| 6 | **Matching is exact and unmatched apps cannot worsen known grades.** | `TestMatchUsesExactDetectTypeAndName` and `TestEvaluateMatchesExactObservationsAndKeepsFlatpakScopes`. |
| 7 | **Expectations and incomplete coverage cannot read as a pass.** | `TestUnmetExpectationsUseSeverityAndUngradedPackageNamesCanSatisfy`, `TestCoverageGapsYieldUnknownWhenTheyBlockAnUnmetExpectation`, and `TestPartialCoverageDoesNotAlterAStillJudgeableCategory`. |
| 8 | **Missing or invalid audit output stays unknown.** | `cmd/privacybit/audit_test.go`, `plugins/privacy/snapshot_test.go`, and `TestPrivacySnapshotReaderTreatsMissingPluginStateAsUnavailable`. |
| 9 | **Findings preserve grade evidence and omit unsupported actions.** | `TestBuildFindingsIncludesNonGreenEntriesAndTierThreeAlternatives`, `TestBuildFindingsUsesJudgedExpectationSeverityAndOmitsUnknowns`, and `TestBuildFindingsReportsKnownFlatpakCopiesAtBothScopesWithoutRegrading`. |
| 10 | **The host accepts only bounded, valid schema-1 state and treats old/future reports as unrated.** | `TestPrivacySnapshotReader...` and `TestPrivacyPageDowngradesStaleSnapshotToUnrated` in `internal/shell/controlcenter_test.go`. |
| 11 | **The privacy panel preserves capture rows and reports grades, VPN state, findings, and coverage.** | `TestPrivacyPageKeepsCaptureRowsAndShowsUnavailableAudit`, `TestPrivacyGradePresentationUsesWordsAndThemeTones`, and `TestPrivacyPageRendersSnapshotTotalsAndCoverageLimits`. |
| 12 | **Opening a stored report does not silently replace it with a fresh scan.** | `TestStoredSnapshotOpensWithoutScanningAndRefreshesOnRequest` in `internal/tui/model_test.go`; `Init` returns no scan command when a snapshot is present. |
| 13 | **Debian explicit-install state reflects apt marks, distinguishes architectures, and remains unavailable when apt-mark fails.** | `TestDpkgUsesAptMarkForExplicitInstallState`, `TestDpkgMatchesAptMarksByArchitectureQualifiedName`, and `TestDpkgKeepsInventoryWhenAptMarkFails` in `internal/detect/detect_test.go`. |
| 14 | **Debian inventory and availability include only installed packages.** | `TestDpkgInventoryIncludesOnlyInstalledPackages` and `TestDpkgAvailabilityTreatsConfigFilesOnlyAsEmpty` in `internal/detect/detect_test.go`. |
| 15 | **Flatpak permission evidence is resolved per installation and never leaks raw environment values.** | `TestFlatpakInventoryPreservesInstallationScope`, `TestFlatpakInventoryQueriesNamedInstallation`, `TestFlatpakSandboxBandsKeepUnknownAndUseWorstEvidence`, and `TestBuildIncludesSandboxEvidenceInOverallSnapshot`. |
| 16 | **Sandbox coverage and panel rendering stay explicit and bounded.** | `TestFlatpakSandboxWithoutInventoryCoverageIsUnknown`, `TestBuildBoundsPerCopySandboxEvidence`, `TestParseAuditOutputBoundsFlatpakSandboxEvidence`, `TestParseAuditOutputRejectsInvalidAndFutureSnapshots`, and `TestPrivacyPanelShowsSeparateFlatpakSandboxGradesAndCoverage`. |
| 17 | **Installer cancellation preserves the current CLI and reports partial service state.** | `TestCtrlCCancelsBuildAndReportsCLIState`, `TestCtrlCCancelsServiceSetupAndReportsPartialState`, `TestCtrlCAfterBuildCompletionDoesNotStartServices`, and `TestInstallBinaryCancellationPreservesTarget`. |
| 18 | **Origin status reports a recognized local name without claiming source authentication.** | `TestWriteHumanShowsOriginsAsContext` in `cmd/privacybit/audit_test.go`, `TestAppViewShowsOriginContextWithoutChangingGrade` in `internal/tui/model_test.go`, and `TestPrivacyPanelShowsOriginAsContext` in sysc-plugins `plugins/privacy/view_test.go`. |
| 19 | **Source capability gaps reach the report without changing grades.** | `TestCapabilitiesFactsUseStableNamesAndOrder`, `TestRunReportsSourceFactsAndUnsupportedCoverage`, `TestWriteHumanShowsSupportedAndReducedFacts`, `TestCoverageViewShowsSupportedAndReducedFacts`, `TestParseAuditOutputProjectsBoundedReducedSources`, and `TestPrivacyPanelShowsSourcesBehindReducedCoverage`. |
| 20 | **PATH exclusions match directory boundaries.** | `TestUserPathDirsUsePathComponentBoundaries` in `internal/detect/detect_test.go` keeps adjacent paths visible while excluding system roots and their descendants, including the Nix store. |
| 21 | **VPN checks treat every verified tunnel interface consistently.** | `TestMultipleVerifiedTunnelsAreCheckedAsASet` in `internal/network/network_test.go` covers stable client reporting and route, DNS, IPv6, and default-route checks with Tailscale and WireGuard. |

§11 records the latest gate results. Earlier phase checkpoints are historical;
use the current results when judging the tree.

---

## 9. How to audit the plan

The following are **opinions, not derivations.** They are the most likely
places the project is wrong, and each was chosen deliberately.

| # | Decision | Reasoning |
|---|---|---|
| 9.1 | **Brave is amber, not green.** | Community consensus rates it green. The reservation is the ad-partner business model. `alternatives` points at Brave Origin first. |
| 9.2 | **OnlyOffice is red despite being AGPL.** | The clearest demonstration that licence and privacy grade are independent axes: the problem is cloud-default behaviour, not licence. |
| 9.3 | **Entries are flat, not nested product/variant.** | `brave` and `brave-origin` are **two entries with two grades**, not one product with a variant flag. Grading stays a pure function of the entry. |
| 9.4 | **`amber`, not `orange`.** | A colour-named grade conflicts with the accessibility requirement — meaning must not live in the colour. The UI shows colour + distinct glyph shape + word. |
| 9.5 | **The 0–100 score is not a quality measure.** | Only meaningful against the user's own past self. The UI must say so. |
| 9.6 | **The panel performs no privileged operation in v1.** | Panel is a viewer, TUI is the operator. *"A control centre that can remove software with one click is a bad control centre."* D4's review token exists to make that impossible, not merely hard. |
| 9.7 | **The daemon is a user daemon, not root.** | A root daemon's "current user" is `/root`, breaking per-user scoping on multi-user machines. All unattended work is user-scoped. moonbit needs root at 3am only for `pacman -Rns`, which is Class C here — so the direction rule applies to scheduling too. |
| 9.8 | **VPN monitoring was cut; point-in-time VPN checks remain in scope.** | On 2026-10-20 the owner directed completion of all remaining phases. Implement D1 §7's route, DNS and IPv6 checks. Continuous monitoring and per-app network history stay cut. |

### Ideas deliberately rejected — do not re-propose these

(The table above numbers these decisions 9.1–9.8; this list is unnumbered so the
two do not collide.)

| Rejected | Why |
|---|---|
| Grade on licence string alone | A licence describes a licence, not a behaviour. Usable only as a cross-check under I2. |
| Grep binaries for "telemetry" | Matches unrelated code, doc strings and translations. |
| `tun0` present means "VPN connected" | Says nothing about routing. Produces a green light that means nothing — worse than an obviously-off VPN. |
| Detecting "suspicious ports" | Detection-only is not enforcement. |
| IP-based telemetry blocklist | Mail, login, docs and video share frontends with telemetry. This is the *default* failure, not an edge case. |
| Per-process network blocking | A project (OpenSnitch), not a feature, without a kernel module. |
| The score as a quality measure | See §9.5. |
| `install-entry` as a helper verb | Violates I4. |
| A pacman `PostInstall` hook for instant alerts | Runs as root and is permanent attack surface on every future package transaction, for a latency improvement. Polling ships first. |
| A shared moonbit/privacybit library | The genuinely-shared surface is ~900 lines and every file in it needs editing on arrival (moonbit strings in `errors.go`, the lock path in `oplock.go`, unit names in `units.go`). A module would be a merge queue, not reusable code. |
| An `nix` source that fits the package-manager model | Nix has derivations in a store, not packages. It wants its own source type and is deferred in D6 §12. |

---

## 10. Known defects and gaps

### 10.1 Work outside the implemented phases

| Item | State |
|---|---|
| v0.1.3 release preparation | Privacybit PR #42 updates the AUR metadata; sysc-plugins PR #281 bumps the plugin manifest. Both merged after their checks passed. Local builds succeeded for the privacybit CLI, helper, and installer on amd64 and arm64. Release rehearsal run `40934309464` passed the gate, vulnerability scan, tests, race checks, manifest/catalog validation, and both plugin builds. Downloaded archives contain version `0.1.3` and protocol `1.8`; their SHA-256 digests are `ec05b2d53a631992077082a9ad80329d2db8675da0697150f58dc4aeb76808e2` (amd64) and `4800b42d450e7d624cb405f69dc6d89ae3ce9d45c0be565336af94de83dd752d` (arm64). The manual run skipped release publication and catalog changes. Issues #41 and #280 track the coordinated release; wait for owner approval before tagging. |
| Laptop testbench integration | Last checked over SSH on 2026-10-29 after the handshake fix. `/home/nomadx/.local/bin/privacybit` was executable and embedded `54c9fb4`; a no-network schema-1 snapshot included origin context. The privacy plugin embedded `9b44520`, the shell embedded `ca3c988`, `sysc-shell.service` was active, and the bar placement was enabled. SSH returned `No route to host` on 2026-10-30, including this audit; current upstream revisions remain unverified on the testbench. |
| Latest SYSC integration changes | Privacybit PR #51 exposes per-source fact coverage and merged at `ef2cd37`; PR #52 refreshed the handover and merged at `ceaab67` after CI passed. `sysc-plugins` main is `9229030` after PR #286, which shows reduced-coverage sources; its `test`, `govulncheck`, and `verify` checks passed. Issue #50 is closed. `sysc-shell` main is `2f7c2a8` after PR #559, which repairs plugin bar interaction; its test, coverage, `govulncheck`, Landlock ABI, and verify checks passed. These latest upstream revisions remain unverified on the unreachable laptop testbench. |
| Live telemetry endpoint attribution | Deferred by D7 §5.2: no reviewed endpoint dataset or reliable PID attribution. Snapshots report this signal as unknown. |
| Effective Flatpak sandbox grade | Implemented in privacybit PR #16 and shown by sysc-plugins PR #259. It grades each installed copy from resolved permissions: red for host filesystem, all-device, or shared-network grants; amber for bounded broad access; green only for complete classified evidence without those grants. Missing or unclassified evidence is unknown. Privacybit `v0.1.2` includes the grade; issue #20 is closed. Plugin release `privacy-v0.1.2` includes the panel and manifest/handshake fix; catalog PR #275 merged and issue #268 is closed. |
| Package origin context | The owner resolved privacybit issue #22: show the observed source name and whether it matches a recognized official label, and never use origin to change a grade. The match uses the name only; privacybit does not authenticate the configured repository or remote. Privacybit PR #23 adds bounded report fields and CLI/TUI output; sysc-plugins PR #264 adds panel context. PRs #38 and #277 clarify that a recognized name does not authenticate the configured source; issues #37 and #276 are closed. Unmatched apps remain unrated and retain visible origin context. Published `v0.1.2` assets still predate the wording correction. |
| Metadata contradiction grading | `gradeEntry` supports test-injected contradictions for version, licence, vendor, explicit-install, reverse-dependency, and signature signals; it excludes origin. Built-in sources do not create `detect.Contradicted`, and `catalog.Entry` stores no expected metadata values. Issue #53 records the decision on curated expectations, licence-only comparison, or deferral and corrected docs. |
| dnf, zypper, apk, and xbps backends | Deferred under D6; universal sources report what they can see, with coverage caveats. |
| Nix and Snap package-manager sources | Deferred under D6 because their models do not fit the package-manager interface. |
| Licence | `LICENSE`, README, `PKGBUILD`, and `.SRCINFO` use GPL-3.0-or-later. The full licence text matches `/usr/share/licenses/spdx/GPL-3.0-or-later.txt`. The checked sysc-shell tree remains BSD-3-Clause and sysc-plugins remains MIT; neither was changed. |
| Panel scans and report actions | The plugin panel runs `privacybit audit --snapshot` as the current user and renders the bounded snapshot. The Control Centre routes to that panel; it does not scan or read the saved report. |
| sysc-plugins store entry | Release `privacy-v0.1.2` is published from tested commit `21b4944`; both architecture bundles contain manifest and handshake version `0.1.2` and have verified SHA-256 digests. Catalog PR #275 merged at `2f67de4`. A disposable sysc-shell store install resolved the live catalog and installed the amd64 bundle; issue #268 is closed. |
| Release and AUR package | GitHub release [`v0.1.2`](https://github.com/Nomadcxx/privacybit/releases/tag/v0.1.2) and AUR package [`privacybit 0.1.2-1`](https://aur.archlinux.org/packages/privacybit) remain the published versions. Tag `v0.1.2` points to `7c63797`; PR #42 prepares `0.1.3` metadata, while issues #41 and #280 track the release gate. |

### 10.2 Design corrections and open conflicts

Recorded because a future reader will trust the docs.

1. **D5 proposed a duplicate wire tone.** The old privacy worktree was based on
   sysc-shell before `plugin/v1.ToneWarning` landed. Current `origin/main`
   accepts `warning` at protocol minor 18 and maps it to `ui.ToneActivity`.
   The clean integration reuses that value; it does not add `activity` or bump
   the protocol. D1 §8.1, D3, D5, and this handover record the correction.

2. **D1 §2.1 said the plugin would import the module. It shells out.** The
   engine lives under `internal/`, and Go forbids cross-module import of
   `internal/`; a module dependency would force publishing the audit API
   publicly, creating a cross-repository compatibility commitment for a
   one-function surface. `plugins/moonbit` already shells out to its binary, so
   this is the house pattern.

3. **D1 §11.1 proposed an HTTP `serve` command. It is a Unix socket.** See D2
   for why HTTP was dropped entirely. `serve` and `panel` also belong to phase
   9 with the scheduler, not to the CLI.

4. **D6 §3.2 previously excluded `Contradicted` evidence from grades.** That
   conflicted with D1's catalogue/package metadata cross-check and D6 §11's
   explicit requirement that contradicted evidence may worsen a grade. The
   owner confirmed that metadata contradictions may worsen an entry's grade.
   Issue #22 later clarified that package origin is context only, including
   when contradicted. `gradeEntry` supports test-injected contradictions for
   version and package facts; origin does not affect app, category, or overall
   grades. Built-in sources do not produce `detect.Contradicted`, and catalogue
   entries store no expected metadata values. Issue #53 tracks the missing
   production comparison rule.

5. **D1, D2 and D5 previously did not define category or overall grade
   aggregation.** The owner chose worst-evidence roll-up. D1 §3.7 now states
   the rule and D1/D2/D5 examples agree: a red category makes the overall red;
   unknown category grades propagate to overall.

6. **D7's Class B list included machine-wide systemd changes.** Class B is
   user-scoped and the daemon runs as a user; D4 already places `set-unit`
   behind the root helper. D7 now keeps system-wide unit changes out of
   unattended work and leaves them as attended helper actions.

7. **D4's `report-root` crossed D1's current-user boundary.** Running as root
   exposes other users' processes and `/proc` entries. The verb is now deferred
   from v1; any wider audit needs a separate owner decision.

8. **D5's host-owned `Run scan` design was superseded.** The owner approved it
   on 2026-10-20, then approved D5 §11 on 2026-10-23. The current plugin owns
   the bar widget, audit panel, and read-only `Scan now` action. The Control
   Centre keeps a compact summary and routes `Open full audit` to that panel.
   The plugin protocol remains unchanged.

9. **D1 described `flatpak info --show-permissions` as metadata-only.** Review
   of Flatpak 1.18.4's implementation showed that it loads the deployed app
   context and merges applicable system/user global and per-app overrides.
   D1 and D6 now use that resolved context, while keeping raw environment
   values out of reports.

10. **Origin labels were displayed as authenticated source status.** The
    detectors only recognize local names such as `core` and `flathub`; they do
    not verify repository servers or Flatpak remote configuration. The CLI,
    TUI, and plugin now label a name match as recognized and say that the source
    is not authenticated. Origin remains context and does not affect grades.

### 10.3 Implementation defects and corrections

1. **`Load` originally panicked on a malformed shipped file.** With ten
   hand-authored files, the first panic killed the test binary and hid the other
   nine — defeating the entire point of the aggregating validator. Fixed to
   return an error.

2. **`any_of` was validated as entry ids only.** 21 validation failures. This
   produced a genuine design correction (§8 item 5), not just a data fix.

3. **Grade rendering diverged.** `explain`'s header and alternatives list once
   rendered the same grade through different paths. The console views now share
   `gradeCell`; JSON keeps the separate glyph and word fields required by D2.

4. **`Observation.Branch` and `Permissive` were set by `flatpak.go` but had
   never been added to the struct.** The build was broken for several turns and
   was reported as "compiling clean" because no build had been run. **Lesson:
   verify with a build, never with a summary of an earlier state.**

5. **`NetworkObservable` existed only in a comment.** `flatpak.go`'s doc comment
   claimed the field carried D1 §6.4 "rather than a comment on it" while the
   field did not exist. A rule that lives in a comment is a rule that gets
   forgotten the day someone refactors the caller. It is a real field now, with
   a zero value of `false` so a source that forgets to set it is conservative.

6. **Flatpak permission inspection originally ignored installation scope and
   overrides.** The detector now queries each exact app ref with its installation
   selector; Flatpak returns the deployed app context with applicable overrides
   merged. The audit stores only recognized grants and short unknown reasons,
   keeping environment values out of the report.

7. **Pacman previously dropped a matched package when `pacman -Qi` failed.** It
   now retains the package with enrichment facts marked unavailable.

8. **User catalogue overrides previously lost the shipped grade in the
   effective entry.** `CatalogueGrade` is now preserved across overrides and
   shown separately.

9. **Catalogue revisions previously ignored most effective fragment changes.**
   They now hash the effective catalogue data, not only shipped entry ids and
   grades plus a fragment count.

10. **Malformed trailing YAML was accepted after the first document.** Parsing
    now rejects malformed trailing content instead of treating it as EOF.

11. **Opening `privacybit report` unexpectedly started another scan.** The
    model's `Init` always ran its scan command, including when opened with a
    stored snapshot. The scan replaced the report before the user could review
    it, contrary to D3. `Init` now skips scanning when a snapshot is present;
    `TestStoredSnapshotOpensWithoutScanningAndRefreshesOnRequest` exercises the
    initialization hook as well as the explicit refresh path.

12. **The Control Centre scan action used an icon outside the embedded subset.**
    The full page-layout test caught `refresh` as unsupported. The button now
    uses the existing `restart_alt` Material symbol, and the full shell package
    test passes. Scan failure copy now points to CLI diagnostics without
    assuming every failure means the executable is missing.

13. **Installer validation checked a resolved unit directory but later used its
    original symlinked path.** A replaced symlink could redirect unit writes
    after validation. `validateUnitDir` now returns the canonical directory;
    install and disable operations continue through that path. The installer
    test covers canonical path resolution.

14. **The runner test checked for a live helper child immediately after
    sending `SIGKILL`.** Signal delivery is asynchronous, so load could make the
    test observe the child before it exited. The test now waits for process
    exit with a one-second deadline.

15. **Pacman origin marked every non-AUR repository official.** Repository
    names are configurable, so a custom or local source could appear official
    in reports. Issue #28 narrows the marker to recognized Arch repository
    labels and leaves unknown names visible as not marked official. The marker
    reflects pacman's local label; it does not authenticate the configured
    server. Origin remains context and does not affect grades.

16. **The installer ignored Ctrl+C during build and service setup.** Each run
    now passes a cancellable context to those operations. The TUI waits for the
    result and reports whether the CLI was installed and service setup may be
    partial. The binary installer checks cancellation immediately before
    atomic replacement, preserving the prior executable when cancellation
    arrives before that point.

17. **Origin copy implied source authentication.** Pacman and Flatpak expose
    local repository or remote names. Privacybit PR #38 and sysc-plugins PR #277
    say when a name matches a recognized label and state that privacybit does
    not authenticate the configured source. Issues #37 and #276 are closed.
    The schema and grading rules stay unchanged. Published `v0.1.2` assets
    predate this wording correction.

18. **Source capabilities were declared but dropped from reports.** D6 §3 and
    §10 require supported facts per source and visible reduced-coverage notes.
    `engine.Run` now passes each capability declaration into the schema-1
    report. The CLI and TUI list supported facts; the plugin lists sources that
    cannot provide each fact. Origin remains context only, and no grade changes.

19. **Binary inventory used string prefixes to filter PATH.** That skipped
    adjacent directories such as `/usr/bin-extra` and included exact roots such
    as `/nix/store` and `/usr/lib`. `userPathDirs` now compares cleaned path
    components; its test covers roots, descendants, and adjacent names.

20. **VPN checks selected the first tunnel from a map.** With multiple active
    tunnel interfaces, the route checks could report one tunnel as an escape.
    The audit now checks membership in the full verified interface set and
    sorts client names before reporting them.

### 10.4 Environment traps

- **`: ` inside an unquoted multi-line YAML plain scalar is a parse error** —
  YAML reads the text before it as a key. Five of these existed. Rephrase with a
  comma or em-dash rather than quoting.
- **`ui.Tone` is an iota enum.** Any new tone must be **appended, never
  inserted**, or every later value shifts.
- **`go:embed` can only reach its own package directory and below.** The
  catalogue lives at `internal/catalog/data/`, not repo-root `catalog/`.
- **`dpkg-query` is installed on this Arch machine and manages zero packages.**
  This is why availability is three-state and not a boolean on `$PATH`.
- **`/nix/store` holds 7,246 content-addressed paths** against 2,882 pacman
  packages. The binary source must exclude the store or it reports thousands of
  phantom applications.
- **Test fixtures need real tabs.** `"${Package}\\t"` in Go source is a literal
  backslash-t and will not match the tab in the actual command line.
- **Go 1.26 uses `GOTMPDIR` for `t.TempDir()`.** Keep build files in the
  required cache path, then run test binaries with a short `GOTMPDIR` through
  `go test -exec` so Unix socket fixtures stay below Linux's path limit.
- **Go 1.26.4 can fail VCS stamping in linked worktrees.** The nested build in
  `TestBuildAndInstallCLIInstallsRunnableBinaryOffline` reports
  `error obtaining VCS status: exit status 128` when the test runs from a
  linked Git worktree. The test passes in the primary checkout, and the full
  suite passes in a clean clone and GitHub CI. Run exact Go 1.26.4 tests from a
  normal checkout; keep VCS stamping enabled.
- **`go mod tidy -diff` is not currently an offline check.** The local module
  cache lacks transitive `golang.org/x/exp/maps` and `slices` test packages.
  Bubble Tea and Lipgloss are direct dependencies; do not run tidy with network
  access merely to rewrite the module files.
- **`resolvectl` is in `foreign` mode on this machine** — Tailscale MagicDNS
  owns `/etc/resolv.conf`. Any future DNS work must account for that.

---

## 11. Verification and open gaps

| Repository | Automated checks | Result |
|---|---|---|
| privacybit | `.github/workflows/ci.yml`: formatting, `go test -count=1 -p 2 ./...`, `go vet -p 2 ./...`, and `go build -p 2 ./...` | PR #51 adds per-source fact coverage; its CI passed and the maintainer merged it at `ef2cd37`. PR #52 records the merge status; its CI passed and the maintainer merged it at `ceaab67`. PR #42 updates the AUR recipe and `.SRCINFO` for `0.1.3`; its CI passed. `makepkg --printsrcinfo | cmp - .SRCINFO` and Linux amd64/arm64 builds of the CLI, helper, and installer passed. The published `v0.1.2` assets still predate PR #38's source-origin clarification. |
| sysc-plugins | PR #259 adds the Flatpak sandbox grade; PR #262 fixes contrast; PR #264 shows origin context; PR #273 prepares `privacy-v0.1.2`; PR #275 updates the catalog; PR #277 clarifies origin labels; PR #279 joins Docker workers on shutdown; PR #281 prepares `privacy-v0.1.3`; PR #283 shows first-scan progress; PR #285 fixes stale-result copy during an active refresh; PR #286 shows reduced-coverage source names. | PR #286's `test`, `govulncheck`, and `verify` checks passed; the maintainer merged it at `9229030` and closed privacybit issue #50. PR #285's checks passed; it merged at `11f0202`. PR #283's checks passed; it merged at `aa35f65`. PR #281's checks passed; it merged at `4adfa32`. Release rehearsal run `40934309464` passed its gate, vulnerability scan, tests, race checks, validation, and both architecture builds; the manual run skipped release and catalog jobs. PR #279's checks passed; it merged at `890a178` and closed issue #278. PR #277's checks passed and it merged at `3578127`. Release workflow run `40920475913` passed its gate, vulnerability scan, tests, race checks, both architecture builds, and catalog validation. PR #275's checks passed and it merged at `2f67de4`. A disposable store install verified the amd64 asset. |
| sysc-shell | PR #436 adds the original summary; PR #437 routes it to the plugin panel; PR #559 repairs plugin bar drop and press states. | All three PRs are merged. PR #559's test, coverage, `govulncheck`, Landlock ABI, and verify checks passed. Upstream main is `2f7c2a8`; the testbench service still embeds shell revision `ca3c988`, and the privacy plugin last checked there embeds revision `9b44520`. |

The live review's evidence is also recorded in sysc-shell issue `sysc-1444`.
The sysc-plugins and sysc-shell primary checkouts contain independent changes,
including `.beads/issues.jsonl`. Keep those changes out of privacybit work and
use clean worktrees for cross-repository changes. The deployed screenshot
remains in the local cache and is not part of the plugin's synthetic catalog
screenshot.

The environment guard blocked `go test -race ./...` because the repository-wide
race run has repeatedly hard-locked this zram-only machine. Race checks were
run sequentially for `plugin/v1`, `internal/plugin`, and `internal/shell`; all
three passed.

Privacybit Go commands use `GOTMPDIR=/home/nomadx/.cache/privacybit-gotmp`; the
sysc-shell worktree uses `GOTMPDIR=/home/nomadx/.cache/sysc-shell-gotmp`. The
repositories use `GOMAXPROCS=4` and `-p 2` for repository-wide work. Go 1.26
also uses `GOTMPDIR` for `t.TempDir()`. Long test paths exceed Linux's Unix
socket limit, so this machine runs test processes with
`-exec='env GOTMPDIR=/var/tmp/pbtt'` while the Go command keeps build files in
the cache directory. `/tmp` hit its quota during a nested installer build; use
the root-backed `/var/tmp` path for the test process.

---

## 12. Privacy of the tool itself

An audit tool that phones home is worse than no tool, because it would not be
noticed.

- **No network calls. At all.** No update check, no crash reporting, no
  analytics, no version ping.
- **Audit output is gitignored.** It fingerprints exactly what someone runs.
  The `.gitignore` ships with the rationale inline so nobody adds it back.
- **The default audit does not read outside the invoking user's scope.** No
  other users' processes, keys, or browser profile contents beyond the specific
  preference keys a grade depends on. D4's `report-root` is deferred; do not
  add a wider audit without a separate owner decision.
- **Secret hygiene in the sudo path** follows moonbit exactly: the password
  never enters the helper's address space as a retained value, and
  `defer clear(password)` runs before any error return.

This is to be **enforced by test, not by convention.**

---

## 13. Useful commands

Run from `/home/nomadx/privacy-bit` unless stated otherwise.

```console
# Build, vet, format check, test — all four must be clean.
go build ./... && go vet ./... && gofmt -l ./internal ./cmd && go test ./...

# Phase 1 CLI.
go run ./cmd/privacybit list --category browser --grade red
go run ./cmd/privacybit explain brave
go run ./cmd/privacybit catalog validate
go run ./cmd/privacybit catalog path

# Ground truth probes on this machine.
pacman -Qq | wc -l                              # 2882
flatpak list --app --columns=application | wc -l # 5
ls /usr/share/applications/*.desktop | wc -l     # 412
dpkg-query -W bash                              # installed, manages nothing
ls /nix/store | wc -l                           # 7246
```

---

## 14. If you must change the plan

You are expected to change it. The plan contains opinions, and some are wrong.

**Do not weaken an invariant to make something work.** If I2 blocks a feature,
the feature is wrong, not the invariant. If something seems to require a false
green, the honest answer is `unknown`.

When you change something material:

1. Amend the relevant document in `/home/nomadx/privacy-bit/docs/plans/` and say
   what changed and why.
2. Update §10.2 if you found another place the docs were wrong.
3. Update §8 if the change affects what the grader must enforce.
4. Update §3 if the file manifest changes.
5. Record it in the commit message — which, per §7.2, must contain no AI
   attribution, and must not contain the word "both".
