# TUI Sandbox Context Implementation Plan

> **For Codex:** Implement this plan in the `fix/tui-sandbox-context-20261030` worktree.

**Goal:** Make existing Flatpak sandbox evidence visible in the TUI and keep incomplete saved reports from showing a stale green overall grade.

**Status:** Implemented for issue #96.

**Architecture:** Normalize missing or incomplete sandbox evidence in `report.ParseSnapshot` to an unknown sandbox and overall grade without rewriting the stored report or changing the schema number. Add a sandbox screen to the existing Bubble Tea model, and wrap and scroll its bounded detail text with Lip Gloss.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing `internal/report` snapshot types.

---

### Task 1: Lock the display contract with tests

**Files:**
- Modify: `internal/tui/model_test.go`
- Modify: `internal/report/snapshot_test.go`

**Steps:**
1. Add parser tests for missing sandbox data, unknown grades, and old green overall grades.
2. Add TUI assertions for unknown overall with an unrated axis and worst known grade.
3. Add sandbox screen tests for applicability, per-copy scope, grants, unknown permissions, partial coverage, and truncation notices.
4. Add navigation and long-detail wrapping/scrolling tests.
5. Run focused report and TUI tests and confirm the new assertions fail because normalization, route, and view are missing.

### Task 2: Implement the screen and overview context

**Files:**
- Modify: `internal/report/snapshot.go`
- Modify: `internal/tui/model.go`

**Steps:**
1. Normalize missing or incomplete sandbox evidence and set the in-memory overall grade to unknown.
2. Add the sandbox route to the overview and the screen title/content switch.
3. Render applicability, grade, coverage counts, per-copy scope and grade, grants, unknown permissions, and existing truncation counts.
4. When overall is unknown, name the category or sandbox axis with unknown evidence and show the worst known grade as context.
5. Wrap long detail text, scroll it within available height, and show a resize message when the terminal cannot display the view.
6. Keep schema version 1 and never rewrite the saved report during parsing.
7. Run focused report and TUI tests and confirm all tests pass.

### Task 3: Verify the focused change

**Steps:**
1. Run `gofmt` on changed Go files.
2. Run `GOMAXPROCS=4 go test -p 2 ./internal/tui ./internal/report`.
3. Inspect the diff and confirm it changes only snapshot parsing, the TUI, tests, and this plan.
