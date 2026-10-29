# privacybit — D4: root helper

**Date:** 2026-10-20
**Status:** corrected implementation design
**Depends on:** [D1](2026-10-20-privacybit-design.md) (§9 actions, §11.4 review binding), [D2](2026-10-20-privacybit-cli-design.md)
**Reference:** `Nomadcxx/sysc-plugins/plugins/moonbit/runner.go` (288 lines, read in full)
**Security-sensitive.** Review this document before writing any code in it.

---

## 1. Scope

`privacybit-helper` is a headless, short-lived privileged session. It performs
tier 4 actions and nothing else. It is not a daemon, it has no UI, and it
never accepts a name it did not already know. A session serves a purge plan and
its confirmation, then exits; its review token never reaches disk.

**In:** verb set, validation order, sudo invocation, review binding, audit
logging, failure modes.

**Out:** the TUI's confirm screens (D3), the panel's presentation (D5).

## 2. Two binaries, not one

moonbit has two privileged binaries doing structurally different jobs, and the
split is worth copying:

| Binary | Shape | Job |
|---|---|---|
| `moonbit panel` | headless, one JSON request per run on stdin, runs under `sudo -S` | performs actions for a caller |
| `moonbit-installer` | Bubble Tea TUI, run as `sudo ./moonbit-installer` | installs moonbit itself: `make build`, `/usr/local/bin`, desktop entry, systemd units, schedule |

privacybit gets the same pair:

| Binary | Job |
|---|---|
| `privacybit-helper` | perform tier 4 actions on behalf of a caller |
| `privacybit-installer` | install embedded user units and enable watcher or timer mode |

The installer does not install the binaries. It writes the embedded user
systemd units under the user's configuration directory and enables exactly one
mode. It is not part of D4's privileged security surface.

## 3. The closed verb set

The single most important property in this document. **The helper never accepts
a package name, a path, a unit name, or an argument vector. It accepts intent,
and resolves it against its own embedded catalogue.**

```jsonc
// accepted — catalogue ids and closed-set enums only
{"op": "set-unit",      "unit": "fwupd.service", "state": "disabled"}
{"op": "plan-purge",    "entry": "google-chrome-stable"}
{"op": "purge-entry",   "entry": "google-chrome-stable", "token": "…"}
```

```jsonc
// rejected before any side effect, and before sudo would need to exist
{"op": "purge-entry",   "entry": "../../etc/cron.d/x"}   // not a catalogue id
{"op": "set-unit",      "unit": "anything-at-all"}      // not in the allowlist
{"op": "purge-entry",   "package": "evil"}              // field does not exist
{"op": "run",           "argv": ["sh", "-c", "…"]}      // verb does not exist
{"op": "install-entry", "entry": "brave-origin"}        // verb does not exist
```

### 3.1 Why resolution must be internal

If the helper accepted a package name from the request, then anything able to
influence that request — the panel, a compromised plugin, a stale cache file —
would hold arbitrary root with attacker-chosen argv. That is not a bug class to
be avoided by careful coding; it is a bug class to be made structurally
impossible.

Resolving names from an embedded table removes it. The helper knows what
`brave-origin` installs because *it* says so, in a table it ships, reviewed
alongside the rest of the source.

### 3.2 The verb set

| Verb | Privileged | Effect |
|---|---|---|
| `ping` | no | Liveness. The `ready` handshake. |
| `status` | no | Helper build version, embedded catalogue revision. |
| `apply-entry` | — | Deferred until the catalogue models a concrete reversible setting. |
| `set-unit` | yes | Enable/disable an allowlisted systemd unit. |
| `plan-purge` | yes | Compute a removal plan and issue a review token. Read-only. |
| `purge-entry` | yes | Execute a removal, given a valid token. |

Note what is *not* in that table. There is no install verb and no disk
encryption verb. See §3.4.

`apply-entry` is not an accepted v1 verb. The catalogue has no modeled
preference-backed setting yet; a generic toggle would not be a closed set.

Packaged builds report the package version. The AUR build injects `pkgver`;
direct source builds report `devel`.

### 3.3 Unit allowlist

`set-unit` accepts only units the helper ships. The v1 allowlist is:

```
fwupd-refresh.timer
```

Disabling this timer stops metadata refresh while leaving manual firmware
updates available. Disabling `fwupd.service` also stops firmware updates;
`upower.service` and `PackageKit.service` have not been shown to be
privacy-only switches. They stay out until their effects are reviewed. Growing
the list is a source change and a review, never a caller decision.

