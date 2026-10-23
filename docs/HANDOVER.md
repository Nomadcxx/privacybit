# privacybit — project handover

**Written:** 2026-10-23
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
| `/home/nomadx/privacy-bit` | **This project.** Branch `main`; local branch `feat/privacybit-final-20261022` retained. Release tag `v0.1.0` points to `fae52ba`. | Delivery phases 0–9 and 12, installer follow-on, Arch package preparation, and the GPL-3.0-or-later README/ANSI wordmark update are published. The unrelated untracked `.commandcode/` data is untouched. |
| `/home/nomadx/privacy-bit/docs/plans/` | Seven design documents and implementation plans. | D1–D7 record the approved design. Phase 3, 4, 8, and 9 plans remain as audit history. |
| `/home/nomadx/sysc-plugins/.worktrees/privacybit-plugin-20261022` | Plugin branch `feat/privacybit-plugin`, commit `a502f38`. | Phase 10 publisher and synthetic catalog screenshot are committed locally. `catalog.json` has no privacy entry yet, so the store cannot install it. |
| `/home/nomadx/sysc-shell/.worktrees/sysc-1307-privacy-final-20261022` | Shell branch `feat/sysc-1307-privacy-final-20261022`, deployed revision `9f05fcc8`. | Phases 11–12 are committed; live Niri verification passed on 2026-10-22. The branch also preserves deployed launcher fix `c2d274e3`. |
| `/home/nomadx/.config/sysc-shell/plugins/org.sysc.privacy` | User plugin link to the sysc-plugins worktree. | Enabled for the local integration. The running plugin published a current schema-1 snapshot. |
| `/home/nomadx/sysc-shell` | Primary shell checkout. | Contains independent work and Beads state; use its Beads database only, and do not clean or deploy this checkout. |
| `/home/nomadx/sysc-plugins/cmd/sysc-plugin-calendar/main.go` | The precedent to copy for the plugin. | Reference only. |
| `/home/nomadx/sysc-plugins/plugins/calendar/manifest.json` | Manifest shape to copy. | Reference only. |
| `/home/nomadx/moonbit` | Sibling project this was modelled on. 100% owned by the same author, so reuse is unrestricted. | **132 commits behind its own `origin/main`** — read `origin/main`, not the working tree. |
| `/tmp/moonbit-header` | Detached worktree at moonbit `origin/main` (`5948bd0`). | May not survive a reboot; recreate if missing. |

### 2.2 Privacybit integration files in the sysc-shell worktree

Phase 11 added the audit card using sysc-shell's existing warning-to-activity
tone mapping. Phase 12 adds the host-owned scan action. All paths are relative to
`/home/nomadx/sysc-shell/.worktrees/sysc-1307-privacy-final-20261022`.

| File | Role |
|---|---|
| `internal/shell/controlcenter_pages.go` | Reads bounded schema-1 plugin state and renders the audit card below capture activity. |
| `internal/shell/controlcenter_test.go` | Covers snapshot validation, card rendering, currentness, and grade tones. |
| `internal/shell/controlcenter_privacy_scan.go` | Runs `privacybit scan` outside `Registry.mu`, reads the private saved report on page open and after a scan, and renders scan status, grade, and timestamp. |
| `internal/shell/controlcenter_privacy_scan_test.go` | Covers report bounds and freshness, timer-created report loading, duplicate-scan prevention, and missing-binary status. |

`plugin/v1.ToneWarning` already maps to `ui.ToneActivity` in current
sysc-shell `main`; this integration reuses it. Protocol v1 also sends
`input.event` to plugin-owned views. The privacy plugin remains state-only; the
native Control Centre owns the `Run scan` action and reads privacybit's private
XDG report.

---

## 3. Implementation map

The early source and catalogue tables below come from the phase 1–4 audit and
do not list the later phase packages. Use `rg --files` for a complete current
inventory. The phase map in §11 lists the added subsystems.

### 3.1 Original core source inventory

