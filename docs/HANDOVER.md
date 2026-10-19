# privacybit — project handover

**Written:** 2026-10-20
**Reader:** an engineer or agent with **no prior context on this conversation.**
**Purpose:** audit what has been built, audit the plan, change the plan if it is
wrong, then continue development to completion.

This document is self-contained. If you have read nothing else, you can work
from it plus the seven design documents in [`docs/plans/`](plans/).

---

## 0. Read this first — three things that are easy to get wrong

**1. Under-grading is the one unacceptable error.** Every other failure in this
project is recoverable: a missed app, a stale catalogue entry, a wrong install
command. Telling someone their privacy setup is fine when it is not corrodes
trust in every other claim the tool makes, and the user has no way to detect it.
Most of this codebase's structure exists to make a false green difficult to
write.

**2. Nothing shown may read as a pass.** An empty result, an empty card, a
silent failure, and a genuinely clean result must be visually distinct. This
has already produced three real bugs (see §8.2) and it is the single easiest
thing for a new contributor to reintroduce.

**3. The design documents are authoritative for *why*, the code is
authoritative for *what*.** Where they disagree, that is a bug in one of them —
find it and fix it explicitly. Three disagreements are already known (§8.2).

---

## 1. What this project is

**privacybit** is a privacy audit for Linux desktops. It inspects what is
installed, what is running, and what is actually going over the network, then
reports a graded breakdown by category, with a stated reason for every grade
and a concrete action for every finding.

It was built to fill a real gap in the sysc-shell Control Centre privacy panel,
which today only answers *"is something capturing my microphone/camera/screen
right now"*. It does not answer *"is my setup reasonable"*. These are different
questions and conflating them would make both worse.

### 1.1 Vocabulary — this distinction is load-bearing

- **Privacy** — who can access your information.
- **Security** — protection from unauthorised access or theft.
- **Anonymity** — your actions cannot be traced back to you.

They are **not interdependent**. Google systems are secure and unlikely to be
hacked, yet Google still has access to your personal data. Signal offers high
privacy but not anonymity, because registration requires a phone number.

privacybit grades **privacy**. Where something is really a security or anonymity
concern, it is either excluded or labelled. The UI copy must never imply that
"green" means "private" in the strong sense. Green means: *no meaningful
unwanted disclosure was identified by the checks we actually ran* — which is
why §3's second invariant matters so much.

### 1.2 The three failure modes the design exists to avoid

| Failure | Why it is fatal |
|---|---|
| **Under-grading** | Undetectable by the user, and destroys trust in every other claim. |
| **Wall-of-shame** | A report opening with fourteen red items and no wins gets deleted. Lead with the summary and the achievable fixes. |
| **False positives** | One wrong accusation destroys trust faster than ten misses. When uncertain, say `unknown`. |

---

## 2. Where the work is

| Location | What is there |
|---|---|
| `/home/nomadx/privacy-bit` | **This project.** Go module `github.com/Nomadcxx/privacybit`. |
| `/home/nomadx/privacy-bit/docs/plans/` | All seven design documents. Read D1 first. |
| `/home/nomadx/sysc-shell/.worktrees/sysc-1307-privacy-20261019` | The live Control Centre privacy panel. Branch `feat/sysc-1307-privacy-20261019`. **Not yet modified by privacybit work.** |
| `/home/nomadx/sysc-plugins` | Plugin repository. No privacy plugin yet. |
| `/home/nomadx/moonbit` | The sibling project this was modelled on. 100% owned by the same author, so reuse is unrestricted. **It is 132 commits behind its own `origin/main`** — read `origin/main`, not the working tree. |

---

## 3. The five invariants

These are the load-bearing rules. **If you change the plan, do not weaken an
invariant to make something work.** Break one and you have a tool that lies
confidently.

### I1 — Derived signals are monotone

A measured fact about the live system may only push a grade **toward more risk**,
or raise a flag. It may **never improve** a grade.

There is no runtime path from `red` to `amber`. Reduction happens only when the
user changes the system and the audit re-runs from scratch. An audit that
reports "better than expected" on the strength of a heuristic is producing an
unearned claim.

*Why:* the heuristic will be wrong sometimes, and the error mode of a heuristic
that can improve a grade is a grade that silently improves.

### I2 — An unavailable signal may not move a grade in **either** direction

