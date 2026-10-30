# privacybit — D2: CLI surface

**Date:** 2026-10-20
**Status:** approved direction, pre-implementation
**Depends on:** [D1 — core engine](2026-10-20-privacybit-design.md)
**Reference:** `Nomadcxx/moonbit` `internal/cli/` (read at `origin/main` `5948bd0`)

---

## 1. Scope

**In:** the command tree, flags, the `--json` contract, exit codes, config,
logging, human output, cancellation, snapshot versioning.

**Out:** TUI screens (D3), root-helper internals (D4), plugin and host wiring
(D5).

D2's real deliverable is **the snapshot contract**, because D3, D5 and every
future consumer are downstream of it. Everything else in D2 is conventional.

## 2. What is deliberately absent

### 2.1 No `serve`, no `panel`

moonbit has both because its daemon is its scheduler, and the desktop panel
needs to reach a resident process that owns the scan cache and the sudo path.

privacybit has no such consumer yet:

| Consumer | How it reaches the engine |
|---|---|
| CLI | Direct call |
| TUI (D3) | Direct library import |
| sysc-shell plugin (D5) | Runs the `privacybit` binary and reads its JSON output |
| Scripts | `--json` |

Nothing needs IPC, so nothing gets IPC. `serve` and `panel` are reintroduced in
phase 9 with the scheduler, when there is a resident process with a reason to
exist.

The snapshot is specified as a **document**, not only as an event payload, so
that adding a transport later is additive rather than breaking.

### 2.2 No HTTP

When a transport arrives it will be a Unix domain socket, not TCP. A local
socket needs no port allocation, no TLS story, and no firewall. If the socket
is ever exposed to other local users — see §8 — it is a disclosure problem, and
HTTP would make it a remote one.

## 3. Command tree

```
privacybit
  audit                  Run the audit. The main command.
  explain <entry-id>     Why an entry has its grade, and what would change it.
  list                   Catalogue contents, filtered.
  catalog validate       Load shipped + user fragments, report problems.
  catalog path           Print every catalogue source and load order.
  watch                  Live fast poll, human output.
  tui                    Full TUI.                        [D3]
  version                Version, commit, catalogue revision.
  completion             Shell completions.
```

`version` is read-only. A source build reports `devel` and its Go-recorded
source commit. A packaged build reports the package version and commit injected
by its build recipe. The command loads the effective catalogue, honoring
`--user-catalog`, and reports its revision without running a scan.

`audit` is the verb rather than moonbit's `scan`, because it does more than
scan: it grades, matches a catalogue, and derives signals.

`tui` ships in D2 as a stub that exits non-zero with an explicit "not
implemented yet" rather than a silent no-op. A stub that lies is worse than a
stub that admits itself.

## 4. `audit` flags

| Flag | Default | Meaning |
|---|---|---|
| `--json` | off | NDJSON event stream on stdout; human output suppressed |
| `--snapshot` | off | Emit the snapshot document once and exit. No progress events |
| `--categories <ids>` | all | Restrict to these category ids |
| `--exclude-categories <ids>` | none | Subtract these |
| `--severity <level>` | `low` | Minimum finding severity to report |
| `--network` | on | `--no-network` skips live connection and VPN checks |
| `--fresh` | off | Ignore the package cache and re-scan |
| `--fail-on <grade>` | none | Exit 10 if the overall grade is at or worse than this |
| `--no-color` | auto | See §9 |
| `-v`, `--verbose` | off | Progress detail on stderr |
| `--config <path>` | XDG | Explicit config file |

### 4.1 `--fast` is not a flag

The obvious flag is `--fast`, and it is the wrong one: "fast" describes the
result, not the intent. The intent is *staleness tolerance*. So there is no
`--fast`; there is `--fresh`, and its absence means "reuse the cached package
inventory, refresh everything volatile".

A cached inventory plus a live network check is the normal panel case and
completes in well under a second. A full re-scan is the install-time case.

### 4.2 `--severity` is a floor, not a filter

Filtering findings out of a report invites the failure where the tool is asked
about the VPN and answers with silence. `--severity high` must still emit the
category rollup, the overall grade, and the coverage block — a summary that
disappears under a filter is a lie about completeness.

## 5. The `--json` contract

Inherited from moonbit, unchanged in shape, because the reasoning is sound and
independently arrived at.

### 5.1 Newline-delimited events

One JSON object per line on stdout. Every event carries `t`, its type.

