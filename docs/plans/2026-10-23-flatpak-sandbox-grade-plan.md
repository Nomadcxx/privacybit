# Flatpak Sandbox Grade Implementation Plan

> **For Codex:** Use the approved Flatpak sandbox design and implement each task with a focused failing check first.

**Goal:** Grade effective Flatpak sandbox permissions per installed copy, include that axis in the overall snapshot, and show bounded evidence in the sysc-plugins panel.

**Architecture:** `detect.Flatpak` queries Flatpak for each exact app ref and installation. `internal/audit` classifies the returned permissions and owns the generic risk bands. `internal/report` emits a bounded `sandbox` section and includes its grade in the overall roll-up. The sysc-plugins panel renders the section and retains catalogue grades in their own fields.

**Tech Stack:** Go standard library, Flatpak CLI, existing privacybit report schema 1, existing sysc plugin tree renderer.

---

### Task 1: Collect effective permissions by installed ref

**Files:**
- Modify `internal/detect/flatpak.go`
- Modify `internal/detect/detect.go`
- Test `internal/detect/detect_test.go`

**Steps:**
1. Add a failing test proving duplicate app IDs query `flatpak info --show-permissions` separately with user/system installation and listed branch/architecture.
2. Add failure and cancellation cases: permission-query errors produce unavailable evidence; context cancellation aborts inventory.
3. Implement scope-aware per-ref querying with argv-only calls and preserve every listed observation.
4. Run the focused detector tests and confirm they pass.

### Task 2: Classify and aggregate sandbox evidence

**Files:**
- Create `internal/audit/flatpak.go`
- Modify `internal/audit/audit.go`
- Test `internal/audit/audit_test.go`

**Steps:**
1. Add failing cases for host filesystem, all devices, shared network, home access, X11, USB, complete low-risk permissions, missing output, malformed output, and unclassified permissions.
2. Add a failing roll-up case showing unclassified evidence remains unknown even when a known red grant is present, and ensure capped reports retain the worst copies.
3. Implement strict parsing of the CLI's key-file output, returning only recognized grants and sanitized reasons.
4. Add the separate sandbox result to `audit.Result`; compute per-copy and aggregate grades independently of catalogue matching.
5. Run the focused audit tests.

### Task 3: Report the sandbox axis

**Files:**
- Modify `internal/engine/scan.go`
- Modify `internal/report/snapshot.go`
- Modify `internal/report/snapshot_test.go`
- Modify `cmd/privacybit/audit.go`
- Modify `cmd/privacybit/audit_test.go`

**Steps:**
1. Add failing snapshot tests for additive sandbox JSON, bounded app rows, coverage counts, and worst-evidence overall grade.
2. Add a failing CLI output case for sandbox grade, scope, recognized grants, and unknown coverage.
3. Implement the bounded top-level `sandbox` object and include its grade when engine recomputes overall after network checks.
4. Keep the category progress score explicitly separate from the overall privacy grade.
5. Run focused report and command tests.

### Task 4: Render sandbox posture in the plugin panel

**Repository:** `Nomadcxx/sysc-plugins`, based on upstream `main`.

**Files:**
- Modify `plugins/privacy/snapshot.go`
- Modify `plugins/privacy/view.go`
- Modify corresponding privacy snapshot and view tests

**Steps:**
1. Add a failing decoder test for bounded sandbox rows and for old snapshots with no `sandbox` object.
2. Add a failing panel-tree test requiring a distinct “Flatpak sandbox” section, visible grade words, scopes, grants, and incomplete-coverage text.
3. Extend the bounded snapshot model and render the section using existing card, grade-chip, and text components.
4. Run the focused plugin tests and build the plugin.

### Task 5: Reconcile design and handover documents

**Files:**
- Modify `docs/plans/2026-10-20-privacybit-design.md`
- Modify `docs/plans/2026-10-20-privacybit-sources-design.md`
- Modify `docs/HANDOVER.md`

**Steps:**
1. Replace the stale claim that `flatpak info --show-permissions` is metadata-only with the verified effective-context behavior.
2. Record the approved grade bands, unknown rules, report shape, and implementation state in D1/D6 and HANDOVER.
3. Refresh changed line counts and plan links in the HANDOVER manifest.

### Task 6: Verify, review, and integrate

**Steps:**
1. Run focused tests, then the privacybit Go suite with `GOCACHE=/var/tmp/pbtt/gocache go test -count=1 -p 2 ./...`.
2. Run `go vet ./...`, `gofmt -l` on changed Go files, and build the plugin.
3. Inspect the complete diffs in both repositories for missing evidence paths, unbounded output, accidental changes, and UI regressions.
4. Open a privacybit PR for the detector, grader, report, and docs; open a sysc-plugins PR for the panel consumer.
5. Merge only after review approves both changes and the privacybit change lands first.