| Lines | Absolute path | Purpose |
|---|---|---|
| 289 | `/home/nomadx/privacy-bit/internal/catalog/catalog.go` | `Grade`, `Risk()`/`Worse()`, `Word()`/`Glyph()`/`ASCII()`, exact detect rules and lookup, `Entry`, `Expectation`, `Category`, `Set` |
| 484 | `/home/nomadx/privacy-bit/internal/audit/audit.go` | Inventory evaluator, measured-signal grading, findings, tier 0/3 actions, expectation and coverage roll-up |
| 287 | `/home/nomadx/privacy-bit/internal/catalog/load.go` | `//go:embed data/*.yaml`, `Load()`, strict one-document parsing, fragment merge, effective-catalogue revision, index |
| 254 | `/home/nomadx/privacy-bit/internal/catalog/validate.go` | `Problem`/`Problems.Err()`, `(s *Set) validate()`, `ids()`, `categoryNames()` |
| 238 | `/home/nomadx/privacy-bit/internal/detect/detect.go` | `Signal[T]` (including measured-state helpers), `Availability`, `Capabilities`, `Origin`, `Facts`, scoped `Observation`, `Source` interface |
| 103 | `/home/nomadx/privacy-bit/internal/detect/runner.go` | `Runner` interface, `ExecRunner`, `MapRunner` for tests, `commandTimeout = 20s` |
| 199 | `/home/nomadx/privacy-bit/internal/detect/pacman.go` | `Pacman` — enriches catalogue matches only; retains a package if enrichment fails |
| 133 | `/home/nomadx/privacy-bit/internal/detect/dpkg.go` | `Dpkg` — the I2 reference case |
| 126 | `/home/nomadx/privacy-bit/internal/detect/flatpak.go` | `Flatpak` — preserves duplicate scopes; permission inspection still reads declared permissions and drops scope |
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
| 331 | `/home/nomadx/privacy-bit/internal/detect/detect_test.go` | 14 — see §8 |
| 399 | `/home/nomadx/privacy-bit/internal/audit/audit_test.go` | Grading, coverage, findings, actions and Flatpak scope reporting |
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
| 871 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-design.md` (**D1** — engine) |
| 333 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-cli-design.md` (**D2** — CLI) |
| 283 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-tui-design.md` (**D3** — TUI) |
| 427 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-helper-design.md` (**D4** — root helper) |
| 429 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-plugin-design.md` (**D5** — plugin + host) |
| 391 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-sources-design.md` (**D6** — multi-distro sources) |
| 262 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-daemon-design.md` (**D7** — daemon) |
| 73 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-phase-3-audit-grader.md` (Phase 3 implementation plan) |
| 46 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-phase-4-findings-actions-design.md` (Phase 4 approved design) |
| 67 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-phase-4-findings-actions-plan.md` (Phase 4 implementation plan) |
| — | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-phase-8-helper-plan.md` (Phase 8 implementation plan) |
| — | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-phase-9-daemon-plan.md` (Phase 9 implementation plan) |
| 71 | `/home/nomadx/privacy-bit/README.md` |
| — | `/home/nomadx/privacy-bit/docs/screenshots/privacybit-overview.png` (synthetic report view) |
| — | `/home/nomadx/privacy-bit/docs/screenshots/privacybit-categories.png` (synthetic category view) |
| — | `/home/nomadx/sysc-plugins/.worktrees/privacybit-plugin-20261022/plugins/privacy/screenshot.png` (synthetic native Privacy page) |
| — | `/home/nomadx/sysc-plugins/.worktrees/privacybit-plugin-20261022/plugins/privacy/thumbnail.webp` (generated catalog thumbnail) |
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
| `/home/nomadx/privacy-bit/packaging/aur/` | `PKGBUILD` and generated `.SRCINFO` for the first Arch package |

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
                    privacybit  (/home/nomadx/privacy-bit)
                    ├── CLI      privacybit audit|explain|list|watch|tui
                    ├── TUI      privacybit tui                    [D3]
                    ├── daemon   event watcher, user-level         [D7]
                    ├── helper   privacybit-helper (root)         [D4]
                    └── internal packages
                              │
                              ▼  shells out to the binary
                      sysc-plugins/plugins/privacy  [D5]
                              │  state.set "control_center"
                              ▼
                     sysc-shell Control Centre → privacy panel
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
sysc-plugins/plugins/privacy   five-minute snapshot publisher
sysc-shell/internal/shell     bounded reader and Control Centre card
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
| **D5** | sysc-shell publisher + host wiring, reusing the existing warning tone |
| **D6** | Multi-distribution source layer, invariant I2 |
| **D7** | Daemon, event watcher, Class A/B/C, systemd units |