Three rules:
1. Only a signal in state `Observed` may inform a grade.
2. The catalogue grade is always the floor.
3. Insufficient coverage to judge means `unknown`, never `green`.

This is the general form of D1 §6.4's flatpak rule and D6 §3.2. It is the most
important thing in the detection layer.

*Concrete bug it prevents:* `dpkg` has no licence metadata. A Debian user with
a package the catalogue calls FOSS cannot be cross-checked, so "no contradiction
found" would read as clean — **a green browser because the backend had nothing
to say.**

*Implementation note:* `SignalState` deliberately has a `StateZero` that is
**not equal to** `StateObserved`, so a `Signal` nobody populated cannot read as
a passing observation. `TestZeroSignalIsNotObserved` pins this.

### I3 — Absence is a finding, not a pass

"No password manager installed" grades `red`. It does not grade empty. Every
category declares an `expectations` list; an unmet expectation is a finding in
its own right.

### I4 — The privileged path removes and configures. It never introduces.

No install verb. No disk-encryption verb. Installing introduces new code that
will later execute — potentially as root, through a systemd unit the package
ships — and that is the one direction a review step cannot bound. Removal and
unit toggling operate on what is already present and *can* be bounded ahead of
time.

moonbit draws the same line and has for its whole life: its privileged verbs
are scan, clean, orphan removal, docker prune and schedule, with no `pacman -S`
anywhere in the codebase. **Adding one would make privacybit less safe than the
project it is modelled on.**

*Corollary:* the helper has no `install-entry`. Installing an alternative is a
**tier 3** action — the tool prints the exact command and the user runs it.

### I5 — Nothing shown may read as a pass

An empty result, an empty card, a silent failure, and a clean result must be
visually distinct. Enforced in the CLI, in the plugin's "unavailable" payload,
and in the validator's requirement that every entry can be explained.

---

## 4. Architecture

```
                    privacybit (this repo)
                    ├── CLI      privacybit audit|explain|list|watch|tui|serve
                    ├── TUI      privacybit tui                    [D3, not built]
                    ├── daemon   event watcher, user-level         [D7, not built]
                    ├── helper   privacybit-helper (root)         [D4, not built]
                    └── library  importable Go packages
                              │
                              ▼
                      sysc-plugins/plugins/privacy  [D5, not built]
                              │  state.set "control_center"
                              ▼
                     sysc-shell Control Centre → privacy panel
```

### 4.1 Package map — WHAT EXISTS vs WHAT DOES NOT

```
internal/catalog/     EXISTS   entry model, loader, validator, 10 YAML categories
internal/detect/      EXISTS   observation sources, honest unavailability
cmd/privacybit/       EXISTS   list, explain, catalog validate  (NO TESTS YET)
─────────────────────────────────────────────────────────────────────────────
internal/audit/       MISSING  the grader — the heart of the project
internal/report/      MISSING  shared grade rendering + snapshot types
internal/render/      MISSING  per-distro command rendering (D6 §8)
internal/tui/         MISSING  Bubble Tea UI                     [D3]
internal/runner/      MISSING  sudo machinery                    [D3]
internal/helper/      MISSING  root helper verbs                 [D4]
internal/daemon/      MISSING  event watcher + systemd units     [D7]
internal/network/     MISSING  VPN checks, telemetry connections [D1 §7]
plugins/privacy/      MISSING  sysc-shell plugin                 [D5]
```

**Grade rendering is currently duplicated** between `cmd/privacybit/list.go` and
`cmd/privacybit/explain.go` — `gradeCell` is used in one place and
`entry.Grade.Glyph()` in another, and that duplication already caused one bug
(two renderings of one grade on one screen). Extracting `internal/report/` is a
prerequisite for D3 and D5, not a nicety.

---

## 5. What is done

### 5.1 Design — complete, seven documents

| Doc | File | Covers |
|---|---|---|
| **D1** | `plans/2026-10-20-privacybit-design.md` | Engine: rating model, identity model, categories, actions, tiers, privacy-of-tool rules |
| **D2** | `plans/2026-10-20-privacybit-cli-design.md` | CLI surface, NDJSON contract, exit codes, config |
| **D3** | `plans/2026-10-20-privacybit-tui-design.md` | Bubble Tea model, 13 screens, keybindings, navigation stack |
| **D4** | `plans/2026-10-20-privacybit-helper-design.md` | Root helper, closed verb set, review tokens, sudo dance |
| **D5** | `plans/2026-10-20-privacybit-plugin-design.md` | sysc-shell plugin + host wiring, the `activity` tone |
| **D6** | `plans/2026-10-20-privacybit-sources-design.md` | Multi-distribution source layer, the I2 invariant |
| **D7** | `plans/2026-10-20-privacybit-daemon-design.md` | Daemon, event watcher, Class A/B/C, systemd units |

