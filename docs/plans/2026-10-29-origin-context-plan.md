# Origin Context Implementation Plan

> **For Codex:** Follow the approved origin-context design and verify each behavior with a focused failing check first.

**Goal:** Report sanitized package origins as context across privacybit and its sysc-plugins panel without changing grades.

**Architecture:** Keep collection in the existing detection sources. `internal/report` sanitizes, deduplicates, sorts, and bounds the origins attached to grouped app snapshots; the CLI and TUI render that shared data. The plugin decodes the additive schema-1 fields, applies its own bounds, and renders the same provenance labels.

**Tech Stack:** Go standard library, existing privacybit snapshot schema 1, existing sysc plugin tree renderer.

---

### Task 1: Keep origin out of grade evidence

**Files:**
- Modify `internal/audit/audit.go`
- Modify `internal/audit/audit_test.go`

**Steps:**
1. Add a failing case with a contradicted origin on a green catalogue entry and require the grade to remain green with no grade finding.
2. Run `go test ./internal/audit -run 'Origin|Contradicted'` and confirm the origin case fails.
3. Remove origin from the contradiction inputs to `gradeEntry`; retain the other measured signals.
4. Run the focused audit tests and confirm the origin exception and other contradiction behavior pass.

### Task 2: Add bounded origin context to report snapshots

**Files:**
- Modify `internal/report/snapshot.go`
- Modify `internal/report/snapshot_test.go`

**Steps:**
1. Add failing tests for origin projection, deduplication by source and official status, deterministic sorting, Flatpak scope separation, label sanitation, the eight-origin cap, and unchanged grades.
2. Run `go test ./internal/report -run Origin` and confirm it fails on missing snapshot fields.
3. Add optional origin fields to `App`; project observed origins in `groupedApps`, sanitize labels to 80 UTF-8 bytes, and retain shown/total/truncated counts.
4. Run the focused report tests and confirm all projection limits and ordering pass.

### Task 3: Display provenance in privacybit

**Files:**
- Modify `cmd/privacybit/audit.go`
- Modify `cmd/privacybit/audit_test.go`
- Modify `internal/tui/model.go`
- Modify `internal/tui/model_test.go`

**Steps:**
1. Add failing CLI and TUI cases requiring origin context and official status to render for catalogue apps and unmatched apps.
2. Run `go test ./cmd/privacybit ./internal/tui -run Origin` and confirm the new cases fail.
3. Render origin labels and status in existing app details, and show unmatched origin context without changing unknown grades.
4. Run the focused command and TUI tests.

### Task 4: Carry origin context through the plugin

**Repository:** `Nomadcxx/sysc-plugins`, based on upstream `main`.

**Files:**
- Modify `plugins/privacy/snapshot.go`
- Modify `plugins/privacy/snapshot_test.go`
- Modify `plugins/privacy/view.go`
- Modify `plugins/privacy/view_test.go`

**Steps:**
1. Add failing parser and panel tests for bounded origin context on catalogue and unmatched apps, control-character sanitation, truncation, and a legacy snapshot without origins.
2. Add a failing panel test requiring source name and official status to display as context.
3. Extend the optional app and unmatched snapshot fields, sanitize/cap them during decoding, and render provenance in the panel.
4. Run `go test ./plugins/privacy ./cmd/sysc-plugin-privacy` and build the privacy plugin.

### Task 5: Reconcile design and handover documents

**Privacybit files:**
- Modify `docs/plans/2026-10-20-privacybit-design.md`
- Modify `docs/plans/2026-10-20-privacybit-sources-design.md`
- Modify `docs/CONTEXT.md`
- Modify `docs/HANDOVER.md`

**Steps:**
1. Record the issue #22 decision: origin is report context and never grade evidence.
2. Update the handover's repository revisions and describe the implemented privacybit and plugin state.
3. Review the diff for any remaining docs that claim origin contradictions change grades or that origin is absent from reports.

### Task 6: Verify, review, and integrate

**Steps:**
1. Run the focused tests above, then `go test ./...` and `go vet ./...` in privacybit.
2. Run the focused plugin tests, `go test ./...`, `go vet ./...`, and the privacy plugin build in sysc-plugins.
3. Manually inspect both repository diffs, snapshot compatibility, bounds, sanitized output, and grade invariants.
4. Open the privacybit PR first, then the sysc-plugins PR; merge only after both checks pass and review finds no blocking issue.
