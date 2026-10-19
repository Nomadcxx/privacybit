# privacybit — TUI design

**Date:** 2026-10-20
**Status:** approved in dialogue, pre-implementation
**Depends on:** [D1 engine](2026-10-20-privacybit-design.md),
[D2 CLI](2026-10-20-privacybit-cli-design.md),
[D4 helper](2026-10-20-privacybit-helper-design.md)

---

## 1. Scope

`privacybit tui`. A full-screen terminal interface for reading an audit,
navigating it, and acting on it.

In scope: screens, navigation, keybindings, grade rendering, live refresh, the
purge review flow, and the root-helper auth state machine.

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
| `overlayCenter(bg, fg, width)` | Modal overlays for plan review and auth. |
| `interpolateColor` / `parseHexColor` | Animated grade transitions, for free. |
| `isShiftX(msg)` | Key-variant helper. |
| `colors.go` palette shape | Named role variables, not inline colours. |
| `partialScanWarning(failures)` | Partial-failure disclosure, which privacybit needs more than moonbit does. |
| `tick()` / `tickMsg` | The periodic-refresh skeleton. |
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
    ScreenAuth      Screen = "auth"
    ScreenPlan      Screen = "plan"
    ScreenConfirm   Screen = "confirm"
    ScreenComplete  Screen = "complete"
    ScreenHelp      Screen = "help"
)
```

| Screen | Shows |
|---|---|
| `Overview` | Overall grade, score, counts per grade, the achievable wins, and the highest-severity findings. Answers "what should I do?" first. |
| `Scanning` | Progress per detection source, accumulating **coverage** as each completes. |
| `Categories` | Every category with its grade. The main index. |
| `Category` | One category: installed apps with grades, expectations met, expectations missing. |
| `App` | One app: why this grade, which sources detected it, caveats, channel/maturity, and its alternatives. |
| `Finding` | One finding: the evidence, the severity, and the actions available at each tier. |
| `Network` | VPN state and defects, plus live telemetry connections. |
| `Coverage` | What was scanned, what was not, what was truncated, and every `unknown` in the result. |
| `Auth` | Root helper state — prompting, failed, cancelled. Never shows the password. |
| `Plan` | A purge plan: exact removal set, reverse dependencies, reclaim, expiry. D4 §7. |
| `Confirm` | Generic two-phase confirm. |
| `Complete` | The outcome of an action. |
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

The TUI calls `grade.Style(profile)`. So does the CLI. So will the panel, once
D5 has the `warning` tone that D1 §8.1 identified as missing from plugin/v1.

`lipgloss.SetColorProfile(termenv.Ascii)` in tests makes every golden render
deterministic.

## 6. Key bindings

| Key | Action |
|---|---|
| `↑` `↓` / `k` `j` | Move selection |
| `g` `G` | Top / bottom |
| `Enter` | Descend (push) |
| `Esc` / `←` / `h` | Back (pop) |
| `/` | Filter visible rows |
| `e` | Explain the selected entry — why this grade, with sources |
| `a` | Actions for the selection |
| `d` | Dismiss the selected finding (tier 1) |
| `p` | Plan a purge for the selected app |
| `n` | Network screen |
| `c` | Coverage screen |
| `w` | Toggle live refresh |
| `r` | Re-run the audit |
| `?` | Help |
| `q` | Quit, or pop if the stack is not at root |

`e` is deliberate and load-bearing. D1 §5.4 says the catalogue encodes
opinions, and D1 §14 repeats it. A TUI that states a grade without offering the
reasoning immediately is making an unfalsifiable claim. One keypress must turn
any grade into its evidence.

`d` maps to tier 1 in D1 §9.1. Dismissal must be cheap and must not be buried,
because an audit that cannot be argued with is an audit that gets deleted.

## 7. The audit is a document; the fast poll is a liveness overlay

D1 §6.2 split the scan into a slow tier (package inventory, catalogue matching)
and a fast tier (network, VPN, sandbox posture). They have different cadences
and different costs, and the TUI must not blur them.

```go
type Model struct {
    audit   *report.Snapshot  // the slow document; changes only on re-run
    live    *report.Live      // the fast overlay; refreshed by tick
}
```

`renderNetwork` composes both: the VPN verdict comes from `audit`, but the
telemetry connection list and the "connected 40s ago" stamp come from `live` and
carry a "as of" timestamp.

The tick is moonbit's `tickMsg`, with one change: privacybit's tick refreshes
`live` only, and never re-runs the slow scan. Re-running is an explicit `r`.
That keeps a long-lived TUI from turning into a background scanner.

When `live` is unavailable — the flatpak network-visibility case in D1 §6.4,
where an unshared network namespace hides connections — the network screen
renders **`unavailable`**, never "no telemetry found". That distinction is the
single most important one in this screen, because conflating them is the
under-grade arriving through the UI.

## 8. Auth and the root helper

D4 owns the security model. The TUI's job is to drive it without touching a
password.

The sudo machinery in `plugins/moonbit/runner.go` becomes a reusable
`internal/runner` package in privacybit — the same code, minus the plugin
coupling. The TUI holds a `*runner.Handle` and reacts to events:

```
Idle → Start → WaitingForReady → AuthPrompting → Ready → Running → Done
                    ↓                  ↓
                 Timeout         WrongPassword (2nd prompt)
