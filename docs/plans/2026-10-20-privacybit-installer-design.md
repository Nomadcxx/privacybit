# privacybit TUI installer design

**Date:** 2026-10-20

**Status:** approved

**Owner decision:** install the application in the current user's `~/.local/bin`; offer the existing user watcher or timer as an optional setup step; leave system-wide installation to the AUR package.

## Scope

This is an application installer, separate from the existing
`privacybit-installer --mode watcher|timer` command, which only installs user
systemd units. A new `privacybit-install` Bubble Tea command builds the
`privacybit` CLI from the checked-out source and installs it in `~/.local/bin`.
It can then configure no background work, the event watcher, or the scan timer
through `internal/installer`.

The installer runs as the current user and refuses to run as root. It does not
write outside the user's home, alter shell startup files, install other
software, or add a privileged helper verb. The AUR package will cover
system-wide installation.

## Build and install behavior

The command must be started from the privacybit source checkout. It builds
`./cmd/privacybit` into a temporary directory with `GOPROXY=off`,
`GOSUMDB=off`, `GOTOOLCHAIN=local`, and `-mod=readonly`. Missing cached modules
or a missing local Go toolchain fail with a clear message; the installer never
fetches them or edits `go.mod` and `go.sum`.

Only after a successful build does it atomically replace
`~/.local/bin/privacybit`. The destination directory and any existing target
must be owned by the current user; a symlink or non-regular target is an error.
If service setup later fails, the new CLI remains installed and the result view
states that explicitly.

## Interaction

The TUI presents the install path and a three-way service choice: no background
work, event watcher, or periodic timer. The no-background option disables any
privacybit watcher or timer already installed for this user; when none exist it
does nothing. A review step shows the selected mode before any writes. Progress
distinguishes building/installing the CLI from configuring user services. The
final view shows the binary path, the selected service mode, and any partial
failure without suggesting that setup completed.

No new dependency is needed: Bubble Tea and Lipgloss are already in the module
graph. The implementation uses the existing installer package for systemd
units and adds tests for safe atomic binary replacement and TUI state/error
handling.
