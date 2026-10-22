# privacybit — TUI design

**Date:** 2026-10-20
**Status:** approved; implementation audited 2026-10-20
**Depends on:** [D1 engine](2026-10-20-privacybit-design.md),
[D2 CLI](2026-10-20-privacybit-cli-design.md),
[D4 helper](2026-10-20-privacybit-helper-design.md)

---

## 1. Scope

`privacybit tui` and `privacybit report`. A full-screen terminal interface for
reading an audit, navigating it, and delegating the attended purge flow.

In scope: screens, navigation, keybindings, grade rendering, opening a stored
snapshot, fresh-scan refresh, and delegation of attended purge review to the CLI.

Out of scope: the engine (D1), the machine-readable contract (D2), the helper's
security model (D4), and the sysc-shell panel (D5).

The TUI is a **consumer of the snapshot**, exactly as the CLI and the future
panel are. It never computes a grade, and it never opens a detection source.
If the TUI needed to do either, that would be a bug in the engine's shape, not
in the TUI.

## 2. What moonbit's TUI gives, and what it does not

moonbit's `internal/ui` is 2,629 lines across `ui.go` and `colors.go`, with
Bubble Tea, two `viewport.Model`s, and a `ViewMode` string enum covering ten
screens. These parts transfer:

| Transfers | Why |
|---|---|
| `tea.NewProgram(m, tea.WithAltScreen())` | Correct for a full-screen inspector. |
| `runXCmd` → `xCompleteMsg` message pattern | Clean command/message separation; no blocking work in `Update`. |
| `borderedPanel(content, borderColor, width)` | The layout primitive. Useful as-is. |
| `interpolateColor` / `parseHexColor` | Animated grade transitions, for free. |
| `isShiftX(msg)` | Key-variant helper. |
| `colors.go` palette shape | Named role variables, not inline colours. |
| `partialScanWarning(failures)` | Partial-failure disclosure, which privacybit needs more than moonbit does. |
| The `ui_test.go` method | See §11. |

Three things do **not** transfer, and each one forces a different architecture
rather than a different widget.

**1. moonbit is a linear wizard; privacybit is a browser.** moonbit's flow is
`Welcome → ScanProgress → Results → Select → Confirm → Clean → Complete` — one
action, one direction, a single `ViewMode` field is sufficient. privacybit's
audit is a tree, and a user navigating it will walk
`Overview → Categories → Browser → Chrome → Alternatives` and then walk back
three levels. A single mode field cannot express that. §3.

**2. moonbit's result is one-dimensional; privacybit's is not.** A category has
a name, a file count and a byte total. A privacybit category has a grade, a set
of installed apps each with their own grade, an expectations-met list, and an
expectations-missing list — and the grade is the thing being rendered, in three
channels, everywhere. §5.

**3. moonbit's work is destructive and privatebit's is mostly not.** moonbit
wears a progress bar through a delete. privacybit's primary job is *reading*,
with actions attached. The interface should look like an inspector, not like a
wizard asking for permission to start. This is why §4's inventory starts at
Overview rather than at a menu.

## 3. Navigation: a stack, not a mode

The single largest divergence from moonbit.

```go
type Route struct {
    Screen Screen
    ID     string // category id, entry id, finding id — screen-specific
}

type Model struct {
    stack []Route // last element is current
    ...
}
```

`Enter` pushes, `Esc` pops, `q` on the root quits. The header renders a trail
from the stack, so the user is never lost:

```
  privacybit   Browser › Google Chrome › Alternatives
```

Why a stack rather than a mode enum:

- **Depth is data.** The number of levels differs per item — some apps have
  alternatives, most do not; some findings have actions, most do not. A fixed
  mode enum would need modes for every combination, which is how moonbit
  accumulated `ModeDocker` and `ModeDockerConfirm` as a pair.
- **Back must be free and correct.** With a stack, popping restores exactly the
  parent state, including scroll offset and selection. With a mode enum, every
  transition has to remember what it came from, which is where wizard state
  machines leak.
