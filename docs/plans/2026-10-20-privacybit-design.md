# privacybit — design

**Date:** 2026-10-20
**Status:** approved in dialogue, pre-implementation
**Repo:** https://github.com/Nomadcxx/privacybit

---

## 1. Problem

Users make privacy decisions from vibes: "I have Brave so I'm fine", "I saw a VPN
icon so I'm covered". Both are frequently wrong. privacybit exists to turn those
vibes into an inspectable, reproducible audit with a stated basis for every
grade and a concrete action for every finding.

Three failure modes it must not have:

1. **Under-grading.** Telling someone their setup is fine when it is not is the
   one unacceptable error. Every other failure mode is recoverable.
2. **Wall-of-shame.** A report that opens with fourteen red items and no wins
   gets deleted. Lead with the summary and the achievable fixes.
3. **False positives.** One wrong accusation destroys trust faster than ten
   misses. When uncertain, say unknown.

### 1.1 Vocabulary: privacy, not security, not anonymity

These are three different properties and the audit must not blur them:

- **Privacy** — who can access your information.
- **Security** — protection from unauthorised access or theft.
- **Anonymity** — your actions cannot be traced back to you.

They are **not interdependent**. Google systems are secure and unlikely to be
hacked, yet Google still has access to your personal data. Signal offers high
privacy but not anonymity, because registration requires a phone number.

privacybit grades **privacy**. Where a finding is really about security or
anonymity it is either excluded or labelled as such. UI copy must never imply
"green" means "private" in the strong sense; green means "no meaningful
unwanted disclosure identified by the checks we ran".

## 2. Scope and shape

privacybit is a **standalone project**: a Go CLI with a TUI, a scheduler, and a
root helper. It does not depend on sysc-shell.

```
                    privacybit (this repo)
                    ├── CLI      privacybit audit|watch|explain|apply|serve
                    ├── TUI      privacybit tui
                    ├── scheduler    cron/systemd-timer friendly
                    ├── scheduler → helper   privacybit-helper (root, closed verb set)
                    └── library   importable Go package
                              │
                              │  Go module dependency (read path)
                              ▼
                      sysc-plugins/plugins/privacy
                              │
                              │  state.set "control_center"  (bounded JSON snapshot)
                              ▼
                     sysc-shell Control Centre → privacy panel
```

### 2.1 Build order

The dependency edge runs one way, so the order is forced:

1. **privacybit** — engine, CLI, catalogue, helper. Independently useful.
2. **plugins/privacy** — a thin sysc-shell plugin that imports the library, runs
   an audit on a tick, and publishes a snapshot.
3. **sysc-shell** — the host renders the snapshot in the existing Control Centre
   privacy panel.

This is the "mix of standalone project and privacy-panel wiring" shape: the
logic lives in a project that can stand alone, is testable without a shell, and
can be consumed by the TUI, a cron job, or the panel. The panel is a *consumer*,
not the owner.

### 2.2 Why not render the panel from the plugin

The Control Centre is entirely host-rendered: `ccPage` in
`internal/shell/popout_controlcenter.go` is a `switch h.section` over host-side
functions, and there is no plugin branch. plugin/v1 can publish bounded JSON
into its own state and the host renders it natively — the calendar plugin
(`org.sysc.calendar` → state key `control_center`) is the existing precedent.

So the panel section stays in the host, and the plugin's only job is to produce
data. This is also why the engine must be a library: if the plugin could not
import it, the plugin would have to shell out, and then the CLI and the panel
would be running two different implementations of the same audit.

## 3. Rating model

### 3.1 Grade vocabulary

| Grade | Meaning |
|---|---|
| `green` | Privacy-respecting by default. No meaningful unwanted disclosure found. |
| `amber` | Acceptable with caveats, or configurable into good shape. Needs a decision from the user. |
| `red` | Privacy-hostile by default, or collects data that cannot practically be avoided. |
| `unknown` | Installed but not in the catalogue. **Not a penalty.** Reported separately. |

Note: `amber` rather than `orange`. A colour-named grade conflicts with the
accessibility requirement in §8 — the whole point is that meaning does not live
in the colour. The UI shows the colour, a distinct glyph, and the word
("Good" / "Review" / "Risky"). Internally the vocabulary is deliberately not
named after a hue.

### 3.2 Two evidence sources

1. **Catalogue** — an authored, versioned table describing each known
   product-variant. This carries the reputation.