### 6.2 Delivery phases

Implementation for phases 0–12 is committed across the local feature branches
listed in §2. Automated verification appears in §11. The clean sysc-shell
branch was deployed through `scripts/deploy`; the live Niri check passed. The
privacy plugin is linked and enabled locally, but is not in the published
plugin catalog.

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
| 11 | sysc-shell Control Centre audit card using the existing warning tone | Complete |
| 12 | Host-owned read-only scan action, shared report lock, and GPL-3.0-or-later licensing | Complete |

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
- `dpkg.go` — the I2 reference case. Reports only `Version` and `Explicit`;
  everything else `Unavailable` with a stated reason. `Capabilities()` returning
  mostly `false` is what makes the unavailable states structural rather than
  something to remember.
- `flatpak.go` — `Observation.NetworkObservable` carries D1 §6.4 in the **type
  system**, not a comment. Reports system and user installations of the same id
  as **two observations**, not deduplicated: two copies at two privilege levels
  is a real finding, and the user copy shadows the system one.
- `desktop.go`, `binary.go` — universal sources. `.desktop` `Version` is
  `Unavailable` because that key is the *specification* version, not the app's;
  reporting "1.5" would be a confidently wrong fact. `binary.go` excludes
  `/nix/store` and maps store paths to `nix:<base>`.

The source tests cover exact matching, unavailable capabilities, Flatpak scope
retention, and the universal scanners.

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
cannot change it. Effective Flatpak sandbox posture remains unknown until the
detector reads effective, scope-specific permissions.

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
existing privacybit user units.

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

`packaging/aur/PKGBUILD` and `.SRCINFO` target version `0.1.0`. The package
builds and installs the CLI, root helper, and user-service installer under
`/usr/bin`; it does not enable units or include the checkout-only TUI installer.
`makepkg --printsrcinfo` passes, and `makepkg` built a local package from the
current source with the source URL redirected to a local archive. The public
recipe fetches the `v0.1.0` tag. The GitHub release page shows `v0.1.0`, and the
AUR package page lists `privacybit 0.1.0-1`. The README declares
GPL-3.0-or-later, and the package installs the verbatim `LICENSE` file at
`/usr/share/licenses/privacybit/LICENSE`.

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

The user identity may not be configured globally, so commit with:

