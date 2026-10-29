# Privacybit GitHub Actions Implementation Plan

> **For Codex:** Implement this plan with the approved CI design and verify each command before merging.

**Goal:** Run privacybit's formatting, test, vet, and build checks automatically for pull requests and pushes to `main`.

**Architecture:** A single GitHub Actions workflow uses Go from `go.mod`, grants read-only repository permissions, and runs the existing Go checks sequentially with bounded parallelism.

**Tech Stack:** GitHub Actions, Go 1.26.4.

---

### Task 1: Add the CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

**Steps:**
1. Trigger on pull requests and pushes to `main`.
2. Set `contents: read` and configure Go from `go.mod`.
3. Run the gofmt check, full test suite, vet, and build in one job.

### Task 2: Verify and record CI

**Files:**
- Modify: `docs/HANDOVER.md`
- Create: `docs/plans/2026-10-29-privacybit-ci-design.md`
- Create: `docs/plans/2026-10-29-privacybit-ci-plan.md`

**Steps:**
1. Validate the workflow syntax with `actionlint` when available.
2. Run `test -z "$(gofmt -l .)"`.
3. Run `go test -count=1 -p 2 ./...`, `go vet -p 2 ./...`, and `go build -p 2 ./...`.
4. Make test fixtures declare their command availability so checks do not depend on runner-installed tools.
5. Confirm the GitHub Actions checks pass on the pull request.
6. Update the handover with the workflow and check results.