2. **Derived signals** — measured facts about the live system. This carries the
   evidence that the catalogue cannot know.

### 3.3 The monotone rule

**A derived signal may only move a grade toward more risk, or raise a flag. It
may never improve a grade.**

An audit that reports "better than expected" on the strength of a heuristic is
producing an unearned claim. If the heuristics are good, the answer is a
finding on something else, not a promotion. This is the single most important
invariant in the codebase.

Corollary: there is no path from `red` to `amber` at runtime. Reduction happens
only when the *user changes the system* and the audit re-runs from scratch.

### 3.4 Accepted derived signals

| Signal | Source | Strength |
|---|---|---|
| Live outbound connections to known telemetry/analytics endpoints | `ss -Htunp` + `/proc/<pid>/exe` → catalogue endpoint set | Strongest. Directly measured. |
| Packaging origin | official repo vs AUR vs third-party flatpak remote vs `~/.local` vs AppImage | Good. Untrusted origin raises supply-chain risk. |
| Sandbox posture | flatpak permissions, setuid/setgid bits | Good. A permissive flatpak weakens containment. |
| Catalogue/package metadata mismatch | `pacman -Qi` licence vs catalogue expectation | Good. Doubles as the table-staleness signal. |

### 3.5 Rejected derived signals

Explicitly rejected as too unreliable to grade on, recorded here so they are not
re-proposed later:

- **Licence string alone.** `License: GPL` describes a licence, not a behaviour.
  It is only usable as a *cross-check* against the catalogue (§3.4, row 4).
- **Grepping binaries for the string "telemetry".** Matches unrelated code,
  doc strings and translations. High false-positive rate.
- **Package descriptions.** Mostly upstream marketing prose.
- **"Is it popular?"** Popularity correlates with nothing useful here.
- **DNS resolution attempts.** A program resolving a hostname is not the same as
  it transmitting the data, and the correlation is too loose to grade on.

### 3.6 Score

A 0–100 score is emitted for the progress indicator people actually respond to.
It is explicitly **not** a quality measure and **not** comparable between
machines. The only meaningful comparison is a user against their own past self,
which is what the scheduler's history is for. The UI must say so.

## 4. The identity model

A flat list of package names does not work. `brave` and `brave-origin` are
**different products** with different privacy postures; so are Chrome,
Chromium and Ungoogled Chromium; so are stable, beta and nightly. The catalogue
must model this without becoming unwieldy.

### 4.1 Entry — the unit of grading

One catalogue entry is **one concrete product-variant**, and it is the only thing
that carries a grade.

```yaml
- id: brave-origin
  category: browser
  name: Brave Origin
  grade: green
  family: brave                      # display grouping only; never grades
  summary: Brave compiled without Brave cloud services, Rewards and ad-partner code.
  reasons:
    - No Brave account or Rewards integration.
    - No Brave ad-partner components in the build.
  caveats: []
  alternatives: []
  detects:
    - {type: package, name: brave-origin}
    - {type: package, name: brave-origin-bin}
    - {type: binary,  name: brave-origin}
    - {type: flatpak, id: com.brave.Browser.origin}

- id: brave
  category: browser
  name: Brave
  grade: amber
  family: brave
  summary: Shields by default, but ships a vendor account, Rewards and ad partners.
  reasons:
    - Brave Accounts and Rewards are tied to a Brave-operated service.
    - Historically routed anonymised browsing data to an ad partner.
  caveats:
    - Disable Rewards and remove the Brave account to close most of this gap.
  alternatives: [brave-origin, librewolf, mullvad-browser]
  detects:
    - {type: package, name: brave}
    - {type: package, name: brave-bin}
    - {type: binary,  name: brave}
    - {type: flatpak, id: com.brave.Browser}
```

`brave` and `brave-origin` are **two entries with two grades**, not one product
with a variant flag. Grading stays a pure function of the entry, which keeps
`explain` trivial and keeps derived signals from having to reason about
inheritance.

### 4.2 Family — display grouping only

`family` exists so the UI can say "your browsers: Brave, Brave Origin, Firefox,
Chrome" instead of a flat alphabetical list. It never affects a grade. It is
used for one more thing: when a red entry has a green sibling in the same
family, that sibling is the default suggestion for "replace this".

### 4.3 Channel — a trust flag, not a grade