```
{"t":"start","schema":1,"mode":"full"}
{"t":"progress","stage":"packages","pct":40}
{"t":"category","id":"browser","grade":"red","installed":4}
{"t":"finding","id":"vpn.dns-leak","severity":"high","title":"DNS queries are leaving the tunnel"}
{"t":"done","grade":"red","score":62,"snapshot":{ … }}
```

### 5.2 The rules

| Rule | Why |
|---|---|
| JSON on stdout, human on stderr | `privacybit audit --json \| jq` must work. |
| Exactly one terminal event | A consumer can `tail -1` and know it is complete. Terminals are `done`, `cancelled`, `error`. |
| Progress throttled to 250 ms | A package scan is seconds long. Unthrottled it floods the consumer and the terminal. First event always passes. |
| Cancellation = close stdin | A panel has no TTY and cannot signal a root child. EOF on the request reader cancels. |
| `--snapshot` suppresses progress | One document, no stream. For storage and comparison. |

### 5.3 The snapshot rides on `done`

There is no second output format. The terminal `done` event carries
`snapshot`, the bounded D1 §11.2 document. `--snapshot` is sugar for "emit just
that one event".

One contract, so there is nothing to keep in sync and nothing to drift.

### 5.4 Bounded, always

Every list is capped. `installed` truncates with an explicit
`installed_truncated: true` and `installed_total: 412` — never silently. A
consumer must be able to tell "you have four browsers" from "I only showed you
four browsers", because confusing them is how a privacy tool talks someone into
thinking they are fine.

Schema 1 app records may also include a bounded `origins` list with the
observed source name and whether that source is marked official. The list has
shown, total, and truncation fields. Origin is provenance context and never
changes a grade. Consumers accept schema-1 app records that omit these fields.

## 6. Exit codes

The trap: exit code zero meaning "your machine is fine" conflates **did the tool
work** with **is the system okay**. A script that does `privacybit audit && echo
clear` is then asserting something the tool never established.

So exit status reports tool health, and policy failure is opt-in and
unambiguous:

| Code | Meaning |
|---|---|
| 0 | The audit completed. Says nothing about the grade. |
| 1 | Usage error. Bad flag, unknown category id. |
| 2 | The audit could not complete. No coverage at all, or a hard failure. |
| 3 | Internal error. A bug. |
| 10 | Policy failure. `--fail-on` matched. |

`--fail-on red` → exit 10 on red. `--fail-on amber` → exit 10 on amber or red.

```sh
# health check
privacybit audit --quiet || alert "privacy audit is broken"

# policy gate
privacybit audit --quiet --fail-on red || block
```

The first script cannot fire on a red machine. The second cannot fire because
the tool crashed. They are different failures and get different codes.

## 7. Config

`~/.config/privacybit/config.toml`, `BurntSushi/toml` as moonbit uses.

Precedence, highest first: **flags → `PRIVACYBIT_*` env → config file →
defaults.**

```toml
[general]
categories = ["browser", "vpn", "passwords"]
severity = "medium"
fail_on = "red"
no_color = false

[network]
check = true
# Refuse to report "no telemetry observed" as clean when a flatpak
# unshares its network namespace. See D1 §6.4.
attribute_unshare = "unknown"

[catalog]
dirs = ["~/.config/privacybit/catalog.d"]
allow_user_grade_override = true

[state]
dismissals = "~/.local/state/privacybit/dismissals.toml"
cache = "~/.cache/privacybit/inventory.json"
```

`allow_user_grade_override` is `true` because overriding a grade is a
legitimate user decision, and the UI shows the catalogue grade beside the
override so the effect is never silent. It is exposed as a flag because
`false` is the right setting for the sysc-shell panel, where a user-supplied
fragment must not be able to make the audit lie.

## 8. `watch`

Live fast poll in human output. The TUI is D3; `watch` is the zero-dependency
fallback for people over SSH, inside a terminal multiplexer, or on a machine
where a Bubble Tea binary is unwelcome.

- Polls volatile state on an interval; does not re-run the package inventory
- `--interval <dur>` (default 2s)
- One terminal line rewritten in place, degrading to scrolling output when not
  a TTY
- Honours `NO_COLOR` and honours the same three-channel grade rendering
- `Ctrl-C` exits cleanly and emits a terminal `cancelled` event in `--json`

## 9. Human output

lipgloss, matching moonbit's `styles.go`.