```console
cd /home/nomadx/privacy-bit
git -c user.name=nomadx -c user.email=nomadx@users.noreply.github.com commit -F <file>
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
| Live Niri verification of the Control Centre page | Passed on 2026-10-22 after deploying sysc-shell revision `9f05fcc8`. The panel rendered on `DP-3`, the plugin published a current snapshot, and the capture rows remained visible. The live screenshot is local-only at `/home/nomadx/.cache/privacybit-live-20261022.png` because it includes real machine inventory. |
| Live telemetry endpoint attribution | Deferred by D7 §5.2: no reviewed endpoint dataset or reliable PID attribution. Snapshots report this signal as unknown. |
| Effective Flatpak permissions by installation scope | Deferred. Current detector reads declared permissions, so the grader leaves sandbox posture unmeasured. |
| dnf, zypper, apk, and xbps backends | Deferred under D6; universal sources report what they can see, with coverage caveats. |
| Nix and Snap package-manager sources | Deferred under D6 because their models do not fit the package-manager interface. |
| Licence | `LICENSE`, README, `PKGBUILD`, and `.SRCINFO` use GPL-3.0-or-later. The full licence text matches `/usr/share/licenses/spdx/GPL-3.0-or-later.txt`. The checked sysc-shell tree remains BSD-3-Clause and sysc-plugins remains MIT; neither was changed. |
| Panel scans and report actions | Implemented as the approved host-owned action in Phase 12. It runs the current user's `privacybit scan` and reads the bounded saved report; it does not invoke plugin actions or privileged operations. |
| sysc-plugins store entry | Publisher and synthetic screenshot are committed locally, linked, enabled, and live-verified. `catalog.json` has no `org.sysc.privacy` entry or release assets, so users cannot install it from the store yet. |
| Release and AUR package | GitHub release [`v0.1.0`](https://github.com/Nomadcxx/privacybit/releases/tag/v0.1.0) and AUR package [`privacybit 0.1.0-1`](https://aur.archlinux.org/packages/privacybit) are published. Tag `v0.1.0` points to `fae52ba`, one commit on top of `origin/main`; the release commit is not merged into `main`. |

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
   owner confirmed that a measured contradiction may worsen an entry's grade.
   D6 §3.2 and invariant I2 now state this; unavailable and unset signals still
   cannot affect grades.

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

8. **D5's original viewer-only host boundary was revised.** Protocol v1 has
   `input.event` for plugin-owned views, but the privacy plugin is state-only
   and the native Control Centre does not route actions to it. The user
   approved a host-owned read-only `Run scan` action instead: it runs
   `privacybit scan` as the current user and reads the private bounded report.
   The plugin protocol remains unchanged; D5 §6.5 records the revised design.

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

6. **Flatpak permission inspection does not meet D1 §6.4.** `PermissionsFor`
   calls `flatpak info --show-permissions`, which reports declared metadata,
   and stores the result by application id without installation scope. It does
   not inspect scope-specific overrides. Do not grade sandbox posture from this
   field until effective-permission semantics are implemented and tested.

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
- **`go mod tidy -diff` is not currently an offline check.** The local module
  cache lacks transitive `golang.org/x/exp/maps` and `slices` test packages.
  Bubble Tea and Lipgloss are direct dependencies; do not run tidy with network
  access merely to rewrite the module files.
- **`resolvectl` is in `foreign` mode on this machine** — Tailscale MagicDNS
  owns `/etc/resolv.conf`. Any future DNS work must account for that.
- **moonbit's working tree is 132 commits behind `origin/main`.**

---

## 11. Verification and remaining gate

| Repository | Automated checks | Result |
|---|---|---|
| privacybit | Formatting, `git diff --check`, `go test -count=1 -p 2 ./...`, `go vet -p 2 ./...`, and `go build -p 2 ./...`; packaging checks | Go checks pass on 2026-10-23, including the installer canonical-path regression test. GPL text comparison, generated `.SRCINFO` comparison, and an isolated-HOME TUI build/install passed on 2026-10-22. |
| sysc-plugins | Privacy tests and race tests, focused vet, plugin build, `make validate catalog-validate thumbnails`, and gofmt check | Pass on 2026-10-22. The privacy screenshot and generated thumbnail match; `catalog.json` still has no privacy release entry. |
| sysc-shell worktree | Full formatting check, `go build -p 2 ./...`, `go vet -p 2 ./...`, and normal tests for `plugin/v1`, `internal/plugin`, and all of `internal/shell`; dependency diff | Pass after merging the deployed launcher fix. `go.mod` and `go.sum` are unchanged. The live Privacy page displayed the capture card and a current audit snapshot without horizontal clipping. |

The live review's evidence is also recorded in sysc-shell issue `sysc-1444`.
The primary checkout's Beads export has independent changes; inspect it before
exporting any tracker update. The deployed screenshot remains in the local
cache and is not part of the plugin's synthetic catalog screenshot.

The environment guard blocked `go test -race ./...` because the repository-wide
race run has repeatedly hard-locked this zram-only machine. Race checks were
run sequentially for `plugin/v1`, `internal/plugin`, and `internal/shell`; all
three passed.

Privacybit Go commands use `GOTMPDIR=/home/nomadx/.cache/privacybit-gotmp`; the
sysc-shell worktree uses `GOTMPDIR=/home/nomadx/.cache/sysc-shell-gotmp`. The
repositories use `GOMAXPROCS=4` and `-p 2` for repository-wide work. Go 1.26 also uses
`GOTMPDIR` for `t.TempDir()`; the long build paths exceed Linux's Unix socket
limit in shell tests. Test commands therefore use
`-exec='env GOTMPDIR=/tmp/pbt'` for test processes while the Go tool keeps build
files in the cache directory.

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
