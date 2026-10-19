# privacybit — project handover

**Written:** 2026-10-20
**This file's absolute path:** `/home/nomadx/privacy-bit/docs/HANDOVER.md`
**Repository:** `https://github.com/Nomadcxx/privacybit` (branch `main`)
**Module:** `github.com/Nomadcxx/privacybit` (Go 1.26.4)

**Reader:** an engineer or agent with **no prior context on this project.**
**Purpose:** audit what has been built, audit the plan, change the plan if it is
wrong, then continue development to completion.

This document is self-contained. If you read nothing else, you can work from
this file plus the seven design documents in `/home/nomadx/privacy-bit/docs/plans/`.

---

## 0. How to use this document

Three different jobs, three reading orders:

| If you want to… | Read |
|---|---|
| **Continue development** | §2 (paths) → §3 (file manifest) → §4 (invariants) → §11 (phases) → then the relevant design doc |
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
find it and fix it explicitly. Three disagreements are already known (§10.2).

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
| `/home/nomadx/privacy-bit` | **This project.** | 2,545 lines of Go + 2,658 lines of catalogue YAML. Build and tests green. |
| `/home/nomadx/privacy-bit/docs/plans/` | Seven design documents. | Complete. Read D1 first. |
| `/home/nomadx/sysc-shell/.worktrees/sysc-1307-privacy-20261019` | The live Control Centre privacy panel. Branch `feat/sysc-1307-privacy-20261019`. | **Not yet modified by privacybit work.** Target for phase 11. |
| `/home/nomadx/sysc-shell/.worktrees/sysc-1307-privacy-20261019/plugin/v1/` | The plugin protocol the plugin speaks. | Target for the `activity` tone wire (§11). |
| `/home/nomadx/sysc-plugins` | Plugin repository (`github.com/Nomadcxx/sysc-plugins`). | No privacy plugin yet. Target for phase 10. |
| `/home/nomadx/sysc-plugins/cmd/sysc-plugin-calendar/main.go` | The precedent to copy for the plugin. | Reference only. |
| `/home/nomadx/sysc-plugins/plugins/calendar/manifest.json` | Manifest shape to copy. | Reference only. |
| `/home/nomadx/moonbit` | Sibling project this was modelled on. 100% owned by the same author, so reuse is unrestricted. | **132 commits behind its own `origin/main`** — read `origin/main`, not the working tree. |
| `/tmp/moonbit-header` | Detached worktree at moonbit `origin/main` (`5948bd0`). | May not survive a reboot; recreate if missing. |

### 2.2 Exact files to be edited in the sysc-shell worktree

These are the only sysc-shell files privacybit work will touch. All paths are
relative to `/home/nomadx/sysc-shell/.worktrees/sysc-1307-privacy-20261019`.

| File | Lines | What changes | When |
|---|---|---|---|
| `plugin/v1/node.go` | 170–179, 400 | Add `ToneActivity Tone = "activity"` and its `knownTones` entry | Phase 11, or **now** — see §11 |
| `internal/shell/controlcenter_pages.go` | 391–406 | `ccPrivacy` gains a second card inside a scroll | Phase 11 |
| `internal/shell/controlcenter_pages.go` | 539–576 | `ccCalendarSnapshotFor` is the pattern to mirror | Reference |
| `internal/shell/registry.go` | ~1992, ~2796 | Action routing for new `privacybit:*` verbs | Phase 11 |

`internal/ui/tree.go` lines 578–589 define `ToneActivity` **already** — see
§10.2 item 1. No theme change is required; Tertiary exists on every palette.

---

## 3. File manifest

Every file that exists today, with its absolute path and what it is for.

### 3.1 Go source — `/home/nomadx/privacy-bit/` (2,545 lines, no tests)