`stable`, `beta`, `nightly`, `esr` describe how well-audited a build is, not
whether it is privacy-respecting. A nightly Ungoogled Chromium is still `green`
on privacy. Channel feeds a separate **maturity** flag surfaced as a note, so
the grade stays honest and the caveat stays visible.

Distribution source (`official`, `aur`, `flatpak-third-party`, `local`,
`appimage`) also feeds maturity and supply-chain, never the privacy grade.

### 4.4 Detection rules

Each entry carries `detects`, a list of match rules:

| Type | Matches |
|---|---|
| `package` | pacman / dpkg package name, exact |
| `binary` | basename found on PATH, `/opt`, `/usr/local/bin`, `~/.local/bin` |
| `flatpak` | flatpak application id |
| `desktop` | `.desktop` basename stem, system and user scopes |

Resolution: collect all matches across all sources, then select.

1. Exact matches on any high-confidence source (`package`, `flatpak`) win.
2. `binary` and `desktop` are corroboration, not primary evidence — a binary
   named `signal` is not necessarily Signal Desktop.
3. Tie-break by entry specificity (an exact match beats a substring match).
4. Two entries matching with equal confidence is a **catalogue defect**, not a
   user problem. It is logged and the item is reported `unknown`.

This is deliberately conservative: corroboration exists so that a package whose
name collides with another product cannot produce a confident wrong grade.

## 5. Catalogue

### 5.1 Data, not code

The catalogue is embedded YAML plus overridable user fragments. Rationale: if
adding an app requires a recompile, the table rots, and a stale privacy table is
worse than none.

```
catalog/                 # shipped, embedded, versioned
  browsers.yaml
  vpn.yaml
  passwords.yaml
  chat.yaml
  email.yaml
  office.yaml
  pdf.yaml
  media.yaml
  network.yaml
~/.config/privacybit/catalog.d/*.yaml   # user overrides, additive
```

Load order: shipped entries first, user fragments override by `id`. An override
may change a grade, which is a legitimate user decision — but the UI shows the
catalogue's grade alongside the override so the effect is never silent.

Validation at load: unique ids, valid grades, `alternatives` resolve to existing
ids, `detects` rules parse, `family` values exist. A bad fragment is rejected
whole with a clear error rather than partially applied.

### 5.2 Categories and the baseline problem

**Absence is a finding, not a pass.** A machine with no password manager has a
`red` password manager category, not an empty one. Every category declares an
`expectations` list — the entries that satisfy it — and an unmet expectation is
a finding in its own right.

```yaml
- id: passwords
  name: Password manager
  expectations:
    - any-of: [bitwarden, pass, gopass, keepassxc, proton-pass, aliasvault]
      severity: high
      why: Nothing is storing, generating or syncing credentials for you.
```

### 5.3 Taxonomy v1

Grades as agreed, with research corrections applied.

| Category | Red | Amber | Green |
|---|---|---|---|
| Browser | Chrome, Edge, Opera, Vivaldi | Brave, Firefox, Konqueror | Brave Origin, Waterfox, Mullvad Browser, LibreWolf, Tor, Falkon, Ungoogled Chromium, Zen, Floorp |
| VPN | ExpressVPN, NordVPN, Surfshark, any closed-source client | Proton VPN | Mullvad VPN, IVPN, Windscribe, Amnezia |
| Password manager | 1Password, LastPass, Dashlane | KeePassXC, Proton Pass, Padloc | Bitwarden, Pass, gopass, AliasVault |
| Chat | Slack, Discord, Teams, Zoom, Telegram, WhatsApp | Signal, Element | HexChat, WeeChat, Irssi, Pidgin, Dino, Gajim, SimpleX, DeltaChat, ToXC |
| Email | TheBat!, Outlook web | Thunderbird | NeoMutt, Mutt, KMail, Evolution |
| Office | WPS Office, MS 365, OnlyOffice Desktop | LibreOffice (unhardened) | LibreOffice (hardened), CryptPad, Etherpad |
| PDF | Foxit, Acrobat Reader | Zathura, Okular | Papers, Atril, mupdf, qpdfview |
| Media | Spotify, proprietary-codec-only players | VLC, mpv (unhardened) | FreeTube, mpv, yt-dlp |

Notes on the corrections:

- **OnlyOffice is red as an open-source-but-phones-home case.** It is AGPL; the
  problem is cloud-default behaviour, not licence. This is the clearest
  demonstration that licence and privacy grade are independent axes.
- **Brave is amber, against community consensus that rates it green.** The
  reservation is the ad-partner business model, and the argument is real. The
  `alternatives` field points at Brave Origin first.