### 3.4 The direction rule

**The privileged path removes and configures. It never introduces.**

This is not an aesthetic preference. The risk of an operation is not symmetric in
the direction it moves:

- **Removal** operates on software already present. Its blast radius is
  knowable before it runs — that is exactly what `plan-purge` computes — and it
  can be reviewed.
- **Unit toggling** is bounded by a shipped allowlist of names that were chosen
  because they are the ones that phone home on a default install.
- **Installation** introduces new code that will execute later, and potentially
  as root, through a unit the package ships. It creates a future privilege
  boundary crossing rather than closing one, and that future crossing is not
  something the review step can bound.

moonbit draws the same line and has drawn it for a long time: its privileged
verbs are scan, clean, orphan removal, docker prune and schedule. There is no
`pacman -S` anywhere in the codebase, and none is added by this design.

So installing an alternative is a **tier 3** action, not a tier 4 one. The
helper emits the exact command and the user runs it:

```console
$ privacybit apply --entry google-chrome-stable
google-chrome-stable is graded red. A greener alternative is brave-origin.

  pacman -S brave-origin

This tool will not install it for you. Installing needs root, and introducing
new software under root is the one direction this helper will not move.
```

One copy-paste, and the largest privileged surface in the design disappears.

**The same reasoning removes disk encryption entirely.** Encrypting a root
partition is irreversible-ish in the worst case: a helper bug can produce an
unbootable machine, and no review dialog recovers it. It is also a *security*
property rather than a privacy one, so D1 §1.1 already places it outside the
audit's remit. Removing it costs the design nothing real, because privacybit
never claimed to cover it.

### 3.5 `report-root` — deferred

Running the audit as root sees other users' processes and `/proc` entries. That
conflicts with D1 §14, which scopes the audit to the current user. The helper
does not expose `report-root` in v1. Any wider audit needs a separate owner
decision and a design for disclosure across user accounts.

## 4. User fragments cannot define root operations

D1 §7 lets `~/.config/privacybit/catalog.d/` override grades. That stays.

**User fragments may never define a removal plan, a unit name, or any operation
the helper performs.** Those come only from the shipped catalogue.

A grade override is safe: it changes a label the user reads, and D1 §5.1
already requires the catalogue grade to be shown alongside it. A root operation
is not safe: it resolves to an argv template, and a user-writable argv template
handed to root is the escalation this document exists to prevent.

The helper therefore embeds the shipped catalogue and ignores
`catalog.d` entirely. This is a deliberate divergence from the CLI, and the
helper reports its own `catalog_rev` so a divergence is visible rather than
silent.

## 5. Validation order

Order is the whole point. Each step must complete before the next begins.

1. Load only the shipped embedded catalogue; never read user configuration.
2. Emit `ready` with the protocol and catalogue revision. This is a transport
   handshake, not approval of a request or evidence that an operation is valid.
3. Read one bounded JSON object per line until EOF. Reject duplicate keys,
   unknown fields, trailing JSON, and fields not allowed for that operation.
4. Check `op` and every enum against their closed sets.
5. Resolve entry ids and unit names against the embedded catalogue and compiled
   allowlist.
6. Check any review token against the active in-memory plan, expiry, entry,
   plan hash, and session.
7. Execute only after every check for that request passes. A malformed request
   closes the session without affecting completed requests.

The caller must receive `ready` before it sends a request, so the helper cannot
validate that request first. `ready` is therefore only a transport handshake.
Nothing before request validation performs an operation.

**Strict JSON, not permissive.** The default `encoding/json` silently drops
unknown fields, so `{"op":"purge-entry","entry":"google-chrome-stable","argv":["…"]}`
would parse cleanly and look like it worked. Disallow unknown fields outright,
and let the extra field be the thing that fails.

## 6. The sudo invocation

Copied from `plugins/moonbit/runner.go` because every piece of it is load-bearing.

```
sudo -k -- privacybit-helper
```

| Element | Why |
|---|---|
| `-k` | Invalidate the sudo timestamp. The password is asked **every time**. |
| no `-S` | sudo prompts through `/dev/tty`; privacybit never reads or retains the password. The helper's JSON protocol uses its stdin. |
| `Setpgid: true` | Own process group, so the grace-kill can signal sudo's group without touching the caller. |

### 6.1 Password handling

