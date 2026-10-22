# Phase 9 — watcher, scheduled scan, and stored report

**Date:** 2026-10-20

**Design:** [D7 — scheduler and event watcher](2026-10-20-privacybit-daemon-design.md)

## Audited scope

D7 originally specified unattended preference writes and live telemetry
attribution. The shipped catalogue has no modelled reversible preferences and
no reviewed endpoint set. The amended D7 therefore keeps the timer read-only
and reports live telemetry as unavailable. The daemon's useful v1 event is a
new package or Flatpak installation that matches a red catalogue entry.

## Implementation

1. Add a user-only report store to `internal/report`. Store bounded schema-1
   snapshots atomically with mode `0600` under the user's state directory and
   maintain an unread marker. Reject malformed and unsupported snapshots.
2. Add an `internal/daemon` inventory watcher. Poll the available package and
   Flatpak sources on a fixed interval, preserve each source's last complete
   inventory across source failures, establish a quiet baseline on first run,
   and notify only for newly observed red catalogue entries. Unmatched entries
   remain unknown and produce no grade claim.
3. Add `privacybit daemon`, `privacybit scan`, and `privacybit report`:
   - `daemon` runs the watcher until canceled.
   - `scan` performs a fresh read-only audit and stores an unread report.
   - `report` opens the stored snapshot in the TUI, marks it read, and lets `r`
     perform a fresh audit.
4. Embed a user daemon service and a monthly persistent scan timer. Their
   `Conflicts=` rules prevent the watcher and timer from running together.
   Both units are read-only apart from privacybit's own state directory.
5. Add `privacybit-installer --mode watcher|timer` to install the embedded user
   units and enable exactly one mode. It writes only under
   `$XDG_CONFIG_HOME/systemd/user`; it does not run during verification.

## Checks

- Report store tests cover mode, round-trip, unread marker, malformed input,
  and size limits.
- Watcher tests cover first-run baseline, red and green additions, duplicate
  scopes, unavailable-source preservation, and notification failure.
- CLI tests prove scheduled scans save reports and `report` starts with the
  saved snapshot; pressing refresh uses a fresh scan.
- Unit-template tests pin the user scope, monthly cadence, read-only state
  access, mutual exclusion, and absence of mutation commands.
- Installer tests use a temporary home and a fake systemctl runner; no units
  are installed on the host during verification.
- Final gate: build, vet, gofmt, and all Go tests.

## Explicit omissions

No Class-B preference writes, package removals, system-unit changes, live
telemetry endpoint matching, nftables changes, IPC socket, or graphical
terminal launch are part of this phase.
