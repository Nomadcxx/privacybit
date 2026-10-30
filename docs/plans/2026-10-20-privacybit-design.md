# privacybit — design

**Date:** 2026-10-20
**Status:** approved; implementation updates recorded through 2026-10-23
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
                    ├── CLI and TUI
                    ├── user daemon and timer
                    ├── privacybit-helper (root, closed verb set)
                    └── internal audit engine
                              ▲
                              │ CLI and TUI call the engine directly
                              │
                    org.sysc.privacy plugin
                       │             │
                       │             ├── bar widget → plugin audit panel
                       │             └── `Scan now` → privacybit binary
                       └── bounded state → Control Centre summary
                                             └── `Open full audit` → plugin panel
```

### 2.1 Build order

The dependency edge runs one way, so the order is forced:

1. **privacybit** — engine, CLI, catalogue, helper. Independently useful.
2. **plugins/privacy** — runs the privacybit binary on a tick, publishes its
   snapshot, and owns the bar widget and detailed audit panel.
3. **sysc-shell** — preserves the capture activity card, renders a compact
   snapshot summary, and routes `Open full audit` to the plugin panel.

The logic lives in a project that can stand alone and can be tested without the
shell. The CLI and TUI call the internal engine directly. The plugin shells out
to the same CLI binary, following the existing moonbit plugin pattern.

### 2.2 Why the plugin panel is separate from the Control Centre

The Control Centre is host-rendered: `ccPage` in
`internal/shell/popout_controlcenter.go` switches over host functions and does
not render plugin views inside a Control Centre section. The shell can open a
separate plugin-owned panel through the existing `panels` capability. The bar
widget and the Control Centre's `Open full audit` action both open that panel.

The plugin also publishes bounded JSON state for the compact summary. The
calendar plugin (`org.sysc.calendar` → state key `control_center`) is the
existing state-publication precedent. The plugin shells out to the same
privacybit binary as the CLI and TUI, so every surface uses one audit engine.

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
| Packaging origin | official repo vs AUR vs third-party flatpak remote vs `~/.local` vs AppImage | Context only. Report its source and official status; never change a grade. |
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

### 3.7 Category and overall grades

Use the worst evidence at each level; do not average away a serious finding.
An entry starts at its effective catalogue grade and grade-bearing measured
signals may only push it toward more risk. A category takes the worst grade of its known
installed entries and unmet expectations. A missing high-severity expectation
is `red`; a missing medium- or low-severity expectation is at least `amber`, so
an unmet expectation never reads as a pass. An expectation is met when any of
its `any_of` names is present in the inventory, including an ungraded app name.

Packaging origin is provenance context. Reports may show the source name and
whether the source is marked official, but origin never changes an application,
category, or overall grade, including when an origin signal is contradicted.
The owner confirmed this rule in issue #22.

The Phase 3 grader moves an entry one level worse when any grade-bearing
measured signal is `Contradicted` (`green` → `amber`, `amber` → `red`; `red`
stays `red`). A contradiction is evidence to review, but does not by itself
establish a red privacy posture. Zero and `Unavailable` signals do not change
the grade.

An expectation that is not met can be graded only when every detect type on
each catalogue entry named by `any_of` has a ready source. An ungraded `any_of`
name is treated as a package name and requires package coverage. If a missing
expectation cannot be judged because a source is unavailable, its category is
`unknown`. Other incomplete source coverage is reported as a caveat and leaves
the category grade unchanged.

An `unknown` category grade propagates to the overall grade: the system has not
been fully judged. Where categories can be judged, the overall grade is the
worst category grade. Partial coverage that does not prevent a category
judgement is shown as a caveat and does not improve or worsen its grade. An
unmatched application remains `unknown` in the inventory; it does not acquire a
category or alter a known entry's grade.

This roll-up is deliberately conservative and simple. The score is a separate
progress indicator; it does not determine the overall grade.

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

Resolution: match each observation by exact detect type and name. The catalogue
validator rejects duplicate keys, so one observation cannot select between two
entries.

1. Exact matches on any high-confidence source (`package`, `flatpak`) win.
2. `binary` and `desktop` are corroboration, not primary evidence — a binary
   named `signal` is not necessarily Signal Desktop.
3. Detect rules do not support substring matching. A source-native name either
   matches one exact catalogue key or stays unrated.
4. If future matching evidence gives one observation conflicting identities,
   report it as `unknown` and retain the conflicting evidence for diagnosis.

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
worth knowing. D1 specifies five checks; v1 does not implement all of them.

**Resolved permissions, with Flatpak applying its own overrides.** For every
installed app copy, the detector calls `flatpak info --show-permissions` with
that copy's installation, application id, branch, and architecture. Flatpak
loads the deployed app context and merges the applicable global and per-app
overrides for the current user and installation. privacybit must use this
resolved output rather than reconstruct override precedence or key paths.
Permission-query failures leave that copy's sandbox grade unknown.

The command output also contains environment values and may contain other
permission groups. The audit never reports the raw output: it grades only its
recognized context keys, ignores environment values, and leaves any
unclassified permission evidence unknown.

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

The owner-approved generic risk bands are:

| Grade | Evidence |
|---|---|
| Red | Host filesystem access, all devices, or shared networking. |
| Amber | Bounded broad access such as home files, host configuration, X11, or USB. |
| Green | Complete, classified permission evidence without a red or amber grant. |
| Unknown | Missing output, a failed query, invalid output, or an unclassified permission. |

The sandbox grade remains separate from the catalogue grade. The overall grade
uses worst evidence across categories and sandbox copies. An unclassified
permission makes that app's sandbox grade unknown, even when another grant is
known red; the recognized grant remains visible as evidence. Unknown sandbox
grades propagate to overall.

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

Use resolver ownership mode to select DNS evidence. In `foreign` mode, use the
nameservers in `/etc/resolv.conf` and check their routes; `resolvectl dns` may
report per-link servers that do not own the resolver path used by applications.
In the recognized `stub`, `uplink`, and `static` modes, use `resolvectl`'s
active DNS servers and default-route state. If the mode is missing or
unrecognized, or its status cannot be read for another reason, report the DNS
check as unknown. When `resolvectl` is unavailable, or its status confirms the
systemd-resolved IPC socket is missing, `/etc/resolv.conf` is the only resolver
evidence and its nameservers may be checked directly. If the active resolver
or its route cannot be identified, report the DNS check as unknown.

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

### 8.1 Privacy grades in sysc-shell

`internal/ui` defines `ToneActivity`, which paints the theme's Tertiary role.
`plugin/v1` accepts `ToneWarning` (`warning`), and `internal/plugin` maps it to
`ui.ToneActivity`. The privacy summary and plugin panel do not use this wire
tone for amber; it appeared green on the laptop palette. D5 §11.2 defines the
grade-specific theme fills, icons, and words. The plugin declares `panels` and
`state` at protocol minor 8; no protocol or renderer change was needed.

The earlier D5 draft proposed an `activity` wire value. The host already had
`warning`, so the plugin keeps the existing wire mapping for other views.

The grade word remains visible without colour, and its icon and fill provide
additional visual cues.

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

### 9.4 Inventory findings and available actions

The Phase 4 builder emits findings for known installed entries graded amber or
red and for judged, unmet expectations. Installed red entries map to high
severity; installed amber entries map to medium severity. An unmet expectation
keeps its authored severity, grade, and explanation. Unmatched software and
unjudgeable expectations stay in inventory and coverage.

Each finding carries a tier 0 information action. A catalogue alternative gets
a tier 3 action only when D6 can render a package command or an HTTPS upstream
URL for it. privacybit returns that instruction for the user to review; it does
not run it. A known Flatpak installed at both system and user scope produces a
low-severity informational finding without changing the app or category grade.

The builder omits tier 1 until privacybit can store dismissals. It omits tier 2
until the catalogue can name a specific reversible setting. Consumers must not
show either tier as an available action until those handlers exist.

### 9.2 Tier 4: the root helper

Modelled on `plugins/moonbit`, which is the existing in-repo precedent.

- `privacybit purge <entry-id>` starts `sudo -k -- privacybit-helper`
- sudo reads authentication on the controlling terminal; privacybit never reads
  or retains the password
- the helper's bounded JSONL protocol uses sudo's stdin, separate from the
  controlling-terminal prompt
- the CLI displays the removal plan and requires the exact entry id before it
  sends the one-use purge request
- the helper owns a private process group and uses bounded shutdown that lets
  sudo finish its cleanup before the final kill

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
  "available": true,
  "generated_at": "2026-10-20T12:00:00Z",
  "duration_ms": 412,
  "catalog_rev": "2026.10.1",
  "entry_point": "privacybit",
  "overall": { "grade": "red", "score": 62, "findings": 1, "high": 1 },
  "network": {
    "vpn": {
      "state": "connected", "client": "mullvad",
      "default_route_via_tunnel": true, "dns_contained": false,
      "ipv6_contained": true,
      "defects": [{"id":"vpn.dns-leak", "title":"DNS leak",
        "detail":"The configured resolver is outside the tunnel."}]
    },
    "telemetry_connections_unknown": true
  },
  "categories": [
    {
      "id": "browser", "name": "Browser", "grade": "red",
      "installed": [ { "id": "chrome", "name": "Google Chrome", "grade": "red",
                       "family": "chromium", "scope": "system",
                       "sources": ["pacman"], "why": ["Catalogue match"] } ],
      "installed_shown": 1, "installed_total": 1,
      "installed_truncated": false
    }
  ],
  "findings": [
    { "id": "vpn.dns-leak", "severity": "high", "grade": "red",
      "category": "network",
      "title": "DNS queries are leaving the tunnel",
      "detail": "Resolver 192.168.1.1 is outside the Mullvad tunnel.",
      "alternatives": [],
      "actions": [ { "tier": 1, "kind": "acknowledge", "label": "Noted" },
                   { "tier": 3, "kind": "launch", "label": "Show fix" } ] }
  ],
  "findings_total": 1, "findings_shown": 1, "findings_truncated": false,
  "coverage": [ { "name": "pacman", "type": "package",
                   "ready": true, "observed": 2882 } ]
}
```

