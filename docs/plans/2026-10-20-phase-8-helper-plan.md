# Phase 8 — privileged helper implementation plan

**Date:** 2026-10-20

**Design:** [D4 — root helper](2026-10-20-privacybit-helper-design.md)

**Status:** approved corrections from the delivery audit

## Purpose

Deliver a separate `privacybit-helper` executable for the small, closed set of
attended tier 4 actions. The ordinary CLI and TUI remain unprivileged. The
helper reads only the embedded shipped catalogue and accepts intent by
catalogue id, never package names, paths, or caller-provided arguments.

## Corrections made before implementation

D4's original one-request process conflicted with both the `ready` handshake
and its in-memory-only review token: one process could not validate a request
before `ready`, then retain a token for a later invocation. The helper will
instead emit a transport-only `ready` event, accept strict JSONL requests in
one short-lived session, and keep a single-use purge token in memory for five
minutes. The token binds the entry, exact removal plan, session, and expiry.
The caller's audit timestamp is displayed as context but is not trusted by the
root helper.

`apply-entry` waits for modeled, reversible settings. `report-root` remains
deferred. The unit allowlist contains only `fwupd-refresh.timer`: disabling
`fwupd.service` also disables firmware updates, and the effects of the other
candidate units have not been shown to be privacy-only. Package purges are
limited to exact package entries on pacman systems. The plan uses supported
`pacman -Rs --print`; execution names the reviewed package set explicitly with
`pacman -Rn`, preventing a later dependency calculation from expanding it.
Pacman remains free to refuse if dependencies changed.

The helper's audit record must be user-owned, mode `0600`, and opened without
following user-controlled symlinks. A record is flushed before a privileged
mutation begins. The CLI/TUI never collect or retain a password; sudo handles
authentication on the controlling terminal while the helper protocol uses the
child's stdin.

## Implementation shape

`internal/helper` owns strict request decoding, operation validation, session
state, plan hashing, expiry, and dispatch through a narrow injected command
runner. The JSON decoder rejects unknown and duplicate keys, trailing data,
oversized lines, operation-inappropriate fields, and unknown enum values
before any command runs. The helper executable loads the embedded catalogue
with user fragments disabled, emits `ready`, processes requests until EOF, and
returns one bounded JSON response per request.

The CLI's `privacybit purge <entry-id>` starts `sudo -k -- privacybit-helper`
with a private stdin/stdout pipe, waits for `ready`, sends a plan request, and
prints the full removal set, reverse dependencies, reclaim estimate, and
expiry. It requires the user to type the exact entry id before sending the
one-use purge request. It never sends the request if the confirmation is
missing or mismatched. TUI action `p` cedes the terminal to this CLI command;
on return it re-runs the audit. A canceled or interrupted session discards its
token.

The helper records successful and attempted mutations in the invoking user's
state directory with directory traversal protected by file descriptors and
`O_NOFOLLOW`. If the user identity, state directory, or log cannot be validated,
the helper refuses the mutation instead of writing a root-owned file into a
user-controlled path.

## Acceptance evidence

The pure helper tests cover malformed JSON, duplicate keys, unknown fields,
wrong-operation fields, unknown ids, unknown units, non-pacman entries, missing
and replayed tokens, changed package plans, token expiry, and exact argv for
the read-only plan and explicit-set execution. Every rejected request is
asserted to make zero runner calls. A fake runner proves the full plan/confirm
session and its one-use token. Audit-log tests cover owner, permissions,
symlink rejection, and failed preflight with no mutation.

CLI tests cover ready-before-request ordering, the full rendered plan, exact-id
confirmation, cancellation, sudo command construction, and re-audit after a
completed action. No live package removal is part of verification. Build,
`go vet`, formatting, and the complete Go test suite are the phase gate.

`privacybit-installer` and user systemd units are delivered with Phase 9's
scheduler packaging; they are not part of the privileged helper binary.
