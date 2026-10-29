# privacybit — D6: multi-distribution source layer

**Date:** 2026-10-20
**Status:** approved in dialogue, pre-implementation
**Depends on:** D1 (engine), D2 (CLI)
**Amends:** D1 §6.1 (source table), D4 §3.4 (install command)

---

## 1. The problem

D1 §6.1 lists sources as a table of specific commands. That is a list, not an
abstraction, and it is written as though Arch were the world. It is the one part
of D1 that has to change before anything else can be built, because detection
is phase 2 and everything after it — grading, findings, the catalogue's
`detects` rules, the install command — sits on top of it.

Three consequences of not fixing it now:

1. The catalogue's `detects` rules would carry Arch names only, and adding a
   distribution later means re-authoring the whole table.
2. D4 §3.4 prints `pacman -S brave-origin` as the install remedy. On Debian that
   command does not exist, and on a distribution with no such package the tool
   would be recommending something that cannot work.
3. A distribution with no backend would be "unsupported" — which is the worst
   possible failure mode, because the universal sources would still have found
   most of the software.

## 2. The source interface

A source is one way of seeing what is installed. It does not answer questions;
it reports observations, and it declares up front which facts it can support.

```go
// Source is one way of seeing what is installed.
type Source interface {
    // Name identifies the source in coverage reporting.
    Name() string

    // Availability reports whether this source can run here, and if not, why.
    Availability() Availability

    // Capabilities declares which facts this source can supply. A source that
    // lists packages but cannot report licences says so, and the audit treats
    // licences as unavailable rather than absent.
    Capabilities() Capabilities

    // Inventory enumerates what this source currently sees.
    Inventory(ctx context.Context) ([]Observation, error)
}
```

```go
// Observation is one piece of software as one source saw it.
type Observation struct {
    // Name is the source-native identifier: an rpm, a deb, a flatpak app id.
    Name string

    // Version, where the source has one.
    Version Signal[string]

    // Product is the catalogue entry this resolves to, if any. Empty means
    // unknown, never a bad grade.
    Product string

    // Origin is provenance: which repo, remote, or nowhere.
    Origin Signal[Origin]

    Facts Facts
}

type Facts struct {
    Licence  Signal[string]
    Vendor   Signal[string]
    Explicit Signal[bool] // installed deliberately, or pulled in as a dependency
    ReverseDeps Signal[[]string] // what depends on this — D4 plan-purge needs it
    Signed   Signal[bool] // package-manager signature validation state
}
```

## 3. Capabilities, and the one invariant that matters

The audit does not want "list packages". It wants facts, and **not every backend
can supply every fact**. Modelling the gap as a capability matrix rather than as
a null value is the whole design.

| Backend | List | Licence | Vendor | Explicit | Origin | Reverse deps | Signature |
|---|---|---|---|---|---|---|---|
| pacman | yes | yes | yes | yes | yes | yes | yes |
| dpkg / apt | yes | — | —¹ | partial | partial | — | — |
| dnf / yum | yes | yes | yes | yes | yes | yes | yes |
| zypper | yes | yes | yes | yes | yes | yes | — |
| apk | yes | partial² | — | — | partial | — | — |
| xbps | yes | partial | — | — | partial | partial | — |
| nix | different shape — §12 | | | | | | |
| flatpak | yes | — | — | — | yes | — | — |

¹ `dpkg-query -f='${Maintainer}'` returns a maintainer string. privacybit does
not expose it as a vendor fact.
² `apk info -W` gives copyright text, not a licence identifier.

`dpkg / apt` explicit-install state comes from `apt-mark showmanual`.
`db:Status-Status=installed` only reports package state; it does not separate
manually selected packages from automatic dependencies. Match apt marks against
dpkg's architecture-qualified `${binary:Package}` identity while keeping
`${Package}` as the catalogue name. If `apt-mark` is missing or fails,
explicit-install state is unavailable for that scan.

`dpkg-query -W` also returns residual package records whose status is
`config-files`. Inventory and availability count only rows with
`${db:Status-Status}=installed`.

Verified on this machine (Arch, 2,882 packages), pacman's field set is:
`Name Version Description Architecture URL Licenses Groups Provides DependsOn
OptionalDeps RequiredBy OptionalFor ConflictsWith Replaces InstalledSize
Packager BuildDate InstallDate InstallReason InstallScript ValidatedBy`.

