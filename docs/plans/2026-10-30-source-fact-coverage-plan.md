# Per-Source Fact Coverage Implementation Plan

> **For Codex:** Follow this plan against issue #50 and the approved coverage contract in D6 §3 and §10.

**Goal:** Show which inventory facts each ready source can provide and identify the sources that leave each fact unavailable, without changing grades.

**Architecture:** Keep `detect.Capabilities` as the source of truth. Carry it through `engine.Run` into the report, serialize a stable fact list per source, and aggregate unsupported facts into bounded `Reduced` notes. The CLI and TUI render those notes; the privacy plugin decodes and renders their source names in its existing Coverage limits card.

**Tech Stack:** Go standard library, existing schema-1 snapshot, existing sysc-plugins privacy panel.

---

### Task 1: Publish source capabilities

**Files:**
- Modify `internal/detect/detect.go`
- Modify `internal/audit/audit.go`
- Modify `internal/engine/scan.go`
- Test `internal/engine/scan_test.go`

Add a deterministic `Capabilities.Facts()` mapping for version, licence, vendor, explicit-install state, origin, reverse dependencies, and signature state. Carry each source's capability value into `SourceCoverage`. Cover a package source and a source with no metadata capabilities.

### Task 2: Project bounded report coverage

**Files:**
- Modify `internal/report/snapshot.go`
- Test `internal/report/snapshot_test.go`

Add optional `facts` to each source coverage row and `sources` to each reduced fact note. For active, non-empty sources, list supported facts and aggregate unsupported facts in stable order. Keep the report's grades unchanged and preserve compatibility with schema-1 snapshots that omit the new fields.

### Task 3: Render coverage in privacybit

**Files:**
- Modify `cmd/privacybit/audit.go`
- Test `cmd/privacybit/audit_test.go`
- Modify `internal/tui/model.go`
- Test `internal/tui/model_test.go`

Show supported facts beside each source and affected source names in reduced notes. Keep the wording factual and keep origin clearly identified as context.

### Task 4: Render reduced sources in the privacy plugin

**Repository:** `Nomadcxx/sysc-plugins`, clean worktree from upstream `main`.

**Files:**
- Modify `plugins/privacy/snapshot.go`
- Test `plugins/privacy/snapshot_test.go`
- Modify `plugins/privacy/view.go`
- Test `plugins/privacy/view_test.go`

Decode optional reduced-source names, sanitize and cap them at the snapshot boundary, then show them in the existing Coverage limits card. Keep older snapshots valid.

### Task 5: Reconcile documentation and verify

Update D6's coverage example and `docs/HANDOVER.md` to describe the implemented report shape. Run focused privacybit and plugin tests, then the repository checks needed for each PR. Review snapshot compatibility, output bounds, grade invariance, and the final diffs before merging.