`generated_at` is load-bearing: an audit is a point-in-time observation and a
VPN can connect immediately afterwards. The Control Centre summary and plugin
panel show currentness and do not present stale data as current.

### 11.3 Plugin and host contract

- Plugin runs `privacybit audit --snapshot` on its five-minute tick and
  publishes the bounded projection under state key `control_center`.
- Host reads it with the `ccCalendarSnapshotFor` pattern. Missing or malformed
  state becomes an explicit unavailable summary, never a clean result.
- The bar widget and `Open full audit` route to the declared plugin panel. That
  panel owns `Scan now`; the host does not run scans or remediation verbs.

Because the plugin runs the same unprivileged binary as the CLI, **no socket
IPC or privileged transport is involved in reading**. moonbit needs `sudo
moonbit panel` because cleaning needs root; an audit does not, and requiring a
password merely to render a panel would be a defect.

### 11.4 Remediation must bind to a reviewed audit

A remediation records the `generated_at` of the audit the user acted on, and
refuses to run if the current machine state has moved past it. moonbit enforces
this with `ScannedAt` for exactly the same reason: a scheduled scan can replace
the cache between review and action, and without the binding the operation acts
on state the user never saw. The same hazard applies to a privacy audit whose
findings have since changed.

## 12. Host integration

1. **Compact `ccPrivacy` summary.** Show overall grade and currentness below
   the existing capture activity card. `Open full audit` opens the plugin panel.