Two of those deserve comment. `RequiredBy` is the reverse-dependency edge D4
§7 needs to compute a purge plan, and it is why pacman is the richest backend.
`ValidatedBy` is pacman's signature validation state — a genuine supply-chain
signal that lands directly on D1 §3.4's packaging-origin row, and one most
backends have no equivalent of.

### 3.1 Three-state signals

```go
// Signal is a fact that may be unavailable. Its zero value is not "false";
// it is "nobody looked", and the audit must never read it as a pass.
type Signal[T any] struct {
    State SignalState // Observed | Unavailable | Contradicted
    Value T
    Why   string      // explains Unavailable and Contradicted states
}
```

### 3.2 The invariant

> **Unavailable or unset signals cannot move a grade. Contradicted evidence may
> only worsen it.**

Unavailable signals only reduce reported coverage. Contradicted signals contain
a measured fact that disagrees with the catalogue and may push the grade toward
more risk. This is D1 §3.3's monotone rule applied to measured evidence, with
absence of evidence kept out of the grade entirely.

The failure it prevents is concrete and would have shipped on Debian: `dpkg`
has no licence metadata, so a package whose catalogue entry says FOSS cannot be
cross-checked. "No contradiction found" is not evidence of agreement; the
backend had nothing to say. The same shape of bug is D1 §6.4's flatpak
network-visibility rule — "no telemetry observed" is not evidence of clean.

Four rules follow:

1. **`Observed` and `Contradicted` carry measured evidence.** Neither may
   improve a grade; `Contradicted` may only worsen it.
2. **`Unavailable` and the zero state cannot inform a grade in either
   direction.** They reduce coverage only.
3. **Catalogue grade is always the floor.** Signals can push toward risk, never
   lift it. This is D1 §3.3, unchanged.
4. **Insufficient coverage to judge means `unknown`, not `green`.** Absence of
   evidence is a reportable state, never a passing one.

### 3.3 Availability means data, not binary

A presence check on `$PATH` is not availability. `dpkg-query` is installed on
this Arch machine and reports `no packages found matching bash` for everything —
the binary exists, the tool works, and there is no Debian system to inspect.

```go
type Availability struct {
    Ready   bool
    Reason  string // why not, when not Ready
    Empty   bool   // installed and working, but nothing to report
}
```

`Empty` is distinct from `Ready` and neither is an error. A machine can
legitimately have no flatpaks, and reporting that as a coverage gap would be
wrong.

## 4. Backends

Each backend is small and independent. The shared shape is: probe, enumerate,
and for the matched subset only, query metadata — because `-Qi` across thousands
of packages is slow, and D1 §6.2's two-tier cadence depends on that split.

| Backend | Enumerate | Metadata for matches | Origin from |
|---|---|---|---|
| pacman | `pacman -Qq` | `pacman -Qi <name>` | repo list, `InstallReason`, `ValidatedBy` |
| dpkg | `dpkg-query -W` (installed status only) | `dpkg-query -W -f=...`; `apt-mark showmanual` for explicit state | apt policy / sources.list |
| dnf/yum | `dnf repoquery --installed` | `dnf repoquery --info` | `@System` vs repo id |
| zypper | `zypper --query --installed` | `zypper --info` | repo id |
| apk | `apk info` | `apk info -a` | repo tag |
| xbps | `xbps-query -l` | `xbps-query -R` | repository field |

Flatpak sandbox permissions use a separate per-copy query. Enumerate each app
with its installation, branch, and architecture, then call
`flatpak info --show-permissions` with those selectors. Flatpak returns the
deployed app context with the applicable global and per-app overrides already
merged for the current user and installation. A failed query is unavailable
evidence; the app remains in inventory. The audit only exposes classified
grants and sanitized reasons, never the raw output or environment values.

**The enrich-only-matches rule matters.** On this machine `pacman -Qq` is
~100 ms across 2,882 packages; running `pacman -Qi` across all of them is orders
of magnitude slower. D1 §6.2's fast/slow split only works if the slow tier is
spent on the handful of packages the catalogue actually matched.

## 5. Universal sources

These work on every distribution, need no native backend, and are the reason an
unsupported distribution degrades rather than fails.

**`.desktop` files.** Every app that registers a launcher is visible in one
directory scan: `/usr/share/applications`, `/usr/local/share/applications`,
`~/.local/share/applications`, and the flatpak export directories. This machine
has 412 system and 85 user entries. It catches `brave.desktop`,
`org.freetube.FreeTube.desktop` and `com.brave.Browser.desktop` identically,
which is precisely the cross-distro case the package backends struggle with.