| Lines | Absolute path | Purpose |
|---|---|---|
| 275 | `/home/nomadx/privacy-bit/internal/catalog/catalog.go` | `Grade`, `Risk()`, `Word()`/`Glyph()`/`ASCII()`, `DetectType` + `Confidence()`, `Install`, `Entry`, `Expectation`, `Category`, `Set` |
| 279 | `/home/nomadx/privacy-bit/internal/catalog/load.go` | `//go:embed data/*.yaml`, `Load()`, `mergeFragments()`, `revision()`, `index()`, `detectKey()` |
| 250 | `/home/nomadx/privacy-bit/internal/catalog/validate.go` | `Problem`/`Problems.Err()`, `(s *Set) validate()`, `ids()`, `categoryNames()` |
| 227 | `/home/nomadx/privacy-bit/internal/detect/detect.go` | `Signal[T]`, `SignalState`, `Availability`, `Capabilities`, `Origin`, `Facts`, `Kind`, `Observation`, `Source` interface |
| 103 | `/home/nomadx/privacy-bit/internal/detect/runner.go` | `Runner` interface, `ExecRunner`, `MapRunner` for tests, `commandTimeout = 20s` |
| 190 | `/home/nomadx/privacy-bit/internal/detect/pacman.go` | `Pacman` — richest backend; enriches catalogue matches only |
| 133 | `/home/nomadx/privacy-bit/internal/detect/dpkg.go` | `Dpkg` — the I2 reference case |
| 127 | `/home/nomadx/privacy-bit/internal/detect/flatpak.go` | `Flatpak` — `NetworkObservable`, duplicate scopes, `PermissionsFor()` |
| 185 | `/home/nomadx/privacy-bit/internal/detect/desktop.go` | `Desktop` — universal `.desktop` scan |
| 210 | `/home/nomadx/privacy-bit/internal/detect/binary.go` | `Binary` — universal PATH/opt scan, nix-store exclusion |
| 107 | `/home/nomadx/privacy-bit/cmd/privacybit/main.go` | Cobra root, `loadCatalogue()`, package doc |
| 46 | `/home/nomadx/privacy-bit/cmd/privacybit/util.go` | `useColour`, `isTerminal()`, `userFragmentPath()` |
| 142 | `/home/nomadx/privacy-bit/cmd/privacybit/list.go` | `list` + `gradeCell()` |
| 215 | `/home/nomadx/privacy-bit/cmd/privacybit/explain.go` | `explain` + `installCommand()` per distro |
| 56 | `/home/nomadx/privacy-bit/cmd/privacybit/catalog_cmd.go` | `catalog validate`, `catalog path` |

### 3.2 Go tests (377 lines)

| Lines | Absolute path | Tests |
|---|---|---|
| 120 | `/home/nomadx/privacy-bit/internal/catalog/load_test.go` | 4 — see §8 |
| 257 | `/home/nomadx/privacy-bit/internal/detect/detect_test.go` | 9 — see §8 |

**`cmd/privacybit` has no test file.** This is the most actionable gap; see §10.1 item 2.

### 3.3 Catalogue data — `/home/nomadx/privacy-bit/internal/catalog/data/` (2,658 lines)

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
| 817 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-design.md` (**D1** — engine) |
| 332 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-cli-design.md` (**D2** — CLI) |
| 362 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-tui-design.md` (**D3** — TUI) |
| 442 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-helper-design.md` (**D4** — root helper) |
| 407 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-plugin-design.md` (**D5** — plugin + host) |
| 387 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-sources-design.md` (**D6** — multi-distro sources) |
| 256 | `/home/nomadx/privacy-bit/docs/plans/2026-10-20-privacybit-daemon-design.md` (**D7** — daemon) |
| 48 | `/home/nomadx/privacy-bit/README.md` |

**Reading order if you are new:** D1 §1–§5 (rating model, identity model,
catalogue), then D6 §3 (the invariant), then D2 (how the CLI surfaces it).
The rest as you reach it.

### 3.5 Other files

| Absolute path | Purpose |
|---|---|
| `/home/nomadx/privacy-bit/go.mod` | Module + deps: `cobra`, `yaml.v3`, `pflag`, `mousetrap` |
| `/home/nomadx/privacy-bit/.gitignore` | Excludes audit output — it fingerprints what a user runs |

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

