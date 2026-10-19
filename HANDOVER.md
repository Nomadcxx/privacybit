# privacybit — project handover

**Written:** 2026-10-20
**For:** an engineer or agent taking ownership of this project with no prior
context. Everything you need to audit the work, audit the plan, change the plan,
and continue development is here or in `docs/plans/`.

**Status at handover:** seven design documents complete and committed. Phases 1
and 2 implemented, building and passing tests. No grading engine, no TUI, no
helper, no plugin yet.

---

## 1. What this project is

privacybit is a privacy audit for Linux desktops. It reports what is installed,
what is running, and what is going over the network, graded by category, with a
stated reason for every grade and a concrete action for every finding.

It exists because users make privacy decisions from vibes — "I have Brave so
I'm fine", "I saw a VPN icon so I'm covered" — and both are frequently wrong.

It will eventually be surfaced in the sysc-shell Control Centre privacy panel.
That is a downstream consumer, not the owner. The engine is standalone and has
no dependency on sysc-shell, which is deliberate and load-bearing: it can be run
from a terminal, from a TUI, from a monthly systemd timer, or from a panel.

### 1.1 The three failure modes it must not have

These are the constraints everything else serves. If you change something, check
you have not violated one of these.

1. **Under-grading.** Telling someone their setup is fine when it is not is the
   one unacceptable error. Every other failure is recoverable.
2. **Wall-of-shame.** A report opening with fourteen red items and no wins gets
   deleted. Lead with the summary and the achievable fixes.
3. **False positives.** One wrong accusation destroys trust faster than ten
   misses. When uncertain, say unknown.

## 2. Where the work is

| Path | What |
|---|---|
| `/home/nomadx/privacy-bit` | This repo. `github.com/Nomadcxx/privacybit`. |
| `docs/plans/` | Seven design documents. Read them before changing anything. |
| `/home/nomadx/sysc-shell/.worktrees/sysc-1307-privacy-20261019` | The live Control Centre privacy panel. D5 targets this. |
| `/home/nomadx/sysc-plugins` | Plugin repo. D5 targets this. |
| `/home/nomadx/moonbit` | Sibling project by the same author. Model for TUI, CLI, root helper, daemon. |

### 2.1 Design documents

| Doc | Covers |
|---|---|
| `2026-10-20-privacybit-design.md` (D1) | Rating model, identity model, catalogue, detection summary, action tiers, test strategy, phases |
| `2026-10-20-privacybit-cli-design.md` (D2) | Command tree, flags, NDJSON contract, exit codes, config |
| `2026-10-20-privacybit-tui-design.md` (D3) | Bubble Tea model, navigation stack, screens, keybindings |
| `2026-10-20-privacybit-helper-design.md` (D4) | **The security core.** Closed verb set, sudo invocation, review tokens |
| `2026-10-20-privacybit-plugin-design.md` (D5) | sysc-plugins plugin + sysc-shell host wiring |
| `2026-10-20-privacybit-sources-design.md` (D6) | Detection abstraction, per-distro backends, the availability invariant |
| `2026-10-20-privacybit-daemon-design.md` (D7) | Event watcher, monthly timer, Class-B auto-fix |

D1 and D6 are the load-bearing ones for what comes next. D4 is the one to read
before touching anything privileged.

### 2.2 Commit history

`c3ad189` init · `ed882f6` D1+README · `8e67317` flatpak as first-class axis ·
`f723c26` D2 · `8561af4` D4 · `e65fcdd` direction rule · `fca13f1` D6 ·
`8bece42` D7 · `c31fd37` D5 · `64ce43d` catalogue · `3e32dd7` CLI · `1dffbac`
detection sources.

### 2.3 Commit policy — do not change this

The repository owner does not permit AI attribution on GitHub. Global git hooks
live in `~/.git-hooks/` and reject any commit message matching:

```
claude|anthropic|chatgpt|openai|copilot|cursor|cody|tabnine|codex|gemini|bard|gpt-[0-9]|llm|ai assistant|bot|agent
```

plus co-author trailers and emoji. **This is deliberate.** If a commit is
rejected, rewrite the message. Never weaken the hook.

**Known false positive: `bot` matches inside the word "Both".** This has bitten
twice. Reword to "the two", "each", etc.

## 3. The invariants

Five rules. Everything in the codebase exists to serve one of them. When you
add code, ask which one it serves; if the answer is "none", it probably belongs
somewhere else.

### I1 — Derived signals are monotone

A measured fact may only push a grade toward *more* risk, or raise a flag. It
may never improve a grade. An audit that reports "better than expected" on the
strength of a heuristic is producing an unearned claim.

Corollary: there is **no runtime path from `red` to `amber`**. Reduction happens
only when the user changes the system and the audit re-runs from scratch.

### I2 — An unavailable signal may not move a grade in either direction