### 5.2 Implementation — phases 1 and 2 of D1's §15

**Phase 1 — catalogue. Complete and green.**

`internal/catalog/`:
- `catalog.go` — `Grade` with `Risk()` ordering **green 0 < amber 1 < red 2 <
  unknown 3**. Unknown sorts *above* red deliberately: an unrated item has not
  been shown safe, and ranking it as better would be an under-grade. Three
  rendering channels: `Word()` (Good/Review/Risky/Unrated), `Glyph()`
  (●/▲/■/?), `ASCII()` (o/^/x/?). `DetectType` closed set (`package|binary|
  flatpak|desktop`) with `Confidence()` 2 for package/flatpak (primary evidence,
  named by the distribution) and 1 for binary/desktop (corroboration — a binary
  named `signal` is not necessarily Signal Desktop). Substring matching is
  deliberately unsupported.
- `load.go` — `//go:embed data/*.yaml`; `dec.KnownFields(true)`; malformed
  shipped file **returns an error rather than panicking** so ten files' errors
  aggregate; `revision()` produces `cat-<12hex>+<N fragments>` so a stored
  snapshot traces to the table that produced it.
- `validate.go` — aggregating `Problems.Err()` sorted by path; no detect key
  claimed by two entries; **every red entry must offer an alternative**.
- Ten data files: addons, browsers, chat, email, media, metadata, office,
  passwords, pdf, vpn.

`cmd/privacybit/`: `main.go`, `util.go`, `list.go`, `explain.go`,
`catalog_cmd.go`. Cobra. `--user-catalog` persistent flag. Per-distro
`installCommand`.

**Phase 2 — detection. Complete and green.**

`internal/detect/`:
- `detect.go` — `Signal[T]{State, Value, Why}`, `Availability{Ready, Reason,
  Empty}`, `Capabilities`, `Origin{Source, Official}`, `Facts`, `Kind`,
  `Observation`, `Source` interface.
- `runner.go` — `Runner` interface so tests inject recorded output.
  `MapRunner` returns `unfixturedError` for an unfixtured command — **never
  empty output**, so an unfixtured call cannot masquerade as "a source with
  nothing to report".
- `pacman.go` — enriches **only catalogue matches**, keeping the slow tier
  affordable (`-Qq` ~100ms over 2,882 packages; `-Qi` orders slower). Missing
  `ValidatedBy` → `Unavailable`, not `false`.
- `dpkg.go` — the reference case for I2. Reports only `Version` and `Explicit`;
  everything else is `Unavailable` with a stated reason.
- `flatpak.go` — `Observation.NetworkObservable` carries D1 §6.4 in the **type
  system**. Reports system and user installations of the same id as **two
  observations**, not deduplicated.
- `desktop.go`, `binary.go` — universal sources. Version `Unavailable` because
  the `.desktop` `Version` key is the *specification* version.
- `detect_test.go` — 9 tests.

**Verified:** `go build ./...`, `go vet ./...`, `gofmt -l` all clean.
`go test ./...` → `ok internal/catalog`, `ok internal/detect`.

### 5.3 Commit history