### I2 — An unavailable signal may not move a grade in **either** direction

Three rules:
1. Only a signal in state `Observed` may inform a grade.
2. The catalogue grade is always the floor.
3. Insufficient coverage to judge means `unknown`, never `green`.

This is the general form of D1 §6.4's flatpak rule and D6 §3.2. It is the most
important rule in the detection layer.

*Concrete bug it prevents:* `dpkg` has no licence metadata. A Debian user with
a package the catalogue calls FOSS cannot be cross-checked, so "no contradiction
found" would read as clean — **a green browser because the backend had nothing
to say.**

*Implementation:* `SignalState` deliberately has a `StateZero` that is **not
equal to** `StateObserved`, so a `Signal` nobody populated cannot read as a
passing observation. Pinned by `TestZeroSignalIsNotObserved`.

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
                    ├── TUI      privacybit tui                    [D3, not built]
                    ├── daemon   event watcher, user-level         [D7, not built]
                    ├── helper   privacybit-helper (root)         [D4, not built]
                    └── library  importable Go packages
                              │
                              ▼  shells out to the binary
                      sysc-plugins/plugins/privacy  [D5, not built]
                              │  state.set "control_center"
                              ▼
                     sysc-shell Control Centre → privacy panel
```

**The plugin shells out rather than importing the module.** The engine lives
under `internal/`, and Go forbids cross-module import of `internal/`; a module
dependency would force publishing the audit API publicly. `plugins/moonbit`
already shells out to its binary, so this is the house pattern. (D1 §2.1 says
otherwise and is wrong — §10.2 item 2.)

### 5.1 Package map — EXISTS vs MISSING

```
internal/catalog/     EXISTS   entry model, loader, validator, 10 YAML categories
internal/detect/      EXISTS   observation sources, honest unavailability
cmd/privacybit/       EXISTS   list, explain, catalog validate  (NO TESTS YET)
─────────────────────────────────────────────────────────────────────────────
internal/audit/       MISSING  the grader — the heart of the project
internal/report/      MISSING  shared grade rendering + snapshot types
internal/render/      MISSING  per-distro command rendering (currently in explain.go)
internal/tui/         MISSING  Bubble Tea UI                     [D3]
internal/runner/      MISSING  sudo machinery                    [D3]
internal/helper/      MISSING  root helper verbs                 [D4]
internal/daemon/      MISSING  event watcher + systemd units     [D7]
internal/network/     MISSING  VPN checks, telemetry connections [D1 §7]
plugins/privacy/      MISSING  sysc-shell plugin                 [D5]
```

**Grade rendering is currently duplicated** between
`/home/nomadx/privacy-bit/cmd/privacybit/list.go` (`gradeCell`) and
`explain.go` (a direct `Grade.Glyph()` call), and that duplication already
caused one bug: two renderings of one grade on one screen. Extracting
`internal/report/` is a prerequisite for D3 and D5, not a nicety.

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
| **D5** | sysc-shell plugin + host wiring, the `activity` tone |
| **D6** | Multi-distribution source layer, invariant I2 |
| **D7** | Daemon, event watcher, Class A/B/C, systemd units |

### 6.2 Phase 1 — catalogue. Complete and green.

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

### 6.3 Phase 2 — detection. Complete and green.

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

**Verified:** `go build ./...`, `go vet ./...`, `gofmt -l ./internal ./cmd` all
clean. `go test ./...` → `ok internal/catalog`, `ok internal/detect`.

---

## 7. Commits and commit policy

### 7.1 History

```
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
| 1 | **I2 is enforced, not merely documented.** | `TestZeroSignalIsNotObserved` in `/home/nomadx/privacy-bit/internal/detect/detect_test.go`. Then grep for any `Signal` built as a bare struct literal — that bypasses the state. Confirm `StateZero != StateObserved`. |
| 2 | **No source sets `Product`.** | `TestSourcesNeverSetProduct`. `Product` is the catalogue entry id; a source assigning it would be converting a filename into a grade. |
| 3 | **No duplicate detect keys.** | `TestEveryDetectKeyBelongsToOneEntry` in `…/internal/catalog/load_test.go`. A duplicate is a contradiction resolved silently into a confidently wrong grade. |
| 4 | **Every red entry offers an alternative.** | `TestRedEntriesOfferAnAlternative`. Without it, a red entry is a wall of shame rather than a to-do list. |
| 5 | **The catalogue is permissive, not strict, on `any_of`.** | `TestAnyOfToleratesUngradedPackages`. Current state: **118 tokens, 98 resolve to entries, 20 name ungraded packages (~17%)**. Deliberate — an expectation asks "does the user have something of this kind", not "did the catalogue happen to grade it". A user with Epiphany installed must not be told they have no browser. |
| 6 | **Nothing renders silence as a pass.** | **Cannot be checked — `/home/nomadx/privacy-bit/cmd/privacybit/` has no test file.** Three real instances of this bug already occurred (§10.3). This is the weakest claim in the handover. |

