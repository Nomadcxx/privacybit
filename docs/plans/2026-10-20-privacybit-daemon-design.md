# privacybit — scheduler and event watcher (D7)

**Date:** 2026-10-20
**Status:** approved in dialogue, pre-implementation
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
| **C — destructive or stateful** | Purge, enable, disk encryption, act on a cached scan | **Never** |

### 2.1 The Class B admission test

> **An auto-fix must be monotone toward privacy and reversible in one command.**

Two conditions, both necessary.

**Monotone** means applying it can only *reduce* disclosure. It never has a
side benefit that a user might later miss.

**Reversible in one command** means the user can undo it without reading a
manual, and knows the command exists because we told them at the moment of
application.

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
  list* of available firmware. Disabling it stops the metadata phone-home
  while leaving manual updates fully working. **Passes** — monotone (only
  disclosure stops), reversible with `systemctl enable --now fwupd-refresh.timer`.

Same software. Same vendor. Same intent. One is Class B and one is Class C, and
the difference is entirely in what else the switch turned off. This is the
discipline.

## 3. What Class B contains, concretely

A closed set. Not a mechanism for arbitrary fixes — a specific list of specific
toggles, each with a documented one-command revert.

| Target | Change | Revert |
|---|---|---|
| Firefox prefs | telemetry / update-related keys | single `user.js` line removal |
| Thunderbird prefs | telemetry-related keys | as above |
| Flatpak overrides | tighten specific over-broad permissions on known apps | `flatpak override --reset <id>` |
| `fwupd-refresh.timer` | disable the metadata timer only | `systemctl enable --now` |
| `PackageKit.service` | disable if not in use | `systemctl enable --now` |

Each is user-scoped (`~/.mozilla`, `~/.local/share/flatpak`) except the systemd
units, which are machine-scoped but allowlisted (D4 §3.3).

**No new toggles may be added by user configuration.** The Class B set is
compiled in, for the same reason D4's verb set is: a user-writable toggle that
a root-capable or unattended path can apply is a user-writable policy with root
consequences.

## 4. The staleness problem

D1 §11.4 bound every remediation to the `generated_at` of the audit it was
reviewed against. **Unattended, that binding is vacuous** — nobody is reviewing
anything at 3am. So the rule becomes structural rather than procedural:

- **The timer re-scans in the same run.** It never auto-fixes from a cache.
- The auto-fix plan is computed from the scan taken in *this* run.
- A monthly timer with `Persistent=true` can fire weeks late after downtime;
  the plan is still computed fresh, so a late run is merely stale-by-design and
  bounded by re-scanning, not by the calendar.

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

D1's PID→exe→endpoint catalogue matching, kept. The distinction that keeps it
here and cut per-app history:

> **A firewall logs destinations; it does not know `metrics.tiktok.com` is a
> tracker.** You could log every byte and still not know. privacybit knows
> because the catalogue says so.

Logging is the firewall's job (nftables, and the user runs that). Judging
against known-bad destinations is the catalogue's job. Per-app history was cut
for exactly this reason — it is a firewall feature wearing a privacy hat. What
stays is *detection of disclosure*, judgement against the catalogue, not
accumulation of a log.

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

**Capability argument.** Everything the unattended path does (Class A and B) is
user-scoped: `~/.mozilla`, `~/.config`, `~/.local/share/flatpak`. The one
moonbit thing that needs root at 3am is `pacman -Rns` — which is Class C here,
so it never happens unattended. **privacybit's scheduled work needs no root,
and therefore should not run as root.** This is the D4 direction rule applied to
scheduling.

**Socket argument.** moonbit's panel socket is `0666`, so it must answer only
`ping`/`status` as *policy*: "root reach without a password must not be one
connect() away for every account." A **user** socket at `0600` in
`$XDG_RUNTIME_DIR` makes privileged reach impossible **by construction** rather
than by rule — the better of the two.

## 7. Units

Like moonbit: a `Conflicts=` relationship between the long-lived watcher and
the periodic timer, so only one mode runs at a time.

```
privacybit-daemon.service   long-lived event watcher (user)
privacybit-scan.service     oneshot: scan + Class-B fix (user)
privacybit-scan.timer       monthly
```

`privacybit-daemon.service` and `privacybit-scan.service` `Conflicts=` each
other. The daemon's socket is `0600`; the timer writes its report to
`$XDG_STATE_HOME/privacybit/report.json`.

Hardening carried over from moonbit's correct choices: `NoNewPrivileges=yes`,
`PrivateTmp=yes`, `ProtectSystem=full`. User units get `ProtectHome=read-only`
plus explicit write paths — Class B needs to write `~/.mozilla`, so `ProtectHome`
is `false` for the scan service but the daemon (watcher only, no writes) keeps
it.

## 8. Reporting

The daemon writes a report and marks it unread. The user runs
`privacybit report`, which opens the TUI report screen (D3).

A daemon **can** launch a terminal on a modern system — `systemd-run --user` a
transient unit with `After=graphical-session.target` running e.g.
`foot -e privacybit report` — verified live on this machine (`graphical-session.target`
active, `CanGraphical=yes`, `notify-send` present). So the "you ran a monthly
scan, here's what changed" flow is real.

But it is a **convenience, not a guarantee.** The daemon must not *require* a
TTY or a logged-in session; when there is no graphical session it writes the
report and marks it unread, and the next `privacybit report` shows it. Launching
a terminal is best-effort. The report is the durable artifact; the terminal is
the presentation.

## 9. Testing

| Property | Method |
|---|---|
| Class B admission enforced | A `fwupd.service`-style toggle is rejected; `fwupd-refresh.timer` accepted. Test the rule, not the current list. |
| No Class C unattended | Attempt to auto-purge / auto-enable from the timer path; must refuse. |
| Never fix from cache | Timer with a stale `generated_at`; assert it re-scans before planning. |
| New-app detection | Install a fixture package set; assert a red addition notifies and a green one does not. |
| Polling duty cycle | Bench the diff loop; assert the interval is honoured and the work is bounded. |
| User-scoped only | Assert the units run as the user; no `User=` omitted (which would mean root). |
| Socket perms | `0600`, in `$XDG_RUNTIME_DIR`; assert a second local account cannot connect. |
| Report is durable | No graphical session; assert report exists and is marked unread. |
| Terminal launch best-effort | With a graphical session, assert the transient unit is requested; failure is non-fatal. |

## 10. Deliverable shape

Phases 7–9 of D1 §15, reordered by this design:

| Phase | Deliverable |
|---|---|
| 7 (revised) | Event watcher: inventory diff, new-app alert, continuous disclosure detection |
| 8 | Monthly timer + Class-B auto-fix + report |
| 9 | TUI report screen (D3), daemon/timer install helpers (a `privacybit-installer` analogue, out of D4's security scope) |