- **Thunderbird is amber because it is telemetry-capable**, not because it is
  proprietary. Same open-source-but-phones-home distinction.
- **Signal is amber because it is not anonymous** (phone-number registration) —
  it is nonetheless genuinely good on privacy. The copy must say so without
  implying it is bad.
- **Extension hygiene** (uBlock Origin, Privacy Badger, ClearURLs, LibRedirect)
  is a category. A hardened browser without a content blocker is not hardened.
- **Metadata hygiene** (MAT2, Metadata Cleaner) is a category. Everything else
  here is about the network; this is about the files.

### 5.4 Alternatives are first-class

Every red entry carries `alternatives`. A wall of shame is demoralising and
unactionable; a red entry with a concrete green replacement is a to-do list.
This is also what makes the audit useful to someone who does not want to be told
to uninstall things — they can dismiss, and still get the suggestion.

## 6. Detection

### 6.1 Sources

Superseded by
[`2026-10-20-privacybit-sources-design.md`](2026-10-20-privacybit-sources-design.md),
which replaces this table with a backend interface and a capability matrix. The
original list was Arch-shaped: it named four package managers by command and
omitted dnf, zypper, apk, xbps and nix entirely, and it had no way to express
"this backend cannot tell me the licence".

What survives from the original table:

| Source | Notes |
|---|---|
| desktop files | `/usr/share/applications`, `/usr/local/share/applications`, `~/.local/share/applications`, flatpak export dirs. 412 + 85 files here; catches AppImage and user installs. Universal — no native backend needed. |
| binaries | `PATH`, `/opt`, `/usr/local/bin`, `~/.local/bin`. Universal. |
| processes | `/proc/<pid>/exe`. Identity, not just presence. |
| flatpak | Its own section, §6.4. |

What D6 adds:

- A `Source` interface with an explicit capability matrix, so an unsupported
  fact is *unavailable* rather than silently absent.
- **The invariant that unavailable never reads as clean** — the general form of
  the rule §6.4 already needed for flatpak network visibility.
- Per-distribution install-command rendering, which §9.3's `pacman -S` example
  assumed and should not have.

`/etc/os-release` selects backends; presence of a binary alone does not.
`dpkg-query` is installed on this Arch machine and manages nothing, which is
precisely why availability is `Ready | Empty | Unavailable` rather than a boolean.

### 6.2 Cadence

Two tiers, because the costs differ by three orders of magnitude.

- **Slow scan** — package inventory, catalogue matching. On install, on demand,
  or hourly at most. Result cached.
- **Fast poll** — network connections, VPN state, sandbox posture. On demand, or
  ~2 s while a view is open. This is what makes the panel feel live.

The 500 ms PipeWire cadence used by the existing capture panel is deliberately
*not* copied here; network state does not move fast enough to justify it.

### 6.3 Findings about the system that are not applications

Several of the highest-value findings are not in any package inventory:

- Desktop analytics daemons (GNOME Shell analytics, KDE telemetry, others)
- Firefox / Thunderbird telemetry preferences
- `fwupd`, `upower`, `systemd` timers that phone home
- Default browser and default search engine
- Which browsers are **running right now**, not merely installed

For browsers specifically, **configuration beats choice**: a hardened Firefox
outscores stock LibreWolf. The audit reads the actual profile preferences where
it can and reports the *effective* posture, not the nominal one.

### 6.4 Flatpak

Flatpak is not just another package source. It carries its own trust model, and
an audit that flattens it to "is the app installed?" throws away most of what is
worth knowing. Five things are checked.

**Effective permissions, not declared ones.** `flatpak info --show-permissions`
reports what the app's *metadata* declares. `flatpak override` mutates what
actually happens, and overrides persist invisibly — nothing in the app listing
shows them. There is no "list all overrides" command, so the audit enumerates
known application ids and reads `flatpak override --show` for each. Reading
declared permissions and reporting them as effective would be a false negative.

**Installation scope, and duplicates across it.** `system` installations are
root-owned; `user` installations are not, and for the owning user the `user`
copy shadows the `system` one. The same app id at both scopes means two copies
at different privilege levels and dead weight, and which one runs depends on
resolution order. Observed on the development machine: one application
installed at both `system` and `user` scope. This is a finding.