Run from `/home/nomadx/privacy-bit`:

```console
go build ./... && go vet ./... && gofmt -l ./internal ./cmd && go test ./...
```

All four must be clean. **Verify with a build, do not trust this document** —
see §10.3 item 4 for a claim in a previous handover that was wrong because
nobody ran a build.

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
| 9.8 | **⚠️ VPN monitoring was cut; the point-in-time VPN checks were NOT.** | **This reading has never been explicitly confirmed with the user.** They cut *monitoring* (continuous watching) and separately cut per-app network history ("that should be managed at the firewall level"). D1 §7's point-in-time checks were their original request. **Whoever builds this must ask before assuming.** |

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
| An `nix` source that fits the package-manager model | Nix has derivations in a store, not packages. It wants its own source type and is deferred (§10.1 item 5). |

---

## 10. Known defects and gaps

### 10.1 Missing work, in priority order

| Priority | Item | Why |
|---|---|---|
| 1 | **`internal/audit/` — the grader. Does not exist.** | The heart of the project. Without it nothing is graded; the tool is a catalogue browser. |
| 2 | **`cmd/privacybit` has no test file.** | Three real bugs were found by *hand-running* the CLI and all three belong in tests. |
| 3 | **`internal/report/` — shared grade rendering.** | Duplicated between `list.go` and `explain.go`. D3 and D5 both need one implementation. |
| 4 | **No `audit` command.** | D2 specifies the full NDJSON contract; only `list`, `explain` and `catalog validate` exist. |
| 5 | **dnf / zypper / apk / xbps / nix backends.** | Not a blocker — D6 deliberately made backends additive behind an interface that already exists. An unsupported distro still audits via the universal sources, with honest coverage. |
| 6 | **`internal/network/` — VPN checks.** | Point-in-time only. **Ask first — see §9.8.** |
| 7 | **D3 TUI, D4 helper, D7 daemon, D5 plugin.** | In that order. |
| 8 | **`internal/render/` — per-distro command rendering.** | Currently a function in `explain.go`; it will also be needed by the tier-3 "print the command" action in D4. |

### 10.2 Three places the design documents were wrong

Recorded because a future reader will trust the docs.

1. **D1 §8.1 claimed plugin/v1 has no amber/warning tone. It does.**
   `/home/nomadx/sysc-shell/.worktrees/sysc-1307-privacy-20261019/internal/ui/tree.go`
   lines 578–589 have defined `ToneActivity` for some time, documented as *"a
   reading past its activity threshold but short of critical"*, painting the
   theme's **Tertiary** role. Already live at `controlcenter_monitor.go:184`,
   `popout_process.go:204`, and `monitorscale_test.go:18-22` — the latter
   asserting exactly the normal→activity→error progression privacybit grades
   want. **Only the wire is missing**: two edits in `plugin/v1/node.go` plus one
   conversion case. D1 §8.1 and §12 have been amended.

2. **D1 §2.1 said the plugin would import the module. It shells out.** The
   engine lives under `internal/`, and Go forbids cross-module import of
   `internal/`; a module dependency would force publishing the audit API
   publicly, creating a cross-repository compatibility commitment for a
   one-function surface. `plugins/moonbit` already shells out to its binary, so
   this is the house pattern.

