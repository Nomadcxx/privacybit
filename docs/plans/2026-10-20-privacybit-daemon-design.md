# privacybit — scheduler and event watcher (D7)

**Date:** 2026-10-20
**Status:** audited and amended for the v1 implementation
**Depends on:** D1 (engine), D3 (TUI), D6 (sources)

---

## 1. Why the daemon barely exists

The daemon justifies itself on **exactly one thing: event watching.** Everything
else it could do is better done by something else.

- **Periodic scan and Class-B fix** — a stateless systemd **timer** does this.
  moonbit runs its clean from a `.service` on a `.timer`, not from the daemon.
- **Report on demand** — the user runs `privacybit report` when they are there.
- **Continuous event watching** — genuinely needs a long-lived process.

moonbit makes its daemon and its timer units `Conflicts=` each other so systemd
stops one when the other starts. That is not a convenience; it is the author
recognising that running both modes at once is a design smell. privacybit
inherits that separation.

**The honesty constraint: if event watching is ever cut, the daemon should not
exist.** Two modes, two units, mutually exclusive.

## 2. Three duty classes

The safety model. Each class is a different relationship to risk.

| Class | What | Unattended? |
|---|---|---|
| **A — read** | Scan, report, coverage | Always |
| **B — reversible toggle** | Preference toggles, user-scoped | **Yes, and only these** |
| **C — privileged or stateful** | Purge, machine-wide unit changes, disk encryption, act on a cached scan | **Never** |

### 2.1 The Class B admission test

> **An auto-fix must be monotone toward privacy and reversible in one command.**

Three conditions, all necessary.

**Monotone** means applying it can only *reduce* disclosure. It never has a
side benefit that a user might later miss.

**Reversible in one command** means the user can undo it without reading a
manual, and knows the command exists because we told them at the moment of
application.

**User-scoped** means the unattended action cannot change machine-wide state or
affect another account. A system-wide operation stays attended even when it
passes the other two conditions.

Together they bound the worst case to: *privacybit made my machine slightly
less convenient for a month, and told me how to undo it.* That is an acceptable
worst case. A month of mild inconvenience, reversible, is not data loss. A
month of a disabled firmware updater is a machine that developed a security
problem while feeling fine.

Anything failing this test is Class C, **however harmless it sounds.**

### 2.2 The discriminating case: `fwupd`

This is the example the rule exists for, because both options sound privacy-
positive and only one is.

- **Disabling `fwupd.service`** — stops firmware phone-home, and stops firmware
  updates. That is a **security regression dressed as a privacy win.** The user
  does not get a warning; they get a machine that quietly stops patching.
  **Fails** — not monotone (it trades one risk for another), and not reversible
  in one command without also re-enabling phone-home.
- **Disabling `fwupd-refresh.timer`** — this timer only refreshes the *metadata
  list* of available firmware. Disabling it stops the metadata phone-home while
  leaving manual updates fully working, so it passes the monotone and
  one-command reversibility checks. It is still machine-wide, so it is not
  Class B and must never run unattended. It remains an attended action through
  D4's root helper.

Same software. Same vendor. Same intent. One is Class B and one is Class C, and
the difference is entirely in what else the switch turned off. This is the
discipline.

## 3. Class B is empty in v1

The admission rule stays, but the current catalogue does not model concrete
preference keys, their effective values, or a reliable one-command revert.
Firefox, Thunderbird and Flatpak settings are candidates for future work, not
implemented actions.

There are therefore **no Class B actions in v1**. The monthly scan is read-only;
it never changes preferences, Flatpak overrides, services, or packages. Add an
action only after its setting is represented in the catalogue and its apply
and revert paths have focused tests.

Machine-wide systemd unit changes such as `fwupd-refresh.timer` remain attended
D4 helper actions; the user daemon never applies them.

**No new toggles may be added by user configuration.** The Class B set is
compiled in, for the same reason D4's verb set is: a user-writable toggle that
a root-capable or unattended path can apply is a user-writable policy with root
consequences.

## 4. The staleness problem

D1 §11.4 binds attended remediation to the `generated_at` of the audit it was
reviewed against. V1 has no unattended remediation, so the scheduler has no
cached action plan to apply.

- The timer runs a new audit and stores that report in the same invocation.
- A monthly timer with `Persistent=true` may run late after downtime; the report
  records its actual generation time and performs no state changes.

Cadence here is a **safety parameter, not a performance parameter.** Frequent
unattended action on privacy posture means acting on stale knowledge. Monthly is
deliberate: it is the longest interval at which the state of the world is
unlikely to have shifted much since the user last looked.

## 5. Daemon duty: event watching

The one thing that needs a long-lived process.

### 5.1 New-app detection (poll)

Every few minutes, re-read the fast inventory (`pacman -Qq` and flatpak list —
~100ms on this machine) and diff against the last seen set. Anything new is run
through the catalogue. If it grades red, notify.

This is the feature that earns the daemon: **the user learns within minutes that
the thing they just installed is a tracker**, which is the fastest useful
privacy feedback available. At ~0.03% duty cycle it is nearly free.

**Polling first.** A `pacman` `PostInstall` hook would be instant but runs as
root, adds permanent attack surface to every future package transaction, and
needs a privileged→user channel — all for a latency improvement. Polling ships;
the hook is a later, strict opt-in that drops a datagram at a `0600` user
socket. Not now.

### 5.2 Continuous disclosure detection