**Remote trust.** Each remote carries its own origin. Flathub is the default
and treated as neutral; any third-party remote is a supply-chain finding and
feeds maturity, never the privacy grade. A remote configured identically at both
`--system` and `--user` scope is worth noting, because updates resolve against
whichever applies and they can silently diverge.

**Sandbox posture is a separate axis from vendor reputation.** A well-regarded
app in a sandbox with `devices=all`, `filesystems=host` and `shared=network` has
lost most of the containment that made it safe to run. Both facts are reported:
the app grade and the sandbox grade, and neither is allowed to launder the
other. A `green` app in a broken sandbox is not a green machine.

**Network visibility is degraded inside flatpak, and that must be stated.** This
is the important one. An app that unshares its network namespace has its
connections in a namespace the host's `ss` will not attribute cleanly; an app
that shares the host namespace inherits whatever the host can see. Either way
PID→exe attribution through `bwrap` is unreliable. **The consequence is that
"no telemetry connections observed" for a flatpak app is not evidence of
clean.** Reporting it as a pass would be exactly the under-grade that §0 rules
out. Flatpak apps with unverified network visibility report `unknown` for that
signal, never `green`.

### 6.5 Runtimes and the shared stack

The runtime layer is shared by every flatpak app, so it deserves a check of its
own rather than being treated as plumbing.

- **Proprietary components in the runtime stack.** The development machine has
  `org.freedesktop.Platform.openh264` and `org.freedesktop.Platform.codecs-extra`
  installed. An open-source desktop running a proprietary codec runtime is a
  legitimate thing to surface, and it affects every flatpak app on the machine
  rather than one.
- **Branch age.** Multiple `org.freedesktop.Platform` branches at different
  versions were present. An end-of-life branch is a maintenance and security
  matter; it is reported as maturity, not as a privacy grade, and the two are
  not conflated.

### 6.6 Snap and AppImage

Deferred, not ignored. Neither tool is installed on the development machine, so
both would need their own probing path and neither can be designed against real
output yet. The `.desktop` scan in §6.1 already catches AppImage-based apps that
register a launcher, so the practical gap is narrower than it looks.

This is the hardest part of the audit, and the easiest to get wrong in a way
that actively misleads.

### 7.1 Why `tun0` is not a check

The presence of a tunnel interface says a client started. It says nothing about
whether traffic is actually routed through it. A VPN client can be running,
connected, and leaking everything. Worse, the naive check produces the most
dangerous outcome available: **a green light that means nothing.** That is worse
than an obviously-off VPN, because it teaches the user the signal is reliable.

### 7.2 What is actually checked

| Check | Method | Fails as |
|---|---|---|
| Tunnel present | `ip -o link` interface names + known client processes | `unknown`, never `disconnected` |
| Default route through tunnel | `ip route get 1.1.1.1` and resolve the egress interface | `red` flag: "connected but not routing" |
| DNS containment | `resolvectl status` / `/etc/resolv.conf` nameserver set vs expected | `red` flag: "DNS leaking to <resolver>" |
| IPv6 escape | compare IPv6 default route against tunnel | `red` flag: "IPv6 bypasses the tunnel" |
| Kill switch | absence of an outside default route in the routing table | note |

A VPN with DNS leaking and IPv6 escaping reports **red**, not green, with the
specific defects named. This is the single most valuable finding the tool can
produce.

### 7.3 Client presence is not connection

`wireguard-tools` is plumbing, not a client. A catalogue hit for a client entry
means a client is installed; connection state comes from the network checks
above, and the two are reported in separate fields so neither implies the other.

## 8. Accessibility of the grade display

Red/green/amber alone fails a substantial minority of men. Grade therefore
carries **three redundant channels**:

1. **Colour** — green / amber / red.
2. **Glyph shape** — circle / triangle / square, distinguishable in greyscale.
3. **Word** — "Good" / "Review" / "Risky".

### 8.1 Host gap this exposes

plugin/v1 text tones are `normal`, `error`, `subtle`, `accent`. **There is no
amber/warning tone.** The host change adding it is part of this work and is
listed in §12 — it is not an optional extra, because without it the amber grade
has no honest rendering in the panel.

## 9. Actions

Five tiers. A finding carries actions; tiers 1–4 escalate in privilege and in
irreversibility.

| Tier | Kind | Reversible | Needs root |
|---|---|---|---|
| 0 | `inform` — a fact | yes | no |
| 1 | `acknowledge` — record a deliberate decision | yes | no |
| 2 | `configure` — write a sysc-owned setting | yes | no |
| 3 | `launch` — guided flow: open a URL, print CLI steps, show a diff | yes | no |
| 4 | `elevate` — root helper with a closed verb set | **sometimes not** | **yes** |