3. **D1 §11.1 proposed an HTTP `serve` command. It is a Unix socket.** See D2
   for why HTTP was dropped entirely. `serve` and `panel` also belong to phase
   9 with the scheduler, not to the CLI.

### 10.3 Five bugs in the implementation worth knowing about

1. **`Load` originally panicked on a malformed shipped file.** With ten
   hand-authored files, the first panic killed the test binary and hid the other
   nine — defeating the entire point of the aggregating validator. Fixed to
   return an error.

2. **`any_of` was validated as entry ids only.** 21 validation failures. This
   produced a genuine design correction (§8 item 5), not just a data fix.

3. **Grade rendering diverged.** `explain`'s header used `Grade.Glyph()`
   directly while the list below it used `gradeCell` — two renderings of one
   grade on one screen.

4. **`Observation.Branch` and `Permissive` were set by `flatpak.go` but had
   never been added to the struct.** The build was broken for several turns and
   was reported as "compiling clean" because no build had been run. **Lesson:
   verify with a build, never with a summary of an earlier state.**

5. **`NetworkObservable` existed only in a comment.** `flatpak.go`'s doc comment
   claimed the field carried D1 §6.4 "rather than a comment on it" while the
   field did not exist. A rule that lives in a comment is a rule that gets
   forgotten the day someone refactors the caller. It is a real field now, with
   a zero value of `false` so a source that forgets to set it is conservative.

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
- **`resolvectl` is in `foreign` mode on this machine** — Tailscale MagicDNS
  owns `/etc/resolv.conf`. Any future DNS work must account for that.
- **moonbit's working tree is 132 commits behind `origin/main`.**

---

## 11. Remaining phases, in order

D1 §15 defines eleven phases. Phases 1–2 are done. The order of phases 2 and 3
was **deliberately changed** so detection precedes grading: invariant I2 must
exist before the first grade is computed. Retrofitting it means auditing every
signal already emitted.

| Phase | Deliverable | Depends on |
|---|---|---|
| 3 | **Grading** — match observations to catalogue entries, compute category grades | needs `internal/audit/` |
| 4 | **Findings and actions**, tiers 0–3 | phase 3 |
| 5 | **Network and VPN checks** | **Ask first — §9.8** |
| 6 | **CLI completion** — `audit`, `--json`, exit codes | phases 3–5 |
| 7 | **TUI** (D3) | `internal/report/` extracted |
| 8 | **Root helper** (D4) | phase 4 actions |
| 9 | **Daemon and timer** (D7) | phase 4 |
| 10 | **sysc-shell plugin** (D5) | `audit` binary exists |
| 11 | **Host wiring** — `activity` tone, `ccPrivacy` second card | phase 10 |

**Two items can be built today, before the engine exists:**

1. The **`activity` tone wire** in
   `/home/nomadx/sysc-shell/.worktrees/sysc-1307-privacy-20261019/plugin/v1/node.go`
   — completely independent of privacybit. Append to `knownTones`, add one
   conversion case. Remember: append, never insert (§10.4).
2. **`ccPrivacySnapshotFor` plus the second card** in
   `…/internal/shell/controlcenter_pages.go`, tested against a hand-written
   JSON fixture. This is the only part of the project whose user-visible outcome
   is testable before the grader exists. Mirror `ccCalendarSnapshotFor`
   (lines 539–576) exactly: absent plugin, absent key, or malformed JSON all
   return `nil` and the page renders anyway.

---

## 12. Privacy of the tool itself

An audit tool that phones home is worse than no tool, because it would not be
noticed.

- **No network calls. At all.** No update check, no crash reporting, no
  analytics, no version ping.
- **Audit output is gitignored.** It fingerprints exactly what someone runs.
  The `.gitignore` ships with the rationale inline so nobody adds it back.
- **No reading outside the user's own scope.** No other users' processes, no
  keys, and no browser profile *contents* beyond the specific preference keys a
  grade depends on.
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