D1's PID→exe→endpoint matching remains a future direction, but v1 has no
reviewed endpoint data in the catalogue and does not inspect live telemetry
connections. The report must keep this check explicitly unavailable; an empty
connection list must never imply that no telemetry was found. Per-app history
also remains out of scope.

The distinction that keeps endpoint matching in the design and cuts per-app
history is:

> **A firewall logs destinations; it does not know `metrics.tiktok.com` is a
> tracker.** You could log every byte and still not know. privacybit knows
> because the catalogue says so.

When endpoint data and attribution are implemented, logging is the firewall's
job (nftables, and the user runs that) and judging known-bad destinations is the
catalogue's job. privacybit will report current evidence, not accumulate a
per-app history.

### 5.3 Not a shield

The daemon does **not** attempt enforcement, and this is a considered
non-goal:

- **IP blocklists break real services.** Google, Meta, Cloudflare and Microsoft
  serve mail, login, docs and video from the same frontends as their telemetry.
  An IP-based telemetry blocklist breaks real services *by default*. This is not
  an edge case; it is the default failure, and it would destroy trust in
  privacybit faster than any leak it prevents.
- **The workable layer (DNS resolution) fights whoever owns the resolver** —
  systemd-resolved, NetworkManager, the VPN client. On this machine resolved
  isn't even managing `/etc/resolv.conf` (foreign mode, Tailscale MagicDNS
  active), so any DNS shield would be inserting itself into a path another VPN
  client already owns.
- **DoH defeats a DNS layer entirely.** Firefox TRR and Chrome DoH mean the
  browser — the app you would most want shielded — is the one app the layer
  cannot touch.
- **Per-process, per-endpoint blocking without a kernel module is not
  practical.** It is a project (OpenSnitch), not a feature.

The daemon **detects** and **alerts**. Enforcement is the firewall's job, run
by the user, in tools designed for it.

## 6. Privilege: user daemon, no root

Both units run as the **user**, not as root.

**Scope argument (D1 §14).** The audit is scoped to "the current user." A root
daemon's current user is `/root` — on a multi-user machine it either audits
root's empty home or reaches into someone else's. Neither is the audit the user
ran interactively, and a privacy tool that quietly audits the wrong person is
worse than one that audits nobody.

**Capability argument.** V1 watches the invoking user's package and Flatpak
inventory and writes only its own state and report. The scheduled audit is
read-only and writes only its own report. **privacybit's scheduled work needs
no root, and therefore should not run as root.** This is the D4 direction rule
applied to scheduling.

The v1 watcher has no IPC client, so it has no socket. Systemd owns its
lifecycle; `systemctl --user status privacybit-daemon.service` is the status
surface. Add a socket only with a real client and a bounded protocol.

## 7. Units

Like moonbit: a `Conflicts=` relationship between the long-lived watcher and
the periodic timer, so only one mode runs at a time.

```
privacybit-daemon.service   long-lived event watcher (user)
privacybit-scan.service     oneshot: read-only scan + report (user)
privacybit-scan.timer       monthly
```

`privacybit-daemon.service` and `privacybit-scan.service` `Conflicts=` each
other. The timer writes its report to
`$XDG_STATE_HOME/privacybit/report.json` with user-only permissions and marks
it unread until `privacybit report` opens it.

Hardening carried over from moonbit's correct choices: `NoNewPrivileges=yes`,
`PrivateTmp=yes`, `ProtectSystem=strict`, and `ProtectHome=read-only`, with an
explicit write path for `$XDG_STATE_HOME/privacybit`. Neither unit writes
browser preferences or any other user configuration.

## 8. Reporting

The timer writes the report and an unread marker. The user runs
`privacybit report`, which opens the stored snapshot in the TUI (D3) and clears
the marker. Pressing `r` runs a fresh audit and replaces the displayed report.

The daemon does not launch a terminal. It sends a desktop notification only
when a newly observed application matches a red catalogue entry. The timer
writes the report and marks it unread whether or not a graphical session is
running. `privacybit report` is the explicit presentation path.

## 9. Testing

| Property | Method |
|---|---|
| Read-only scheduled scan | The timer entrypoint invokes the audit and report store only; no mutation runner is reachable. |
| No Class C unattended | Purge and unit mutations are absent from the scheduled path. |
| New-app detection | Install a fixture package set; assert a red addition notifies and a green one does not. |
| Polling duty cycle | Bench the diff loop; assert the interval is honoured and the work is bounded. |
| User-scoped only | Assert the units run as the user; no `User=` omitted (which would mean root). |
| Report is durable | No graphical session; assert report exists and is marked unread. |
| Explicit gaps | Snapshot and UI identify telemetry attribution as unavailable; no connection count is rendered as zero. |

## 10. Deliverable shape

Phases 7–9 of D1 §15, reordered by this design:

| Phase | Deliverable |
|---|---|
| 7 (revised) | Event watcher: package/Flatpak inventory diff and red catalogue-entry alerts; telemetry attribution remains unavailable |
| 8 | Monthly read-only scan and durable report |
| 9 | TUI report screen (D3), daemon/timer install helpers (a `privacybit-installer` analogue, out of D4's security scope) |

### V1 scope amendment

The original D7 plan described Class-B preference writes and endpoint matching
without corresponding catalogue keys or reviewed endpoint data. V1 keeps the
admission rule and event-watcher shape, but does not guess either input. The
scheduled scan is read-only, and live telemetry attribution remains an explicit
coverage gap until its evidence source and data are designed.
