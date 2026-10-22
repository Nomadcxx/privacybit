# Phase 4 Findings and Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add conservative, inspectable findings and safe tier 0 and tier 3 action descriptions to the Phase 3 audit result.

**Architecture:** Keep grading in `internal/audit`, derive findings from its existing result, and put catalogue install rendering in a small shared `internal/render` package. Actions are descriptions only; commands are never executed here.

**Tech Stack:** Go standard library and existing internal catalogue/audit packages.

---

### Task 1: D6 install instruction renderer

**Files:**
- Create: `internal/render/install.go`
- Create: `internal/render/install_test.go`
- Modify: `cmd/privacybit/explain.go`

**Steps:**
1. Write tests for Arch, Debian, and Fedora commands, an unsupported family with an HTTPS upstream, missing route, and unsafe package/URL data.
2. Run `GOMAXPROCS=4 go test -p 2 ./internal/render`; confirm the tests fail because the renderer does not exist.
3. Implement the smallest pure renderer; validate package tokens and HTTPS URLs before returning copyable instructions.
4. Reuse it from `explain` without changing its existing all-routes behavior.
5. Run `GOMAXPROCS=4 go test -p 2 ./internal/render ./cmd/privacybit`.

### Task 2: Finding and action model

**Files:**
- Modify: `internal/audit/audit.go`
- Modify: `internal/audit/audit_test.go`

**Steps:**
1. Add failing tests for non-green installed findings, judged missing expectations, severity mapping, no findings for unmatched/unjudged results, and tier 0 explanations.
2. Run `GOMAXPROCS=4 go test -p 2 ./internal/audit`; confirm the new tests fail for missing behavior.
3. Add the typed finding/action model and pure finding builder.
4. Add tests for tier 3 alternative instructions and ensure tiers 1–2 are omitted.
5. Run `GOMAXPROCS=4 go test -p 2 ./internal/audit`.

### Task 3: Scope duplicate finding

**Files:**
- Modify: `internal/audit/audit.go`
- Modify: `internal/audit/audit_test.go`

**Steps:**
1. Add a failing test for a known Flatpak present at both system and user scope.
2. Run the focused audit test and confirm the failure.
3. Add the low-severity informational finding without changing inventory or grades.
4. Test that single-scope and unmatched Flatpak observations do not create this finding.
5. Run `GOMAXPROCS=4 go test -p 2 ./internal/audit`.

### Task 4: Documentation and capped verification

**Files:**
- Modify: `docs/plans/2026-10-20-privacybit-design.md`
- Modify: `docs/HANDOVER.md`

**Steps:**
1. Record the approved Phase 4 rules in D1 and update the file/test manifest and phase status in the handover.
2. Review the complete worktree diff and run:
   - `GOMAXPROCS=4 go build -p 2 ./...`
   - `GOMAXPROCS=4 go vet -p 2 ./...`
   - `gofmt -l ./internal ./cmd`
   - `GOMAXPROCS=4 go test -p 2 -count=1 ./...`
   - `git diff --check`
3. Record only results actually observed.