### 9.1 Dismissal is a first-class action

Tier 1 exists because an audit that cannot be argued with is an audit that gets
deleted. A dismissed finding is stored with its reason and stops appearing, but
remains in `explain` output. Dismissing does not change any grade; it changes
what the user is shown.

### 9.2 Tier 4: the root helper

Modelled on `plugins/moonbit`, which is the existing in-repo precedent.

- `sudo -S -k -p "[privacybit-panel-password]" privacybit-helper apply`
- password written to **sudo's stdin only**, never to the helper's
- `defer clear(password)` after the call
- `-k` invalidates the sudo timestamp, so the password is asked every time
- own process group (`Setpgid`), bounded cancel grace, kill wait that lets sudo
  finish its cleanup before the kill lands
- two-phase confirm state machine: `Idle → Scanning → Review → Confirm → Working → Done/Error`, with `Auth` for the pending root run and `Back` restoring the return phase

### 9.3 The closed verb set — the important property

The helper **does not accept package names, paths, units, or argv from the
caller.** It accepts *intent*, and resolves it against its own embedded copy of
the catalogue.

```go
// Accepted. Catalogue ids only.
{"op": "apply-entry",        "entry": "brave",           "state": "off"}
{"op": "set-unit",           "unit":  "fwupd.service",  "state": "disabled"}
{"op": "plan-purge",         "entry": "google-chrome-stable"}
{"op": "purge-entry",        "entry": "google-chrome-stable", "token": "…"}

// Rejected before sudo is ever reached.
{"op": "purge-entry",  "entry": "../../etc/cron.d/x"}    // unknown id
{"op": "set-unit",     "unit":  "anything-at-all"}       // not in allowlist
{"op": "install-entry","entry": "brave-origin"}          // verb does not exist
```

Two rules are visible in that list before any of the machinery is explained.

**The helper never installs.** There is no install verb, because installing
introduces new code that will later execute — potentially as root, through a
unit the package ships — and that is the one direction a review step cannot
bound. Removal and unit toggling operate on what is already present and can be
bounded ahead of time. moonbit draws the same line and has drawn it for a long
time; its privileged verbs are scan, clean, orphan removal, docker prune and
schedule, with no `pacman -S` anywhere in the codebase. Installing an
alternative is therefore tier 3: the tool prints the exact command.

**The helper never touches disk encryption.** A bug during root encryption
produces an unbootable machine, and disk encryption is a security property
rather than a privacy one — which §1.1 already excludes from the audit's remit.

Full treatment, including the closed verb set, the token design and the sudo
invocation, is in
[`2026-10-20-privacybit-helper-design.md`](2026-10-20-privacybit-helper-design.md).

Two reasons this matters:

1. **Privilege escalation.** If the helper accepted a package name from plugin
   state, then anything able to influence that state would have arbitrary root
   with attacker-chosen argv. Resolving names from an embedded catalogue makes
   that class of bug structurally impossible.
2. **Accident.** `pacman -Rns` unattended deletes dependencies nobody asked it
   to delete. Any removal therefore requires the review phase to show the exact
   removal set and its reverse dependencies (`pactree -r`) before confirm.

The helper also runs with an embedded catalogue it controls, so a user-supplied
`catalog.d` override cannot widen the verb set into something dangerous.

## 10. Privacy of the tool itself

An audit tool that phones home is a joke, and the failure is worse than having
no tool because it would not be noticed.

- **No network calls. At all.** No update check, no crash reporting, no analytics,
  no version ping. `examine-udp`/`examine-tcp` in tests assert this.
- **Audit output is gitignored.** It fingerprints exactly what someone runs.
  The `.gitignore` ships with the rationale inline so nobody "helpfully" adds it
  back.
- **No reading outside the user's own scope.** No other users' processes, no
  `/etc/shadow`, no keys, no browser profile *contents* — only the specific
  preference keys needed for a grade.
- **Secret hygiene in the helper path** follows moonbit exactly: the sudo
  password never enters the helper's address space as a retained value.

This is enforced by test, not by convention.

## 11. Interfaces

### 11.1 CLI