- **It matches the domain.** The audit *is* a tree. The navigation should be the
  tree.

The stack replaces `maxMenuIndex`-style bounds logic with a single
`visibleRows()` per screen, which is where the real complexity lives anyway.

## 4. Screen inventory

```go
const (
    ScreenOverview  Screen = "overview"
    ScreenScanning  Screen = "scanning"
    ScreenCategories Screen = "categories"
    ScreenCategory  Screen = "category"
    ScreenApp       Screen = "app"
    ScreenFinding   Screen = "finding"
    ScreenNetwork   Screen = "network"
    ScreenCoverage  Screen = "coverage"
    ScreenFindings  Screen = "findings"
    ScreenUnmatched Screen = "unmatched"
    ScreenHelp      Screen = "help"
)
```

| Screen | Shows |
|---|---|
| `Overview` | Overall grade, score, finding count, and routes into categories, network, coverage, and unmatched apps. |
| `Scanning` | Initial scan or refresh status. |
| `Categories` | Every category with its grade. The main index. |
| `Category` | One category: installed apps with grades, expectations met, expectations missing. |
| `App` | One app: grade, sources, scope, reasons, and caveats. |
| `Findings` / `Finding` | The bounded finding list and one finding's evidence and instructions. |
| `Unmatched` | Installed applications absent from the catalogue, shown as unknown. |
| `Network` | Point-in-time VPN state and defects; live telemetry attribution is unavailable. |
| `Coverage` | What was scanned, what was not, what was truncated, and every `unknown` in the result. |
| `Help` | Keybindings and the grade legend. |

`Overview` is the default landing screen, not a menu. A menu would make the user
ask "what do I want to do?" before they know what the audit found, which is the
wrong order for a tool whose job is to tell them something.

## 5. Grade rendering, and where it lives

D2 §8 established three redundant channels so the grade survives colour-blindness,
a monochrome terminal, and a screenshot:

| Grade | Colour | Glyph | Word | ASCII |
|---|---|---|---|---|
| `green` | green | `●` | Good | `o` |
| `amber` | amber | `▲` | Review | `^` |
| `red` | red | `■` | Risky | `x` |
| `unknown` | muted | `?` | Unrated | `?` |

The ASCII fallback loses nothing semantic, because the word always renders.

**This table does not live in the TUI.** It renders in three places — the CLI's
human output, the TUI, and eventually D5's sysc-shell panel — and three
implementations of one vocabulary is exactly how a colour that means "risky" in
one surface and "selected" in another starts. It belongs in the engine, in
`internal/report`, as:

```go
func (g Grade) Style(profile termenv.Profile) Render
type Render struct { Colour termenv.Color; Glyph, ASCII, Word string }
```

The TUI and CLI use `internal/report`'s shared rendering. The sysc-shell panel
uses the existing `warning` wire tone, which the host maps to `ToneActivity`.

`lipgloss.SetColorProfile(termenv.Ascii)` in tests makes every golden render
deterministic.

## 6. Key bindings

| Key | Action |
|---|---|
| `↑` `↓` / `k` `j` | Move selection |
| `Enter` | Descend (push) |
| `Esc` / `←` / `h` | Back (pop) |
| `/` | Filter visible rows |
| `p` | Open `privacybit purge <entry-id>` for a red catalogue entry |
| `n` | Network screen |
| `c` | Coverage screen |
| `r` | Re-run the audit |
| `?` | Help |
| `q` | Quit, or pop if the stack is not at root |

The app and finding detail screens display grade evidence and available tier
0/3 instructions. Tier 1 dismissal and tier 2 settings are not exposed until
their storage and handlers exist.

## 7. Snapshot and refresh

The TUI holds one audit snapshot. `privacybit tui` scans at startup; `privacybit
report` opens the saved snapshot without scanning. In either case, `r` starts a
fresh audit and replaces the snapshot when it completes. Scheduled scans write
the saved snapshot and mark it unread; opening the report clears that marker
after the TUI exits successfully. Live telemetry attribution is not implemented
and renders as unavailable, never as an empty connection list.