```
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

---

## 6. Commit policy — read before committing

The repository owner **does not allow AI attribution in GitHub.** A global
commit hook enforces this and it is **deliberate. Do not modify or weaken it.**

Hooks live at `/home/nomadx/.git-hooks/`, wired via `core.hooksPath`. The
`commit-msg` hook rejects any message matching, case-insensitively:
`claude`, `anthropic`, `chatgpt`, `openai`, `copilot`, `cursor`, `cody`,
`tabnine`, `codex`, `gemini`, `bard`, `gpt-N`, `llm`, `ai assistant`, `bot`,
`agent`, plus co-author trailers and emoji.

**Known false positive: `bot` matches inside "Both"** (b-o-t-h). This has
rejected two legitimate commit messages. **The fix is to reword the message,
never to weaken the hook.** Avoid the word "both" in commit bodies.

Use `git -c user.name=nomadx -c user.email=nomadx@users.noreply.github.com`
when committing, since the user identity may not be configured globally.

---

## 7. How to audit the work

Specific, falsifiable claims. Each is checkable.

| # | Claim | How to check |
|---|---|---|
| 1 | **I2 is enforced, not merely documented.** | `internal/detect/detect_test.go:TestZeroSignalIsNotObserved`. Then grep for any place a `Signal` is constructed as a bare struct literal — that would bypass the state. Confirm `StateZero != StateObserved`. |
| 2 | **No source sets `Product`.** | `TestSourcesNeverSetProduct`. `Product` is the catalogue entry id; a source assigning it would be converting a filename into a grade. |
| 3 | **No duplicate detect keys.** | `internal/catalog:TestEveryDetectKeyBelongsToOneEntry`. A duplicate is a contradiction resolved silently into a confidently wrong grade. |
| 4 | **Every red entry offers an alternative.** | `TestRedEntriesOfferAnAlternative`. Without it, a red entry is a wall of shame rather than a to-do list. |
| 5 | **The catalogue is permissive, not strict, on `any_of`.** | `TestAnyOfToleratesUngradedPackages`. Current state: **118 tokens, 98 resolve to entries, 20 name ungraded packages (~17%)**. This is deliberate — an expectation asks "does the user have something of this kind", not "did the catalogue happen to grade it". A user with Epiphany installed must not be told they have no browser. |
| 6 | **Nothing renders silence as a pass.** | `cmd/privacybit` has **no test file**. Read §8.2 — three real instances of this bug already occurred. |

**Also verify the build, do not trust this document:** `go build ./... &&
go vet ./... && gofmt -l ./internal ./cmd && go test ./...`. All four must be
clean. *A claim in a handover document that the code compiles has already been
wrong once in this project — see §8.2, item 4.*

---

## 8. Known defects and gaps

### 8.1 Missing work, in the order it should be done

| Priority | Item | Why |
|---|---|---|
| 1 | **`internal/audit/` — the grader. Does not exist.** | This is the heart of the project. Without it nothing is graded; the tool is a catalogue browser. |
| 2 | **`cmd/privacybit` has no test file.** | Three real bugs were found by *hand-running* the CLI and all three belong in tests. |
| 3 | **`internal/report/` — shared grade rendering.** | Currently duplicated between `list.go` and `explain.go`. D3 and D5 both need one implementation. |
| 4 | **No `audit` command.** | D2 specifies the full NDJSON contract; only `list`, `explain` and `catalog validate` exist. |
| 5 | **dnf/zypper/apk/xbps/nix backends.** | Not a blocker — D6 deliberately made backends additive behind an interface that already exists. An unsupported distro still audits via the universal sources, with honest coverage. |
| 6 | **`internal/network/` — VPN checks.** | Point-in-time only (see §9.8). |
| 7 | **D3 TUI, D4 helper, D7 daemon, D5 plugin.** | In that order. |

### 8.2 Three places the design documents were wrong

Recorded because a future reader will trust the docs.

1. **D1 §8.1 claimed plugin/v1 has no amber/warning tone. It does.**
   `internal/ui/tree.go` in the sysc-shell worktree has defined `ToneActivity`
   for some time, documented as *"a reading past its activity threshold but
   short of critical"*, painting the theme's **Tertiary** role. Already live at
   `controlcenter_monitor.go:184`, `popout_process.go:204`, and
   `monitorscale_test.go:18-22` — the latter asserting exactly the
   normal→activity→error progression privacybit grades want. **Only the wire is
   missing**: two edits in `plugin/v1/node.go` plus one conversion case.
   D1 §8.1 and §12 have been amended.

2. **D1 §2.1 said the plugin would import the module. It shells out.** The
   engine lives under `internal/`, and Go forbids cross-module import of
   `internal/` — a module dependency would force publishing the audit API
   publicly, creating a cross-repository compatibility commitment for a
   one-function surface. `plugins/moonbit` already shells out to its binary, so
   this is the house pattern.

3. **D1 §11.1 proposed an HTTP `serve` command. It is a Unix socket.** See D2
   for why HTTP was dropped entirely.

### 8.3 Three bugs in the implementation worth knowing about

1. **`Load` originally panicked on a malformed shipped file.** With ten
   hand-authored files, the first panic killed the test binary and hid the other
   nine — defeating the entire point of the aggregating validator. Fixed to
   return an error.

2. **`any_of` was validated as entry ids only.** 21 validation failures. This
   produced a genuine design correction (§7 item 5), not just a data fix.

3. **Grade rendering diverged.** `explain`'s header used `Grade.Glyph()`
   directly while the list below it used `gradeCell` — two renderings of one
   grade on one screen.

4. **`Observation.Branch` and `Permissive` were set by `flatpak.go` but had
   never been added to the struct.** The build was broken for several turns and
   was reported as "compiling clean" because no build had been run. **Lesson:
   verify with a build, never with a summary of an earlier state.**

### 8.4 Environment traps

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
- **Shell test fixtures need real tabs.** `"${Package}\\t"` in Go source is a
  literal backslash-t and will not match the tab in the actual command line.
- **moonbit's working tree is 132 commits behind `origin/main`.**

---

## 9. How to audit the plan

The following are **opinions, not derivations.** They are the most likely places
the project is wrong, and each was chosen deliberately.

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

### 9.1 Ideas deliberately rejected — do not re-propose these

| Rejected | Why |
|---|---|
| Grade on licence string alone | A licence describes a licence, not a behaviour. Usable only as a cross-check (I2). |
| Grep binaries for "telemetry" | Matches unrelated code, doc strings and translations. |
| `tun0` present means "VPN connected" | Says nothing about routing. Produces a green light that means nothing — worse than an obviously-off VPN. |
| Detecting `suspicious ports` | Detection-only is not enforcement. |
| IP-based telemetry blocklist | Mail, login, docs and video share frontends with telemetry. This is the *default* failure, not an edge case. |
| Per-process network blocking | A project (OpenSnitch), not a feature, without a kernel module. |
| score as a quality measure | See §9.5. |
| `install-entry` as a helper verb | Violates I4. |

---

## 10. Remaining phases, in order

D1 §15 defines eleven phases. Phases 1–2 are done. The order of phases 2 and 3
was **deliberately changed** so that detection precedes grading: the three-state
invariant must exist before the first grade is computed. Retrofitting it means
auditing every signal already emitted.

| Phase | Deliverable | Depends on |
|---|---|---|
| 3 | **Grading** — match observations to catalogue entries, compute category grades | needs `internal/audit/` |
| 4 | **Findings and actions**, tiers 0–3 | phase 3 |
| 5 | **Network and VPN checks** | see §9.8 — **ask first** |
| 6 | **CLI completion** — `audit`, `--json`, exit codes | phases 3–5 |
| 7 | **TUI** (D3) | `internal/report/` extracted |
| 8 | **Root helper** (D4) | phase 4 actions |
| 9 | **Daemon and timer** (D7) | phase 4 |
| 10 | **sysc-shell plugin** (D5) | `audit` binary exists |
| 11 | **Host wiring** — `activity` tone, `ccPrivacy` second card | phase 10 |

**Two items can be built today, before the engine exists:**
- The `activity` tone wire in `plugin/v1` — independent of privacybit entirely.
- `ccPrivacySnapshotFor` plus the second card, tested against a hand-written
  JSON fixture. This is the only part of the project whose user-visible outcome
  is testable before the grader exists.

---

## 11. Privacy of the tool itself

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
  never enters the helper's address space as a retained value.

This is to be **enforced by test, not by convention.**

---

## 12. Useful commands

```console
go build ./... && go vet ./... && gofmt -l ./internal ./cmd && go test ./...

go run ./cmd/privacybit list --category browser --grade red
go run ./cmd/privacybit explain brave
go run ./cmd/privacybit catalog validate
go run ./cmd/privacybit catalog path
```

Ground truth probes on this machine:

```console
pacman -Qq | wc -l                              # 2882
flatpak list --app --columns=application        # 5
ls /usr/share/applications/*.desktop | wc -l     # 412
dpkg-query -W bash                              # installed, manages nothing
ls /nix/store | wc -l                           # 7246
```

---

## 13. If you must change the plan

You are expected to change it. The plan contains opinions, and some are wrong.

**Do not weaken an invariant to make something work.** If I2 blocks a feature,
the feature is wrong, not the invariant. If something seems to require a
false green, the honest answer is `unknown`.

When you change something material:
1. Amend the relevant document in `docs/plans/` and say what changed and why.
2. Update §8.2 if you found another place the docs were wrong.
3. Update §7 if the change affects what the grader must enforce.
4. Record it in the commit message — which, per §6, must contain no AI
   attribution.