**Binary paths.** `PATH`, `/opt`, `/usr/local/bin`, `~/.local/bin`. Catches
AppImages, tarball installs, `go install`, and anything a user put somewhere.

Universal sources are **corroboration, not primary evidence** (D1 §4.4). A file
called `signal` is not necessarily Signal Desktop. They also yield no licence,
vendor, or reverse-dependency facts, so on a distribution with no native backend
the audit reports reduced coverage — visible, honest, and still useful.

### 5.1 Source interaction: nix store pollution

A universal source that walks a nix installation will drown. This machine has
**7,246 paths under `/nix/store`** against 2,882 pacman packages, named like
`002l21jraxpj4cx6z1w4jyjzjwk8g0m9-docbook5-5.0.1.drv`.

Left alone, the binary-path scan reports thousands of phantom applications and
the audit becomes noise. Two mitigations, both required:

1. `/nix/store` is excluded from the binary-path source unless a nix source is
   active, in which case nix owns it.
2. Deduplication is by resolved target, not by path, so the ~6,000 store paths
   behind one profile entry collapse to the profiles that are actually installed.

## 6. Detection and selection

No configuration. The distribution is detected.

1. `/etc/os-release` `ID=` is authoritative.
2. Binary presence is the fallback when `os-release` is absent or unrecognised.
3. When more than one backend is available, all of them run, and the one matching
   `os-release` is marked primary in coverage. Multiple backends are not an
   error — a container or a WSL layer can legitimately have two.

Detection picks *which* backends to run. It never decides what a grade is. A
machine with dpkg and pacman gets two inventories and one merged, deduplicated
view, with the primary marked.

## 7. The catalogue across distributions

The same product has different names everywhere:

| Entry | Arch | Debian / Ubuntu | Fedora | Flatpak |
|---|---|---|---|---|
| brave | `brave-bin` | `brave-browser` | `brave-browser` | `com.brave.Browser` |
| brave-origin | `brave-origin` | — | — | `com.brave.Browser.origin` |
| bitwarden | `bitwarden` | `bitwarden-desktop` | `bitwarden` | `com.bitwarden.desktop` |
| freetube | `freetube` | `freetube` | — | `org.freetube.FreeTube` |
| mullvad-browser | `mullvad-browser` | — | — | — |

The `detects` rule list already accepts multiple rules per entry, so this is
simply more rules. **No new axis is added to the identity model.**

That is worth stating as a principle, because the identity model was settled
deliberately and a nesting level here would be expensive:

> **Grades do not vary by distribution. Only names do.**

Brave is Brave on Arch and on Debian. The thing that genuinely varies is
packaging origin — official distro repository against AUR against a third-party
remote — and that already feeds `Origin` and `maturity`, never the grade.

An empty cell above is meaningful data: `brave-origin` genuinely is not packaged
for Debian, and that is what the install-command renderer (§8) has to report.

## 8. Per-distribution command rendering

This section exists because D4 §3.4 hardcoded `pacman -S brave-origin`, which
would have been a broken recommendation on every other distribution.

```go
// Renderer turns a catalogue entry into a command the user can actually run
// on this machine, or an honest statement of why they cannot.
type Renderer interface {
    // Install returns the command for obtaining entry, or ErrUnavailable.
    Install(ctx context.Context, entry catalog.Entry) (Instruction, error)

    // Remove returns the command, which always requires review (D4 §7).
    Remove(ctx context.Context, entry catalog.Entry) (Instruction, error)
}

type Instruction struct {
    Command  string   // "" when Available is false
    Available bool
    Reason   string   // e.g. "not packaged for Debian; see https://brave.com"
    Source   Origin
}
```

Three outcomes, and the third matters as much as the other two:

| Case | Output |
|---|---|
| Packaged in a configured repo | `pacman -S brave-origin` |
| Known but not packaged here | *"brave-origin is not packaged for Debian. See https://brave.com."* |
| Unmapped name | *"No install route is known for <entry> on <distro>."* |

The failure mode this prevents is worse than an error. A tool that emits a
command which cannot work teaches users that its recommendations are untested,
and the next recommendation gets ignored.

The renderer also carries D4 §3.4's refusal forward: even when a command exists,
the helper never runs it. Installing is tier 3, printed for the user, for every
distribution.

