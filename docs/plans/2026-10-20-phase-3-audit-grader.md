# Phase 3 Audit Grader Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a deterministic audit grader that matches inventory observations, grades catalogue entries and expectations, reports coverage, and rolls up category and overall grades conservatively.

**Architecture:** Keep matching in `internal/catalog`, where exact detect rules are indexed. Add a pure evaluator in `internal/audit` that accepts observations and per-source coverage, returns matched and unmatched inventory plus category results, and never runs commands. A contradicted measured signal worsens its entry by one grade; zero, unavailable, and ordinary observed signals do not change the catalogue floor.

**Tech Stack:** Go 1.26.4, standard library, existing `internal/catalog` and `internal/detect` packages.

---

### Task 1: Exact catalogue lookup

**Files:**
- Modify: `internal/catalog/catalog.go`
- Modify: `internal/catalog/load_test.go`

**Steps:**
1. Add a public lookup taking `DetectType` and source-native name, using the existing exact indexed key semantics.
2. Add tests that a full type/name match resolves, a different type does not, and a longer/substr name does not.
3. Run `go test ./internal/catalog` and confirm it passes.

### Task 2: Audit result and inventory matching

**Files:**
- Create: `internal/audit/audit.go`
- Create: `internal/audit/audit_test.go`

**Steps:**
1. Define input coverage as source name, exact detect type, `detect.Availability`, and observed count; keep evaluator free of source execution.
2. Match each observation by its exact `Kind`-to-detect-type mapping and name. Preserve every observation, including two Flatpak installs with different scopes; unmatched observations receive `unknown` and do not enter category roll-up.
3. Add tests for exact matching, unknown inventory, and both scoped Flatpak installs remaining distinct.
4. Run `go test ./internal/audit` and confirm it passes.

### Task 3: Entry evidence and expectations

**Files:**
- Modify: `internal/audit/audit.go`
- Modify: `internal/audit/audit_test.go`

**Steps:**
1. Start each matched entry at its effective catalogue grade. A measured contradiction in any observation signal moves green to amber or amber to red; red stays red. Other states do not move the grade.
2. Treat an expectation as met by a matched entry ID or an exact raw package name for an ungraded package token. Grade fully judgeable unmet expectations red for high severity and amber for medium/low severity.
3. Add tests for catalogue-grade floor, contradiction downgrade, zero/unavailable/observed invariance, and expectation satisfaction/severity.
4. Run `go test ./internal/audit` and confirm it passes.

### Task 4: Coverage-aware category and overall roll-up

**Files:**
- Modify: `internal/audit/audit.go`
- Modify: `internal/audit/audit_test.go`

**Steps:**
1. Judge a missing expectation only when every detect type capable of satisfying its tokens has at least one ready source; ungraded tokens are package names. A missing source that blocks that judgement makes the category `unknown`.
2. Mark a category as partially covered when any of its detect types lacks a ready source, while retaining its worst known grade if all its expectations remain judgeable.
3. Roll category grades up with `catalog.Worse`; unknown propagates to overall. Unmatched observations never affect this roll-up.
4. Add tests for unavailable coverage yielding unknown, partial coverage with a known grade, worst-evidence aggregation, and unknown overall propagation.
5. Run `go test ./internal/audit` and confirm it passes.

### Task 5: Reconcile handover and verify the working tree

**Files:**
- Modify: `docs/HANDOVER.md`

**Steps:**
1. Update package/file/test manifests, phase status, and current line counts; retain the Flatpak permissions limitation and other open phase blockers.
2. Review the full diff and run the capped project verification requested by the handover:
   - `GOMAXPROCS=4 go build -p 2 ./...`
   - `GOMAXPROCS=4 go vet -p 2 ./...`
   - `gofmt -l ./internal ./cmd`
   - `GOMAXPROCS=4 go test -p 2 -count=1 ./...`
3. Confirm all checks pass and the working tree contains only intended changes.