```
privacybit audit [--json|--snapshot] [--categories ...] [--no-network] [--fresh]
privacybit explain <entry-id>          # why this grade, with sources
privacybit watch                        # live fast poll
privacybit tui                          # full TUI
privacybit list [--category ...]        # catalogue contents
privacybit catalog validate             # load + validate user fragments
```

`--json` emits an NDJSON event stream whose terminal `done` event carries the
snapshot document. `--snapshot` emits that document alone. See
[D2](2026-10-20-privacybit-cli-design.md) for the full contract.

**No IPC in this design.** There is no `serve` and no `panel` command. Every
current consumer — CLI, TUI, and the sysc-shell plugin — reaches the engine
directly, so a socket would have no reason to exist. It is reintroduced with
the scheduler in phase 9, as a Unix domain socket and never as HTTP.

### 11.2 Snapshot schema

Bounded: every list is capped, nothing is unbounded, so the panel can render
without a defensive pass.

```json
{
  "schema": 1,
  "generated_at": "2026-10-20T12:00:00Z",
  "duration_ms": 412,
  "coverage": { "package_manager": "pacman", "flatpak": true, "network": true },
  "overall": { "grade": "amber", "score": 62, "findings": 14, "high": 3 },
  "network": {
    "vpn": {
      "state": "connected", "client": "mullvad",
      "default_route_via_tunnel": true, "dns_contained": false,
      "ipv6_contained": false,
      "defects": ["dns-leak", "ipv6-bypass"]
    },
    "telemetry_connections": [
      { "pid": 1234, "exe": "/usr/bin/chrome", "endpoint": "google-analytics.com", "class": "analytics" }
    ]
  },
  "categories": [
    {
      "id": "browser", "name": "Browser", "grade": "red",
      "installed": [ { "id": "chrome", "name": "Google Chrome", "grade": "red",
                       "family": "chromium", "channel": "stable",
                       "source": "official", "maturity": "stable",
                       "caveats": ["Set as default browser"], "why": ["..."] } ],
      "expectations_met": [ "librewolf" ],
      "expectations_missing": [ "No content blocker installed" ]
    }
  ],
  "findings": [
    { "id": "vpn.dns-leak", "severity": "high", "category": "network",
      "title": "DNS queries are leaving the tunnel",
      "detail": "Resolver 192.168.1.1 is outside the Mullvad tunnel.",
      "alternatives": [],
      "actions": [ { "tier": 1, "kind": "acknowledge", "label": "Noted" },
                   { "tier": 3, "kind": "launch", "label": "Show fix" } ] }
  ]
}
```

`generated_at` is load-bearing: an audit is a point-in-time observation and a
VPN can connect immediately afterwards. The panel shows "as of" and does not
present stale data as current.

### 11.3 Plugin and host contract

- Plugin imports `github.com/Nomadcxx/privacybit`, runs `audit` on the slow
  tick, publishes `state.set("control_center", <snapshot JSON>)`.
- Host reads it with the exact `ccCalendarSnapshotFor` pattern: absent plugin,
  absent key, or malformed JSON all mean "show what we have", never an error
  page.
- Panel actions route through `activateControlCentre` as new verbs
  (`privacybit:refresh`, `privacybit:run-action`).

Because the plugin holds a library reference rather than spawning the CLI,
**no IPC and no privileged transport are involved in reading**. moonbit needs
`sudo moonbit panel` because cleaning needs root; an audit does not, and
requiring a password merely to render a panel would be a defect.

### 11.4 Remediation must bind to a reviewed audit

A remediation records the `generated_at` of the audit the user acted on, and
refuses to run if the current machine state has moved past it. moonbit enforces
this with `ScannedAt` for exactly the same reason: a scheduled scan can replace
the cache between review and action, and without the binding the operation acts
on state the user never saw. The same hazard applies to a privacy audit whose
findings have since changed.

## 12. Host changes required in sysc-shell

1. **`ccPrivacy` renders categories.** A second card under the existing capture
   card, driven by the snapshot. The existing capture panel is untouched.
2. **Snapshot reader.** `ccPrivacySnapshotFor(r *Registry)` mirroring
   `ccCalendarSnapshotFor`.
3. **New text tone `warning`.** Added to plugin/v1 and the host renderer, so
   amber is honestly renderable (§8.1).
4. **Action verbs** in `activateControlCentre`.
5. **Optional:** the bar widget badge could reflect overall grade. Deferred —
   the widget currently reports capture state, and overloading it risks
   conflating "something is recording" with "your setup is mediocre".