Not better, not worse. Only an `Observed` signal may inform a grade. Insufficient
coverage to judge means `unknown`, never `green`.

This is the single most important invariant. Its concrete form: on `dpkg` there
is no licence metadata, so a package the catalogue calls FOSS cannot be
cross-checked — and "no contradiction found" must not read as clean. A Debian
user must never see a green browser because the backend had nothing to say.

Implemented in `internal/detect/detect.go`: `Signal[T]` carries
`State ∈ {Zero, Observed, Unavailable, Contradicted}`, and **`StateZero` is
deliberately not equal to `StateObserved`** so a signal nobody populated cannot
be mistaken for one that was checked and passed.

### I3 — Absence is a finding

"No password manager installed" is `red`, not an empty category. Every category
declares `expectations`, and an unmet expectation is a finding in its own right.

Related, and weaker: `any_of` expectation tokens may name a graded entry *or* a
bare package name. The table may be over-broad about what counts, never
under-broad. (98 of 118 tokens currently resolve to entries; 20 deliberately
name ungraded packages.)

### I4 — The privileged path removes and configures. It never introduces.

No install verb exists. Installing puts new code on a machine that will later
execute — possibly as root through a unit the package ships — and no review step
can bound that future crossing. Removal and unit toggling act on what is already
present and can be bounded ahead of time.

moonbit draws the same line and has drawn it for years: its privileged verbs are
scan, clean, orphan removal, docker prune and schedule. There is no `pacman -S`
anywhere in its codebase. Adding one here would make privacybit *less* safe than
the project it is modelled on.

Corollary: `privacybit-helper` has **no disk-encryption verb at all**. The owner
was explicit — anything that impacts encryption is off the table. It is also a
security property rather than a privacy one, so D1 §1.1 already excludes it.

### I5 — Nothing shown may read as a pass

An empty category, a truncated list, a failed source, a missing binary — none of
these may look like "you are fine". Concretely:

- Bounded lists carry `installed_truncated` / `installed_total`.
- A missing binary publishes `{"available": false, ...}`, not an empty snapshot.
- Unknown `--category` errors rather than printing nothing.
- An audit that saw nothing reports `unknown`, never good.

## 4. Architecture

```
privacybit (this repo)
├── cmd/privacybit/          CLI entry point
├── cmd/privacybit-helper/   root helper — NOT YET WRITTEN, see D4
├── internal/catalog/        the authored table: entries, grades, detection rules
├── internal/detect/         sources: pacman, dpkg, flatpak, desktop, binary
├── internal/report/         NOT YET WRITTEN — shared grade rendering (D2 §8)
├── internal/audit/          NOT YET WRITTEN — the grader (D1)
├── internal/runner/         NOT YET WRITTEN — sudo machinery (D3 §9)
├── internal/tui/            NOT YET WRITTEN
└── docs/plans/              seven design documents
```

Dependency direction is strictly downward: `cmd` → `audit` → `detect`/`catalog`.
`detect` does not import `catalog` in production code; instead `Pacman` is
constructed with a `Matches func(string) bool` callback so the layer stays
separable and testable.

## 5. What is done

### Phase 1 — catalogue (complete)

`internal/catalog/`:

- `catalog.go` — `Grade` with `Risk()` ordering **green 0 < amber 1 < red 2 <
  unknown 3**. Unknown sorts *above* red deliberately: an unrated item has not
  been shown safe, and ranking it as better would be the under-grade. Also
  `Word()`/`Glyph()`/`ASCII()` (three-channel rendering), `DetectType` with
  `Confidence()` 2 for package/flatpak and 1 for binary/desktop, `Install` keyed
  by `id_like`.
- `load.go` — `//go:embed data/*.yaml`. **`dec.KnownFields(true)`** so a typo
  becomes a load error rather than a rule that silently never matches. A
  malformed *shipped* file returns an error, not a panic — with ten files, a
  panic on the first hides the other nine.
- `validate.go` — aggregates every problem rather than failing on the first.
  Enforces: unique ids, valid grades, no detect key claimed by two entries,
  every red entry offers an alternative, `any_of` non-empty.
- `data/*.yaml` — ten categories: addons, browsers, chat, email, media,
  metadata, office, passwords, pdf, vpn.

