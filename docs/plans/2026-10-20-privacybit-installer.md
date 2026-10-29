# privacybit TUI Installer Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an unprivileged Bubble Tea installer that builds privacybit offline, installs it under `~/.local/bin`, and can optionally enable the existing user watcher or timer.

**Architecture:** Add safe binary build/install operations to `internal/installer`, reusing its ownership checks and user-service setup. Add a small `cmd/privacybit-install` TUI that reviews the selected mode before running those operations and reports partial failures accurately.

**Tech Stack:** Go 1.26.4, standard library, existing Bubble Tea and Lipgloss dependencies.

---

### Task 1: Add atomic user-binary installation

**Files:**
- Create: `internal/installer/binary.go`
- Create: `internal/installer/binary_test.go`
- Modify: `internal/installer/install.go` only if a shared file-writing helper is needed

**Step 1: Write a failing test**

Test that an executable is atomically installed as `privacybit` with mode `0755`, replacing an existing owned regular file. Add a case proving that a symlink target is rejected and its referent remains untouched.

**Step 2: Run the focused test and confirm the expected failure**

Run: `GOTMPDIR=/home/nomadx/.cache/privacybit-gotmp go test ./internal/installer -run 'TestInstallBinary' -count=1 -exec='env GOTMPDIR=/tmp/pbt'`

Expected: the new API is not defined.

**Step 3: Implement the minimum safe file operation**

Validate the source executable, require an absolute destination directory owned by the current user, reject symlink/non-regular destinations, copy into a temporary file in the destination directory, sync it, set mode `0755`, and rename it into place. Reuse or generalize existing directory/file checks only where that removes duplication without changing unit-install behavior.

**Step 4: Run the focused test and confirm it passes**

Run the same command. Expected: both install and rejection cases pass.

### Task 2: Add offline build-and-install orchestration

**Files:**
- Modify: `internal/installer/binary.go`
- Modify: `internal/installer/binary_test.go`

**Step 1: Add a failing test for checkout validation or build failure**

Verify that a failed build leaves an existing installed binary unchanged. Also build and run the actual CLI under a temporary home, proving the successful offline path and that the source `go.mod` and `go.sum` remain unchanged.

**Step 2: Run the focused test and confirm it fails for the missing behavior**

Run: `GOTMPDIR=/home/nomadx/.cache/privacybit-gotmp go test ./internal/installer -run 'TestBuildAndInstall' -count=1 -exec='env GOTMPDIR=/tmp/pbt'`

**Step 3: Implement the build path**

Build `./cmd/privacybit` into a temporary directory using `go build -mod=readonly`, with `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, `GOWORK=off`, and empty `GOFLAGS` in the child environment. Install only after a successful build, then remove temporary output.

**Step 4: Run the focused test and confirm it passes**

Run the same command. Expected: build failures leave the prior target intact.

### Task 3: Add the installer TUI and command

**Files:**
- Create: `cmd/privacybit-install/main.go`
- Create: `cmd/privacybit-install/main_test.go`

**Step 1: Write failing model tests**

Cover that service setup is not invoked before review confirmation, the selected service mode is honored, and a service-setup failure says the CLI remains installed. Cover root invocation rejection before any install callback runs.

**Step 2: Run the focused tests and confirm expected failures**

Run: `GOTMPDIR=/home/nomadx/.cache/privacybit-gotmp go test ./cmd/privacybit-install -count=1 -exec='env GOTMPDIR=/tmp/pbt'`

Expected: the command and model are not defined.

**Step 3: Implement the smallest TUI state flow**

Show the fixed install path, allow `none`, `watcher`, or `timer`, require Enter on a review screen before starting, display build/service progress separately, and show precise success or partial-failure text. Use `installer.Disable` to stop previously installed privacybit units for `none`; use the existing `installer.Install` for watcher or timer setup. Refuse root execution. Do not add an uninstall path, shell-profile edits, or system-wide target.

**Step 4: Run the focused tests and confirm they pass**

Run the same command. Expected: all model and root-guard cases pass.

### Task 4: Verify and document the installer

**Files:**
- Modify: `docs/HANDOVER.md`
- Modify: `docs/CONTEXT.md` after the implementation is verified

**Step 1: Run focused formatting, tests, build, and vet**

Run `gofmt` on changed Go files, focused tests for `internal/installer` and `cmd/privacybit-install`, then `go build ./cmd/privacybit-install ./cmd/privacybit` and `go vet ./internal/installer ./cmd/privacybit-install` with the repository's offline Go settings.

**Step 2: Exercise the TUI without installing anything**

Launch it in a terminal, confirm the welcome/review screens, then quit before confirmation. Verify the command does not create `~/.local/bin/privacybit` or user systemd files before explicit confirmation.

**Step 3: Update the handover**

Record the new command, tests, design/plan paths, and exact verification results. Keep the live Niri gate and other unrelated dirty work unchanged.

### Follow-up: Cancel active install operations

Issue #34 adds cancellation to the active build and service setup.

**Files:**
- Modify: `cmd/privacybit-install/main.go`
- Modify: `cmd/privacybit-install/main_test.go`
- Modify: `internal/installer/binary.go`
- Modify: `internal/installer/binary_test.go`

**Acceptance:** Ctrl+C cancels the running command and the TUI waits for its
result. A canceled build does not replace the current CLI. Cancellation during
service setup reports that the CLI remains installed and service state may be
partial.

**Verification:** Cover build and service cancellation, the late build-result
race, and cancellation immediately before atomic replacement. Run focused
tests, vet, build, and race checks for `cmd/privacybit-install` and
`internal/installer`.