sudo owns password input on the controlling terminal. The TUI cedes the
terminal while the action runs; the CLI does the same through `/dev/tty`. The
helper request travels on a pipe attached to sudo's stdin, and no privacybit
struct, buffer, log, or environment variable contains the password. The parent
waits for `ready` before sending a request.

### 6.2 Shutdown

```go
cmd.Process.Signal(syscall.SIGALRM)
time.Sleep(killWait)                       // 3s: covers sudo's 2s terminate_command
syscall.Kill(-pgid, syscall.SIGKILL)
cmd.Process.Kill()
```

`SIGALRM`, not `SIGTERM`. sudo runs `terminate_command` on the way out, and
that cleanup kills the root child. Signalling sudo with something it does not
handle aborts the cleanup and **orphans a root process**.

The caller also refuses to signal its own group:

```go
if own, err := syscall.Getpgid(0); err != nil || pgid == own {
    pgid = 0                               // never signal ourselves
}
```

Getting this wrong is a self-inflicted `SIGKILL` of the panel.

### 6.3 `readyTimeout`

Bounds sudo's check plus the helper's start. On expiry, kill and report. A
helper that never becomes ready has failed in a way the user needs told about,
not an operation to wait on indefinitely.

## 7. Review before destroy

`pacman -Rns` can remove dependencies nobody asked it to delete. That is not a
hypothetical: it is what the flag means.

So `purge-entry` cannot be called directly. The sequence is:

1. `plan-purge entry=<id>` → the helper resolves a package-only catalogue
   entry, computes the removal set through pacman's supported read-only
   `pacman -Rs --print` path, lists reverse dependencies and installed sizes,
   then issues a **single-use in-memory review token**.
2. The caller shows all of it to the user and gets a real confirmation.
3. `purge-entry entry=<id> token=<t>` → re-resolves and re-computes, then
   refuses unless the exact package set still matches. Execution uses
   `pacman -Rn` with that explicit set, so it cannot expand into a new
   dependency removal after review. Pacman still checks dependency integrity
   and may refuse if package state changed.

### 7.1 The token binds four things

| Binds to | Prevents |
|---|---|
| the entry id | applying a plan for a different entry |
| the plan hash | the removal set changing between plan and execute |
| the helper session and plan timestamp | replaying a token in another process |
| an expiry | a plan sat around after the user reviewed it |

The caller's audit `generated_at` is displayed as context, but is not trusted
as proof of freshness: a root helper cannot verify a timestamp supplied by an
unprivileged caller. It re-computes package state immediately before executing.
The token lives only in the helper process for five minutes and is consumed
once.

### 7.2 The confirm is enforced at the privileged boundary

This is the part that is easy to get wrong.

The CLI displays the plan and requires the exact entry id on the controlling
terminal before it sends `purge-entry`. The panel has no tier 4 verb in v1.
Regardless of the caller, the helper independently refuses a `purge-entry`
without a valid in-memory plan token and a matching recomputed package set.
The helper's check is the privileged boundary; the CLI prompt is the user's
attended review step.

### 7.3 Why orphans would be different

moonbit *does* run unattended `pacman -Rns`, on `-Qtdq` output. That is not a
counterexample to this section — it is the boundary of it.

`pacman -Qtdq` lists packages that are neither required by anything nor
explicitly installed. Removing such a package cannot cascade, because by
definition nothing depends on it. The removal set is safe **by construction**
rather than safe because it was reviewed.

The distinction the token enforces is therefore exactly: **review is required
when the user chose the target, and not required when the removal set is
computed to be safe.** A user-chosen purge can cascade into a desktop
environment they still want. An orphan cannot.

Should privacybit ever gain a `purge-orphans`, the token comes off. It has no
place here in the meantime — that is a cleaner job, and moonbit already has it.

## 8. Audit logging

moonbit logs every panel operation with its arguments and result. privacybit
logs more, because its operations change the user's privacy posture and that
record is itself sensitive.

Each entry: timestamp, op, resolved entry or unit, exact package set, result,
and error. The audit timestamp is contextual data supplied by the caller, not
an authorization claim.

| Property | Reason |
|---|---|
| Append-only | An audit log that can be rewritten is not an audit log. |
| `0600`, user-owned | It records exactly what someone did about their privacy. |
| Never uploaded | D1 §10: no network calls, anywhere, ever. |
| Never committed | `.gitignore` already excludes it, with the rationale inline. |

`purge-entry` and `set-unit` are recorded in full. `ping` is not recorded.

## 9. What the helper must never do

Stated negatively so it is testable:

- accept a filesystem path
- accept an argument vector, or any field named `argv`, `cmd`, `args`, `shell`
- accept a unit name outside the shipped allowlist
- perform a removal without a matching, unexpired review token
- **install software** (§3.4 — the direction rule)
- **touch disk encryption in any way** (§3.4)
- read or write outside its declared scope
- make a network call
- read `catalog.d` or any other user-writable configuration
- run an operation that is not idempotent

The last one matters more than it looks. If a run is interrupted between
`plan-purge` and `purge-entry`, re-running must converge, not compound. Every
verb must be safe to run twice.

## 10. Failure modes

| Failure | Behaviour | Side effects |
|---|---|---|
| sudo authentication fails | sudo exits and the client reports its status | none — validation precedes the operation |
| Not in sudoers | sudo exits and the client reports that authorization failed | none |
| Helper never readies | `readyTimeout`, kill | none |
| Helper crashes mid-op | terminal `error` event; op must be idempotent | partial, convergent |
| Malformed request | strict JSON rejection | none |
| Stale review token | rejection naming the reason | none |
| `pacman` lock held | report and retry advice; never force-unlock | none |

The lock case deserves a note: the classic response to a held `pacman` lock is
to delete it. Do not. A held lock means another package operation is running,
and removing its lock file is how two package managers corrupt each other.

## 11. Testing

| Property | Method |
|---|---|
| Rejection has no effect | Table of ~30 invalid requests. A spy filesystem and a spy exec assert **zero** calls. |
| Unknown fields rejected | A request with a valid op and one extra field must fail. |
| User fragments cannot widen power | A `purge-entry` spec injected via `catalog.d` is ignored by the helper. |
| No install verb exists | `install-entry` is rejected as an unknown verb, with zero side effects. |
| Audit scope stays per-user | `report-root` is rejected as an unknown verb, with zero side effects. |
| No encryption surface | No request can reach a disk, a crypttab, or an initramfs. Asserted by spy filesystem. |
| Password handling | The privacybit process never receives password bytes; test command setup and helper protocol independently. |
| No orphan on cancel | Cancel mid-run; assert no surviving process in the group, and that sudo's cleanup ran. |
| Review token binding | Wrong entry, changed plan, expired token, replay in another process, and consumed token are rejected. |
| Confirm not bypassable | A `purge-entry` with no token is refused even when presented as a panel request. |
| Idempotency | Every verb run twice; second run is a no-op, not an error. |
| Own-group guard | Simulated `pgid == own` must result in `pgid = 0`. |
| No network | Assert no socket is opened during any helper path. |

The fake sudo is moonbit's approach and it works: a stand-in script that mimics
the prompt, the re-prompt, and sudo's own refusals.

## 12. Resolved decisions

The three questions this document originally closed with are settled.

**1. `install-entry` does not exist.** Resolved against the direction rule
(§3.4): installing is tier 3, and the helper prints the command. moonbit has
never had a `pacman -S` in its codebase and adding one would have made it
strictly less safe than the project this design is modelled on. Cost: one
copy-paste. Benefit: the largest privileged surface in the design is gone.

**2. Review tokens live only in the helper session.** The original one-request
process could not retain a token for a second invocation, while a token file
would survive the process and outlive its reviewed plan. A short-lived session
keeps the capability in memory and recomputes the package plan before execution.

**3. `configure-fde` does not exist.** Removed on both grounds that matter. A
helper bug during root encryption produces an unbootable machine, which no
review dialog recovers from; and disk encryption is a security property, not a
privacy one, so D1 §1.1 already excluded it from the audit's remit. Reaching for
a root helper to change how data is *stored* is not something a privacy audit
should be doing.

## 13. Remaining open questions

No open scope decision blocks the v1 helper. `report-root` and `apply-entry`
are deferred. The implementation uses a fixed five-minute expiry and the
supported read-only pacman simulation path. Package removals on non-pacman
distributions remain unsupported in v1.

The earlier design asked:

1. **Should `plan-purge` ever skip the token?** moonbit runs unattended
   `pacman -Rns` on `-Qtdq` orphans, which is safe by construction — a package
   with no reverse dependencies cannot cascade. A "remove orphans" verb would be
   the one privileged action that needs no review token. It is also a cleaner
   job than a privacy audit, so it is probably moonbit's and not ours.
2. **Token expiry window.** Resolved to five minutes because the TUI's review
   is expected to be immediate; it is fixed rather than user-configurable.