Page budget is comfortable: the Control Centre body resolves to ~596 × 480 in
a `KindScroll` viewport, and the existing privacy page is currently a single
small card.

## 13. Testing strategy

| Layer | Approach |
|---|---|
| Catalogue | Golden-file tests per category. Every entry parses; ids unique; `alternatives` resolve; every red entry has at least one alternative. |
| Detection | Fixture-driven per package manager. Fake `pacman`/`flatpak`/`dpkg` scripts emitting recorded output, mirroring moonbit's fake-`sudo` pattern. |
| Matching | Table tests for specificity tie-breaks and the corroboration rule, including a deliberate name-collision case. |
| Monotone rule | Property test: for any catalogue entry and any derived signal set, the resulting grade is `>= ` the catalogue grade in risk order. |
| Network | Fixture `ss` and `resolvectl` output covering tunnel-up-but-not-routing, DNS leak, IPv6 escape, and no-VPN. |
| Helper | Fake `sudo` mimicking prompt, re-prompt, wrong password, and "not in the sudoers file"; argv-allowlist tests asserting rejection before sudo; process-group orphan tests. |
| Snapshot bounds | Test asserting no list exceeds its cap under a synthetic 10,000-package input. |
| No network | Assert no outbound socket is opened during any audit path. |
| Rendering | `shelllint.Tree` against the declared panel size, so a tree that validates but refuses at render is caught in CI. |

## 14. Known gaps in this design

Recorded so they are decisions rather than surprises:

- **No `snap` or AppImage index scanning.** Neither tool is installed here, so
  neither could be designed against real output. Partially mitigated by the
  `.desktop` scan (§6.6).
- **Network visibility inside flatpak is degraded** and is reported as `unknown`
  rather than graded (§6.4). This is a permanent limitation, not a bug to be
  fixed later.
- **Flatpak override enumeration is O(applications)** because flatpak offers no
  bulk listing command.
- **Locale and translated metadata** are not parsed.
- **Single-user scope.** Other users' processes and per-user installs are not
  audited. A `--all-users` mode would need root and is out of scope.
- **Containerised and nested installs** (flatpak-in-flatpak, systemd-nspawn,
  WSL) are not detected.
- **Endpoint classification is only as good as the endpoint list**, which is
  itself a catalogue and will drift. It is treated as evidence, not proof.
- **The catalogue encodes opinions.** The copy must read as opinion, with
  reasoning shown per entry and `explain` available for any grade.
- **False negatives on renamed packages** are expected and acceptable under
  §0's priority ordering.
- **Disk encryption is deliberately out of scope, in both directions.** Not
  audited (it is a security property, and §1.1 separates the two) and never
  acted on (a botched root encryption bricks the machine, and D4 §3.4 removes
  it from the helper verb set). Recorded here because "should my audit tell me
  about disk encryption" is an obvious question, and the answer is a decision
  rather than an oversight.
- **No remediation of the operating system itself** — kernel, bootloader,
  initramfs.

## 15. Delivery phases

Each phase ends with something runnable.

| Phase | Deliverable |
|---|---|
| 0 | Repo, this document, README |
| 1 | Entry model, catalogue format, loader, validator, `list`, `explain` |
| 2 | Detection: D6 source interface, pacman backend, universal sources, coverage reporting → **first useful audit output** |
| 3 | Matching and grading |
| 4 | Findings and actions, tiers 0–3, D6 §8 command rendering |
| 5 | Network and VPN checks |
| 6 | CLI: `audit`, `--json`, exit codes |
| 7 | TUI (D3) |
| 8 | Root helper (D4), tier 4 |
| 9 | Scheduler, daemon, event watcher (D7) |
| 10 | sysc-plugins `plugins/privacy` |
| 11 | sysc-shell host wiring + `warning` tone |

Phase 2 was reordered ahead of phase 3 deliberately. Grading cannot be tested
without something to grade, and D6 §3.2's three-state invariant needs the
capability matrix in place before the first grade is ever computed — retrofitting
it means auditing every signal already emitted.

**Remaining backends are not a phase gate.** D6 §4's dnf, zypper, apk and xbps
backends land after the first useful audit, each one a small additive change
behind an interface that already exists. §12 defers nix and snap with reasons.
An unsupported distribution still audits through D6 §5's universal sources with
coverage reported honestly.

Phases 1–3 are the smallest thing that answers the original question and should
be built first. Nothing in phase 4 onward blocks it.