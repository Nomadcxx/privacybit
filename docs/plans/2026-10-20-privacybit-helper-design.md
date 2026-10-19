# privacybit — D4: root helper

**Date:** 2026-10-20
**Status:** approved direction, pre-implementation
**Depends on:** [D1](2026-10-20-privacybit-design.md) (§9 actions, §11.4 review binding), [D2](2026-10-20-privacybit-cli-design.md)
**Reference:** `Nomadcxx/sysc-plugins/plugins/moonbit/runner.go` (288 lines, read in full)
**Security-sensitive.** Review this document before writing any code in it.

---

## 1. Scope

`privacybit-helper` is a headless, per-request privileged binary. It performs
tier 4 actions and nothing else. It is not a daemon, it has no UI, and it
never accepts a name it did not already know.

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
| `privacybit-installer` | install privacybit, its systemd user units, and its schedule |

Installing a privacy tool means installing a periodic audit, so the installer
is a real deliverable rather than an afterthought. It is not part of D4's
security surface: it runs interactively, from a checkout, with a human
watching.

## 3. The closed verb set

The single most important property in this document. **The helper never accepts
a package name, a path, a unit name, or an argument vector. It accepts intent,
and resolves it against its own embedded catalogue.**

```jsonc
// accepted — catalogue ids and closed-set enums only
{"op": "install-entry", "entry": "brave-origin"}
{"op": "set-unit",      "unit": "fwupd.service", "state": "disabled"}
{"op": "plan-purge",    "entry": "google-chrome-stable"}
{"op": "purge-entry",   "entry": "google-chrome-stable", "token": "…"}
```