```

Two rules, both from moonbit's runner and both load-bearing:

- **The TUI never handles the password.** `runner` writes it to sudo's stdin and
  `clear`s it. No `tea.Model` field ever holds it, so it cannot be rendered,
  logged, or copied into a snapshot.
- **A second password prompt means failure, not a retry.** moonbit detects a
  wrong password by sudo asking twice rather than by parsing its complaint,
  which survives localisation and every sudo message variant. Replicated exactly.

`ScreenAuth` shows *that* authentication is happening and what it is for. It
never echoes, masks, or holds the password. It is a status screen, not an input
screen — because a TUI that owns a password field is a TUI that can leak it.

Cancelling uses moonbit's sequence exactly: `SIGALRM` to sudo, wait out its
`terminate_command`, then `SIGKILL` to the process group. Signalling sudo with
something it does not handle aborts its cleanup and orphans a root child.

## 9. Purge review

D4 §7 requires a token before `purge-entry`, and binds it to the entry, the plan
hash, the audit's `generated_at`, and an expiry. The TUI implements that shape:

1. `p` on a red app → `plan-purge` → `ScreenPlan`.
2. `ScreenPlan` renders the removal set, each package's reverse dependencies, the
   reclaim, and the token's expiry — all of it, not a summary of it.
3. `Enter` → `ScreenConfirm` → `purge-entry` with the token.

**The confirm here is a courtesy. D4 §7.2 is the control.** A `purge-entry`
without a valid token is refused by the helper regardless of what the UI did,
because D5's `privacybit:run-action` path bypasses this TUI entirely. The TUI
must not be written as though its confirm were load-bearing.

`ScreenPlan` is modal and cannot be dismissed by a stray keypress, following
moonbit's file-preview pattern (`TestFilePreviewPopupIsModal`).

## 10. Coverage, truncation, and `unknown`

Three states that are easy to render as nothing, and must not be.

**Coverage gaps.** A category whose package manager was unavailable is not an
empty category. It renders as `unavailable`, with the reason. D1 §6.4's
"network visibility degraded must report `unknown`, never green" applies to the
whole interface, not just one screen.

**Truncation.** D2 requires `installed_truncated` and `installed_total` on every
bounded list. The TUI renders `showing 20 of 63` rather than silently showing
twenty, because "you have four browsers" and "I showed you four" are different
claims and the user cannot tell them apart otherwise.

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

Matched, plus three additions privacybit needs:

| Property | Method |
|---|---|
| Navigation stack | Push, pop, push, pop; state at each depth is correct and scroll offsets are restored. |
| **Rendering is pure** | `TestRenderDoesNotAudit` — `View()` on a model with no audit must not touch a detection source. moonbit's `TestScheduleRenderDoesNotRunSystemctl` is the precedent; this is the stronger version. |
| Grade rendering is shared | Assert the CLI and the TUI produce the same colour, glyph and word for every grade. This is the test that stops the two front ends drifting. |
| ASCII fallback | Force `termenv.Ascii`; assert every grade still renders a glyph and a word. |
| Coverage states render | `unavailable`, `truncated`, `unknown` each have a golden render. |
| Live overlay does not rescan | A tick updates `live` and leaves `audit` pointer-identical. |
| Auth never holds a password | Struct-field reflection asserts no `string`/`[]byte` field on any model type ever contains one; and a test drives two prompts to `WrongPassword`. |
| Cancel orphans nothing | Cancel mid-privileged-op; assert no surviving process in the group. |
| Plan is modal and complete | `ScreenPlan` cannot be popped by a stray key, and contains every package in the removal set. |

The purity test matters more than it looks. A `View()` that shells out is a
`View()` that runs on every keystroke, and in a TUI wired to a root helper that
is a very bad afternoon.

## 12. Package layout

```
internal/ui/          Bubble Tea model, screens, keybindings, layout
internal/runner/      sudo machinery (moonbit's runner.go, de-pluginised)
internal/report/      snapshot types + the shared grade rendering contract
```

`internal/runner` is the addition this design makes to the earlier plan. It was
implicitly assumed to live in the helper, but it must not: the helper is the
program *under* sudo, and the runner is the program *invoking* it. Putting the
runner inside the helper would mean the password-handling code ships in the
root binary, which is precisely where it must not be.

`internal/report` carries the grade rendering contract precisely because D2, D3
and D5 all read from it. If it lived in the TUI, D5 would reimplement it.

## 13. Open questions

1. **Is a full-screen alt-screen TUI right, or should this be an inline
   pager-style view?** moonbit uses `WithAltScreen`, which is right for a tool
   you sit inside. privacybit might be a tool you run once and read once, where
   taking over the terminal is heavier than the content deserves. Worth
   deciding before 2,500 lines get written against the wrong interaction shape.
2. **Should the filter (`/`) be fuzzy or substring?** Substring is predictable
   and testable; fuzzy is friendlier across sixty catalogue ids. Deferred until
   it is felt to matter.
3. **Does dismissing a finding need a reason?** D1 §9.1 stores the reason.
   Prompting for one on every dismissal is friction; storing an empty one makes
   the record less useful. Probably optional with a default.
4. **How much of D4's root flow belongs in the TUI at all?** If most privileged
   actions are rare and consequential, a well-worded command line may be the
   better interface and the TUI may link to it. The `ScreenPlan` work is
   reusable either way.