| Rule | Detail |
|---|---|
| `NO_COLOR` honoured | https://no-color.org |
| `--no-color` | Same effect as the env var |
| Not a TTY | Plain text, no ANSI, no in-place rewriting |
| Width detection | Truncate with `…`; never wrap a grade word |
| stdout vs stderr | Findings to stdout; progress and diagnostics to stderr |

### 9.1 Three-channel grade rendering

D1 §8 requires colour, glyph shape and word. In the CLI the glyph must survive
a plain terminal and a dumb pipe:

| Grade | Colour | Glyph | Word |
|---|---|---|---|
| `green` | green | `●` | Good |
| `amber` | amber | `▲` | Review |
| `red` | red | `■` | Risky |
| `unknown` | dim | `?` | Unrated |

ASCII fallback when the output encoding cannot represent the glyphs:
`green → o`, `amber → ^`, `red → x`. The word is always present, so the
fallback loses nothing semantic.

## 10. Cancellation

Inherited unchanged from moonbit.

- `--json`: `cancelOnEOF(os.Stdin)` — EOF or close on stdin cancels the run
- human mode: `SIGINT` → graceful cancel, terminal `cancelled` event
- in-flight `exec.CommandContext` children are killed with `Cancel` first, then
  `Kill` after a grace period, so `pacman` gets to unwind rather than being
  orphaned
- a cancelled run is **not** a failed run: exit 0 in JSON mode, matching
  moonbit's `emitTerminal`

That last one is worth stating because it looks like a bug and is not. A user
pressing Ctrl-C is not an error condition, and treating it as one trains people
to ignore error output.

## 11. Snapshot versioning

```json
{ "schema": 1, "generated_at": "…", … }
```

| Rule | Detail |
|---|---|
| Additive changes | Bump minor only. New optional fields are always safe. |
| Breaking changes | Bump major. Remove or retype a field. |
| Consumer rule | A consumer **must** reject a snapshot whose major is higher than it knows. |
| Rejection behaviour | Reject to *no data*, never to a partial parse. |
| Catalogue revision | Every snapshot carries `catalog_rev` — a hash of the effective catalogue. |

The consumer rule matters most. A panel that parses a future snapshot with
today's struct does not fail — it silently reads zero for every unknown field
and reports `browser: no browsers found`. That is the under-grade, arriving
through the back door. Rejecting to no-data makes the panel say "audit
unavailable", which is the truth.

`catalog_rev` exists so a stored snapshot can be traced to the table that
produced it. Without it, "why did my grade change?" is unanswerable.

## 12. Errors

Typed errors, following moonbit's `internal/errors` (which is on the
near-verbatim copy list).

Errors are written **with a suggested next action**, not just a cause. moonbit
already does this well and it is worth preserving verbatim in spirit:

```
pacman: database not initialised
  → run `sudo pacman -Sy` first, or pass --no-network to skip package checks
```

An error message that only states the cause forces the user to guess the fix.

## 13. Testing

| Layer | Approach |
|---|---|
| Command tree | Table test over `[]string` → expected exit code and stdout sentinel. |
| `--json` contract | Every command path asserts exactly one terminal event, first event passes the throttle, stdout is pure JSON. |
| Throttle | Synthetic slow audit; assert event count is bounded, not proportional to work. |
| Cancellation | Close stdin mid-run; assert terminal `cancelled` and that no orphaned child remains. |
| Exit codes | Assert `audit` returns 0 on a red machine, and 10 only under `--fail-on`. |
| Snapshot bounds | 10,000-package input; assert caps and `installed_truncated`. |
| Schema rejection | Feed a `schema: 2` snapshot to a schema-1 consumer; assert no-data, not partial parse. |
| Colour | `NO_COLOR`, `--no-color`, non-TTY, and ASCII-fallback output all render grade + glyph + word. |
| Config precedence | Flag beats env beats file beats default, asserted per key. |

## 14. Open questions for implementation

1. **Does `watch` earn its place, or is `audit --snapshot` in a loop enough?**
   Leaning yes — the in-place rewrite is the whole value and a shell loop
   cannot do that.
2. **`explain` output format.** Plain text, or a second JSON event? Leaning
   plain text plus `--json`; the grade reasoning is prose and does not want to
   be a struct.
3. **Should `list` support `--grade red --alternatives`?** A catalogue dump
   filtered to "red entries and what to use instead" is the table most people
   want to read, and it doubles as catalogue-authoring workflow.