## 8. Attended purge entry point

The TUI does not own sudo, a password, a helper protocol, or a purge-plan
screen. Pressing `p` on a red catalogue app starts the sibling CLI as
`privacybit purge <entry-id>` and cedes the terminal to it. The CLI prints the
complete plan, asks for the exact entry id on the controlling terminal, and
starts the helper only after confirmation. The helper enforces the single-use
review token independently. When the CLI exits, the TUI runs a fresh audit.

## 9. Purge review

The CLI implements D4's review flow: it shows the exact package removal set,
reverse dependencies, reclaim estimate, and expiry, then requires the exact
entry id on the controlling terminal. It sends no purge request when
confirmation is canceled or mismatched. The helper checks its in-memory token
and recomputed plan before mutation. The TUI delegates to this flow and never
treats its own UI as the authorization boundary.

## 10. Coverage, truncation, and `unknown`

Three states that are easy to render as nothing, and must not be.

**Coverage gaps.** A category whose package manager was unavailable is not an
empty category. It renders as `unavailable`, with the reason. D1 §6.4's
"network visibility degraded must report `unknown`, never green" applies to the
whole interface, not just one screen.

**Truncation.** D2 requires `installed_truncated` and `installed_total` on every
bounded list. The TUI renders `showing 100 of 163` rather than silently showing
one hundred, because "you have four browsers" and "I showed you four" are
different claims and the user cannot tell them apart otherwise.

**Unrated apps.** An installed app absent from the catalogue is `unknown` /
`Unrated` / `?` — a distinct visual state from `green`, not a greyed-out
`green`. It is not a finding and it must not sit in the grade counts looking
like one.

`ScreenCoverage` is the single place all three are enumerated. It exists because
the alternative is each of them being quietly absent, and an audit that hides
its own gaps is the failure mode D1 §0 was written to prevent.

## 11. Testing

moonbit's `internal/ui/ui_test.go` is 866 lines and 42 tests, and the method is
the house standard: drive `Update` with synthetic messages, assert on model
state and on `View` strings. No PTY, no terminal emulator.

The focused checks cover:

| Property | Method |
|---|---|
| Navigation | `TestNavigationPushAndPopPreserveParentSelection` checks stack behavior. |
| Stored report | `TestStoredSnapshotOpensWithoutScanningAndRefreshesOnRequest` checks stored startup and explicit refresh. |
| **Rendering is pure** | `TestViewDoesNotRunScan` verifies `View()` does not touch a detection source. |
| Coverage states render | `TestUnknownAndCoverageRenderAsExplicitStates` checks unavailable, unknown, and reduced coverage output. |
| Flatpak scope is retained | `TestAppRouteKeepsFlatpakScope` checks duplicate installation scopes remain addressable. |
| Purge delegates narrowly | `TestPurgeKeyLaunchesHelperFlowOnlyForRedCatalogueApps` checks only a known red app invokes the CLI flow. |

The purity test matters more than it looks. A `View()` that shells out is a
`View()` that runs on every keystroke, and in a TUI wired to a root helper that
is a very bad afternoon.

## 12. Package layout

```
internal/tui/         Bubble Tea model, screens, keybindings, layout
internal/runner/      bounded helper process session and shutdown
internal/helper/      strict privileged protocol and mutation boundary
internal/report/      snapshot types, storage, and shared grade rendering
```

`internal/runner` is the addition this design makes to the earlier plan. It was
implicitly assumed to live in the helper, but it must not: the helper is the
program *under* sudo, and the runner is the program *invoking* it. Putting the
runner inside the helper would mean the password-handling code ships in the
root binary, which is precisely where it must not be.

`internal/report` carries the grade rendering contract precisely because D2, D3
and D5 all read from it. If it lived in the TUI, D5 would reimplement it.

## 13. Open questions

1. **Should the filter (`/`) be fuzzy or substring?** The current implementation
   uses case-insensitive substring matching. Revisit only if users find it
   insufficient.