```jsonc
// rejected before any side effect, and before sudo would need to exist
{"op": "install-entry", "entry": "../../etc/cron.d/x"}   // not a catalogue id
{"op": "set-unit",      "unit": "anything-at-all"}     // not in the allowlist
{"op": "install-entry", "package": "evil"}              // field does not exist
{"op": "run",           "argv": ["sh", "-c", "…"]}     // verb does not exist
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
| `status` | no | Helper build, embedded catalogue revision. |
| `apply-entry` | no | Flip a preference-backed entry. Tier 2. |
| `report-root` | yes | Audit with root visibility (§3.4). |
| `install-entry` | yes | Install an alternative resolved from the catalogue. |
| `set-unit` | yes | Enable/disable an allowlisted systemd unit. |
| `plan-purge` | yes | Compute a removal plan and issue a review token. Read-only. |
| `purge-entry` | yes | Execute a removal, given a valid token. |
| `configure-fde` | yes | Guided full-disk-encryption setup. |

### 3.3 Unit allowlist

`set-unit` accepts only units the helper ships. v1 allowlist, in full:

```
fwupd.service          fwupd-refresh.timer
upower.service
PackageKit.service
```

These are the three that phone home from a default Arch install, which is the
population privacybit starts with. Growing the list is a source change and a
review, never a caller decision.

### 3.4 `report-root`

Running the audit as root sees other users' processes and other users'
`/proc` entries. That directly contradicts D1 §14, which scoped the audit to
the current user.

Rather than silently narrow the scope, this verb makes the wider scope an
explicit, separate, opt-in request. Its output is labelled with the wider
scope, because an audit covering every user on a shared machine discloses
something a per-user audit does not.

**Default: off.** A machine with one user gains nothing from it.

## 4. User fragments cannot define install specs

D1 §7 lets `~/.config/privacybit/catalog.d/` override grades. That stays.

**User fragments may never define an install spec, a unit name, or a removal
plan.** Those come only from the shipped catalogue.

A grade override is safe: it changes a label the user reads, and D1 §5.1
already requires the catalogue grade to be shown alongside it. An install spec
is not safe: it is an argv template, and a user-writable argv template handed
to root is the escalation this document exists to prevent.

The helper therefore embeds the shipped catalogue and ignores
`catalog.d` entirely. This is a deliberate divergence from the CLI, and the
helper reports its own `catalog_rev` so a divergence is visible rather than
silent.

## 5. Validation order

Order is the whole point. Each step must complete before the next begins.

1. **Read one line from stdin.** Refuse if more arrives before the line ends.
2. **Parse strict JSON.** Unknown fields rejected, not ignored.
3. **Check `op` against the closed verb set.** Unknown → reject.
4. **Check every field against its closed set.** Unknown enum value → reject.
5. **Resolve ids against the embedded catalogue.** Unresolved → reject.
6. **Check any review token.** Invalid, expired, or mismatched → reject.
7. **Check the audit binding.** `generated_at` still current → else reject.
8. **Emit `ready`.**
9. **Execute.**

Nothing before step 8 touches the system. A request that fails validation has
provably had no effect, which is what makes the audit log honest and the tests
meaningful.

**Strict JSON, not permissive.** The default `encoding/json` silently drops
unknown fields, so `{"op":"install-entry","entry":"brave-origin","argv":["…"]}`
would parse cleanly and look like it worked. Disallow unknown fields outright,
and let the extra field be the thing that fails.

## 6. The sudo invocation

Copied from `plugins/moonbit/runner.go` because every piece of it is load-bearing.

```
sudo -S -k -p "[privacybit-panel-password]" privacybit-helper <verb>
```

| Element | Why |
|---|---|
| `-S` | Password on stdin. Never on argv, never through the environment. |
| `-k` | Invalidate the sudo timestamp. The password is asked **every time**. |
| `-p "[privacybit-panel-password]"` | A marker string the stderr watcher can detect. |
| `Setpgid: true` | Own process group, so the grace-kill can signal sudo's group without touching the caller. |

### 6.1 Password handling

```go
asked := false
for {
    select {
    case <-prompts:
        if asked {
            kill()
            return nil, ErrWrongPassword   // sudo asked twice: wrong password
        }
        asked = true
        stdin.Write(append(password, '\n'))
    case <-ready:
        stdin.Write(append(line, '\n'))     // request sent only after ready
        return op, nil
    }
}
```

Three properties, each with a reason:

- **A wrong password is detected by sudo asking twice**, not by parsing sudo's
  complaint. Prompt-counting is robust to every sudo localisation and every
  failure message.
- **The request is sent only after `ready`.** The request line is a capability;
  handing it to a process that has not proved it is listening would leak it into
  whatever inherited the pipe.
- **`defer clear(password)`** in `Start`, before the error is returned. The
  password does not survive the call.

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

`pacman -Rns` unattended deletes dependencies nobody asked it to delete. That
is not a hypothetical: it is what the flag means.

So `purge-entry` cannot be called directly. The sequence is:

1. `plan-purge entry=<id>` → the helper resolves the entry, computes the exact
   removal set, its reverse dependencies, and the disk reclaim, then issues a
   **review token**.
2. The caller shows all of it to the user and gets a real confirmation.
3. `purge-entry entry=<id> token=<t>` → re-resolves, re-computes, and refuses
   unless the plan matches the token exactly.

### 7.1 The token binds four things

| Binds to | Prevents |
|---|---|
| the entry id | applying a plan for a different entry |
| the plan hash | the removal set changing between plan and execute |
| the audit `generated_at` | acting on state the user never reviewed (D1 §11.4) |
| an expiry | a plan sat on disk and ran a week later |

### 7.2 The confirm is enforced at the privileged boundary

This is the part that is easy to get wrong.

The TUI (D3) will have a proper two-phase confirm. If that is the only place a
confirmation is required, then the **panel's button becomes a backdoor around
it** — D5 wires a `privacybit:run-action` verb into `activateControlCentre`,
and that path skips the TUI entirely.

So the helper independently refuses a `purge-entry` without a valid token.
The UI's confirm is a courtesy; the helper's check is the control.

## 8. Audit logging

moonbit logs every panel operation with its arguments and result. privacybit
logs more, because its operations change the user's privacy posture and that
record is itself sensitive.

Each entry: timestamp, op, resolved entry or unit, args, result, error, and
the audit `generated_at` the action was bound to.

| Property | Reason |
|---|---|
| Append-only | An audit log that can be rewritten is not an audit log. |
| `0600`, user-owned | It records exactly what someone did about their privacy. |
| Never uploaded | D1 §10: no network calls, anywhere, ever. |
| Never committed | `.gitignore` already excludes it, with the rationale inline. |

`install-entry` and `purge-entry` are recorded in full. `ping` is not recorded.

## 9. What the helper must never do

Stated negatively so it is testable:

- accept a filesystem path
- accept an argument vector, or any field named `argv`, `cmd`, `args`, `shell`
- accept a unit name outside the shipped allowlist
- perform a removal without a matching, unexpired review token
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
| Wrong password | `ErrWrongPassword`, kill, report | none — validation precedes privilege |
| Not in sudoers | `startError` maps "not in the sudoers" to a readable message | none |
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
| User fragments cannot widen power | An `install-entry` spec injected via `catalog.d` is ignored by the helper. |
| Password handling | Fake sudo: prompt once (success), prompt twice (`ErrWrongPassword`), and "not in the sudoers file". |
| No orphan on cancel | Cancel mid-run; assert no surviving process in the group, and that sudo's cleanup ran. |
| Review token binding | Wrong entry, stale plan, expired token, and mismatched `generated_at` each rejected. |
| Confirm not bypassable | A `purge-entry` with no token is refused even when presented as a panel request. |
| Idempotency | Every verb run twice; second run is a no-op, not an error. |
| Own-group guard | Simulated `pgid == own` must result in `pgid = 0`. |
| No network | Assert no socket is opened during any helper path. |

The fake sudo is moonbit's approach and it works: a stand-in script that mimics
the prompt, the re-prompt, and sudo's own refusals.

## 12. Open questions

1. **Should `install-entry` delegate to the system package manager at all?** An
   alternative framing is to print the exact command and let the user run it
   themselves. That removes the whole privileged surface for the most-used
   action, at the cost of one copy-paste. Worth a deliberate decision rather
   than drifting into it.
2. **Review token transport.** Passed on stdin in the request, or a file under
   `$XDG_RUNTIME_DIR` with `0600`? The file survives a panel restart; stdin
   does not.
3. **`configure-fde` is a large surface.** Encrypting a root partition can make
   a machine unbootable. It may deserve to be a separate documented flow rather
   than a verb.