## 9. Validation

The catalogue validator gains one rule that matters more than the rest:

> **No package name may be claimed by two entries.**

A duplicated alias is a direct contradiction, and the audit's tie-break rules
would resolve it by specificity into a confidently wrong grade. Rejecting it at
load turns a silent false positive into a startup error.

A misspelled alias is the opposite case — a silent false *negative*, on one
distribution only, which D1 §14 already accepts. It is caught by the
cross-distribution golden tests in §11, not by the validator, because a validator
cannot know which names exist.

## 10. Coverage reporting

Coverage is a first-class output, not a debug affordance. The snapshot grows a
per-source entry, and D2's human output renders it.

```json
{
  "coverage": {
    "distribution": { "id": "arch", "id_like": ["arch"] },
    "sources": [
      { "name": "pacman",    "state": "ready", "observed": 2882, "facts": ["licence","vendor","explicit","origin","reverse_deps","signed"] },
      { "name": "flatpak",   "state": "ready", "observed": 5,    "facts": ["origin"] },
      { "name": "desktop",   "state": "ready", "observed": 497,  "facts": [] },
      { "name": "binary",    "state": "ready", "observed": 61,   "facts": [] },
      { "name": "dpkg",      "state": "empty", "observed": 0,    "facts": [] },
      { "name": "nix",       "state": "unavailable", "why": "no nix source in v1" },
      { "name": "snap",      "state": "unavailable", "why": "snap not installed" }
    ],
    "reduced": [ { "fact": "licence", "sources": ["desktop","binary"], "effect": "catalogue grades uncorroborated on these observations" } ]
  }
}
```

`reduced` is the honest form of §3.2's invariant reaching the user. When the
`reduced` array is non-empty, the overall grade carries a visible caveat rather
than silently reading as more confident than the evidence supports.

D3's `ScreenCoverage` renders exactly this, and it already exists on the
thirteen-screen inventory.

## 11. Testing

| Property | Method |
|---|---|
| Per-backend parsing | Golden fixtures per backend per distro. Recorded output, never live. |
| Cross-distribution catalogue | Run the full catalogue against each distro's package set fixture; assert every entry with a known alias resolves. This is what catches the misspelled aliases §9 cannot. |
| Monotone + unavailable | Property test: for every entry and every signal-state combination, an `Unavailable` signal cannot improve or worsen the grade. `Contradicted` may only worsen it. |
| Availability ≠ binary | `dpkg-query` present with an empty database must yield `Ready+Empty`, not `NotReady`. |
| Nix pollution | A fixture with 7,000 store paths must not produce phantom applications. |
| Command rendering | One case per outcome in §8, per distribution, asserting the third case never emits a command. |
| No shell | Every backend test asserts no command outside its declared argv. |

CI runs the backend matrix as containers — Arch, Debian, Fedora, Alpine — so the
golden fixtures are produced against real output rather than invented. The
universal-source tests run once and must behave identically everywhere.

## 12. Deferred, explicitly

**nix.** Verified on this machine: 7,246 store paths, content-addressed names
where even the readable name requires stripping a 32-character hash, and
`nix profile list` refuses to run without `--extra-experimental-features
nix-command`. Nix has no packages in the sense every other backend means it; it
has derivations and a store. It wants its own source type rather than a flag on
the existing one, and pretending otherwise would produce wrong answers. §5.1
excludes the store from the binary scan in the meantime, so nix machines get a
clean audit that does not mention nix rather than a noisy one that mentions it
wrongly.

**snap.** Absent here and absent on most non-Ubuntu distributions. It needs its
own source for the same reason, and D1 §6.6 already records it as deferred.

## 13. Consequences for other designs

| Design | Change |
|---|---|
| **D1 §6.1** | Replaced with a pointer here. The command table becomes §4. |
| **D1 §6.2** | Cadence gains the enrich-only-matches rule, which is what makes the slow tier affordable. |
| **D1 §3.5** | Gains a companion: licence strings are not just weak evidence, they are *absent* evidence on some backends. |
| **D4 §3.4** | Install output is per-distribution (§8). The helper still never runs it. |
| **D2 §6** | `--json` gains `coverage.sources[]` and `coverage.reduced[]`. |
| **D3** | `ScreenCoverage` renders the new shape. Already on the inventory. |
| **D7 (daemon)** | The package-poll source for new-app alerts is whichever backends are active, so the daemon inherits this layer for free. |