2. **Snapshot reader.** `ccPrivacySnapshotFor(r *Registry)` mirroring
   `ccCalendarSnapshotFor`; reject malformed or oversized state and show an
   explicit unavailable result.
3. **Grade treatment.** Use a visible word and theme-styled icon/fill from D5
   §11.2. Do not use `ToneWarning` for amber because its tertiary color appeared
   green on the laptop palette. Keep `ui.Tone` additions append-only.
4. **Panel route.** Route `cc:privacy-open` through the existing plugin panel
   API. The host does not launch audits or run remediation actions.
5. **Plugin bar entry.** The user-placeable `org.sysc.privacy` bar widget shows
   the overall grade and opens the same audit panel; it remains separate from
   capture activity.

Page budget is comfortable: the Control Centre body resolves to ~596 × 480 in
a `KindScroll` viewport. The capture card and compact summary fit there; the
620 × 720 plugin panel holds the detailed audit view.

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
| Rendering | Check the compact Control Centre summary and plugin panel against their declared surfaces; exercise the plugin's scrollable view at its bounded panel size. |

## 14. Known gaps in this design

Recorded so they are decisions rather than surprises:

- **No `snap` or AppImage index scanning.** Neither tool is installed here, so
  neither could be designed against real output. Partially mitigated by the
  `.desktop` scan (§6.6).
- **Network visibility inside flatpak is degraded** and is reported as `unknown`
  rather than graded (§6.4). This is a permanent limitation, not a bug to be
  fixed later.
- **Flatpak permission inspection is O(installed app copies)** because the CLI
  exposes resolved permissions per app ref rather than as a bulk inventory.
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
| 11 | sysc-shell compact summary and route to the plugin panel |
| 12 | Plugin bar widget, detailed audit panel, read-only `Scan now`, and GPL-3.0-or-later licensing |

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