`cmd/privacybit/` — `list`, `explain`, `catalog validate|path`, with per-distro
install command rendering (D6 §8's three outcomes: command / honest "not
packaged for your distribution" / "no install route known").

### Phase 2 — detection (complete)

`internal/detect/`: `detect.go` (types + the invariant), `runner.go` (executable
interface plus `MapRunner` for tests), `pacman.go`, `dpkg.go`, `flatpak.go`,
`desktop.go`, `binary.go`, `detect_test.go` (9 tests).

## 6. What remains

Ordered. D1 §15 is the authority; the ordering was changed during design and the
current sequence is below.

1. **Phase 3 — grading.** The matcher that turns `Observation`s into catalogue
   entries, and computes per-category and overall grades. This is the first thing
   that produces a real audit. **I2 must be enforced here**, in the grader, not
   left to callers.
2. **Phase 4 — findings and actions**, tiers 0–3.
3. **Phase 5 — network and VPN checks** (D1 §7). The point-in-time checks —
   default route through tunnel, DNS containment, IPv6 escape. Note the daemon's
   *monitoring* of VPN leaks was cut; these point-in-time checks were the user's
   original request and are still in scope.
4. **Phase 6 — `audit` command**, `--json`, exit codes (D2).
5. **Phase 7 — event watcher daemon** (D7).
6. **Phase 8 — root helper** (D4).
7. **Phase 9 — monthly timer + Class-B auto-fix + TUI report screen** (D7, D3).
8. **Phase 10 — sysc-plugins plugin + sysc-shell host wiring** (D5).

## 7. Known defects and gaps

Read this before assuming the current state is sound.

### 7.1 Things that are wrong or incomplete right now

- **`cmd/privacybit` has no test file.** Three defects were found by hand-running
  the CLI — an unknown `--category` printing silence, all errors being
  swallowed, a redundant grade string. Those are exactly the cases that belong
  in a test. Write this first.
- **There is no grader.** Phases 1 and 2 produce observations and catalogue
  entries that nothing joins together yet. Running `privacybit` today shows the
  table, not the machine.
- **`internal/report/` does not exist.** D2 §8 puts grade rendering there so the
  CLI, TUI and panel share one implementation. The CLI currently has its own
  `gradeCell` in `cmd/privacybit/list.go`. Extract it before the second
  consumer appears.
- **Only five sources exist.** dnf, zypper, apk, xbps and nix are designed in D6
  but not written. They are additive behind an interface that already exists, so
  they are not blocking — an unsupported distribution still audits through the
  desktop-entry and binary sources.

### 7.2 Places where the design docs were wrong

These were found and corrected in place, but the corrections are recorded here
because a reader who trusts a design doc deserves to know it has been wrong.

- **D1 §8.1 claimed the host had no warning tone.** False. `ui.ToneActivity`
  exists in sysc-shell's `internal/ui/tree.go`, documented as *"a reading past
  its activity threshold but short of critical"*, painting theme **Tertiary**,
  and already in live use at `controlcenter_monitor.go:184` and
  `popout_process.go:204`. Only the *plugin wire* is missing. D1 §8.1 and §12
  were amended.
- **D1 §2.1 said the plugin would import the Go module.** Overturned in D5 §4.1:
  the engine lives under `internal/`, Go forbids cross-module import of
  `internal/`, and `plugins/moonbit` already shells out to its binary. The
  plugin runs `privacybit audit --json`.
- **moonbit's `cmd/installer/main.go` was described as the privileged action
  helper.** It is the *installer* — it runs `make build`, installs the binary,
  writes the desktop entry, sets up systemd units. The real privileged path is
  `plugins/moonbit/runner.go`. If you copy a privileged pattern from moonbit,
  copy that one.

### 7.3 Traps in the environment

- **`ui.Tone` is an iota enum.** Any new tone must be *appended*, never
  inserted, or every later value shifts.
- **`: ` inside an unquoted multi-line YAML scalar starts a key.** Five such
  bugs were found in the catalogue files. Rephrase with a comma rather than
  quoting — quoting a multi-line plain scalar is fiddly and worse prose.
- **`go:embed` cannot reach outside its own package directory.** That is why the
  catalogue lives at `internal/catalog/data/` rather than the repo-root
  `catalog/` that D1 §5.1 specifies. The deviation is intentional; update D1 if
  you care about the mismatch.
- **`dpkg-query` is installed on this machine and manages nothing.** It is the
  reason `Availability` is three-state (`Ready` / `Ready+Empty` / `NotReady`)
  rather than a boolean on `$PATH`.
- **nix holds 7,246 store paths against 2,882 pacman packages.** The binary scan
  excludes `/nix/store` unless a nix source is active, or it reports thousands
  of phantom applications.

## 8. How to audit the work

If you are reviewing what exists, these are the claims worth checking, in rough
order of how much damage being wrong would cause.

1. **Is I2 actually enforced?** Search for every place a `Signal` value is read.
   Every one must be behind an `IsObserved()` check. A single unguarded
   `ValueOr` call is a live under-grade. `ValueOr` exists and returns the
   fallback for unobserved signals, which is safe — but a caller using `.Value`
   directly is not.
2. **Does any source set `Product`?** It must not. Sources report; the grader
   grades. There is a test (`TestSourcesNeverSetProduct`) covering desktop and
   binary — extend it to pacman, dpkg and flatpak.
3. **Is `MapRunner` ever returning empty output for an unfixtured command?** It
   must return an error instead. An unfixtured source that looks empty is
   indistinguishable from a source that looked and found nothing.
4. **Does the CLI still swallow errors?** The `SilenceErrors: true` +
   print-to-stderr arrangement in `cmd/privacybit/main.go` exists because cobra
   was silent and `main` was silent. Check nothing regressed it.
5. **Are the ten catalogue files still valid?** Run `privacybit catalog validate`
   and `go test ./internal/catalog/`. Twenty-one real validation errors were found
   this way, and more by hand-running.
6. **Are there uncommitted changes?** `git status` was clean at handover.

## 9. How to audit the plan

The designs were argued into existence over a long conversation, and several
specific decisions are opinions rather than derivations. Challenge these first.

1. **Brave is amber, against community consensus that rates it green.** The
   argument is the ad-partner business model. It is a judgement call and it is
   defensible either way.
2. **OnlyOffice is red despite being AGPL.** The argument is that it phones home
   for cloud features by default. This is the catalogue's flagship
   licence≠privacy case and other entries depend on the distinction holding.
3. **The catalogue is a flat list of entries, not a nested product/variant tree.**
   `brave` and `brave-origin` are two graded entries; `family` is display-only;
   `channel` feeds a maturity note, never a grade. Retrofitting a nesting level
   later means re-authoring the whole table, so if you want one, do it now.
4. **Grades are `green`/`amber`/`red`, not "orange".** A colour-named grade
   conflicts with the accessibility requirement. The UI shows colour + distinct
   glyph shape + word.
5. **The 0–100 score is not a quality measure** and is only meaningful against
   your own past self. Do not let it become a competitive number.
6. **The panel performs no privileged operation in v1** (D5 §4.2). Panel is
   viewer, TUI is operator. Revisitable, but the reasoning — that a control
   centre which can remove software with one click is a bad control centre —
   should be re-read before changing it.
7. **The daemon runs as a user service, not a root service.** Justified on D1
   §14's scope argument plus the observation that all its unattended work is
   user-scoped. If you disagree, the monthly timer and the event watcher split
   into two designs rather than becoming one root daemon.
8. **VPN monitoring was cut but the point-in-time VPN checks were not.** The user
   cut continuous watching ("complex and not something we want") but the checks
   were their original request. **This reading has not been explicitly confirmed
   with them.** If you are about to build phase 5, ask.

### 9.1 Ideas deliberately rejected, with reasons

Do not re-propose these without new evidence.

| Rejected | Why |
|---|---|
| IP-based telemetry blocklists | DNS is many-to-many onto shared addresses. Google serves mail, login and video from the same frontends as its telemetry. An IP blocklist breaks real services *by default*. |
| A DNS-level shield | You fight whoever owns the resolver — systemd-resolved, NetworkManager, the VPN client, Tailscale. And DoH defeats it in exactly the browser you would want shielded. |
| Per-process network blocking without a kernel module | A project, not a feature. OpenSnitch exists because of this. |
| Licensing a shared moonbit/privacybit package | The genuinely shared surface is ~900 lines of leaf utilities, and every file needs editing on arrival. That is a merge queue, not a reusable module. |
| Licence string as a grading signal | It describes a licence, not a behaviour. Usable only as a cross-check against the catalogue. |
| Grepping binaries for "telemetry" | Matches unrelated code, doc strings and translations. |
| An `install-entry` helper verb | I4. |
| Attending unattended `pacman -Rns` removal | moonbit does it on orphans, where cascading is impossible by definition. A user-chosen purge needs a review token. |

## 10. Environment

Developer machine: Arch Linux, 2,882 pacman packages. Go 1.27.2 toolchain,
modules declare `go 1.26.4`. `flatpak` 1.18.4, `nix` 2.35.3 (unused), `snap` and
`appimagectl` not installed. `dpkg-query` installed but managing nothing.

moonbit at `/home/nomadx/moonbit` is **132 commits behind `origin/main`** — read
the remote, not the working tree. It has ~11,769 non-test lines and is the
reference for TUI structure, CLI shape, sudo handling and daemon design.

Useful commands:

```console
go build ./... && go vet ./... && go test ./...
gofmt -l ./internal ./cmd
go run ./cmd/privacybit list --category browser --grade red
go run ./cmd/privacybit explain brave
go run ./cmd/privacybit catalog validate
```

## 11. If you have to change the plan

The design docs are the plan. If you change one, change the doc in the same
commit as the code, and record *why* in the doc. Several docs already carry
"this was wrong, here is the correction" notes, and those notes are the most
valuable part of the documents — they explain the constraint that produced the
correct answer, which is not recoverable from the answer alone.

Do not weaken an invariant to make something work. If I2, I4 or I5 is blocking
you, the thing you are building is probably wrong, not the invariant.