// Package tui renders a read-only, keyboard-navigable view of an audit
// snapshot. Scanning is supplied by the caller and never happens in View.
package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/Nomadcxx/privacybit/internal/audit"
	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/report"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Screen string

const (
	ScreenScanning   Screen = "scanning"
	ScreenOverview   Screen = "overview"
	ScreenCategories Screen = "categories"
	ScreenCategory   Screen = "category"
	ScreenApp        Screen = "app"
	ScreenFindings   Screen = "findings"
	ScreenFinding    Screen = "finding"
	ScreenNetwork    Screen = "network"
	ScreenCoverage   Screen = "coverage"
	ScreenUnmatched  Screen = "unmatched"
	ScreenHelp       Screen = "help"
)

type ScanFunc func(context.Context) (report.Snapshot, error)

type Route struct {
	Screen    Screen
	ID        string
	Cursor    int
	Filter    string
	Filtering bool
}

type scanResult struct {
	snapshot report.Snapshot
	err      error
}

type purgeResult struct{ err error }

type row struct {
	id    string
	label string
	grade catalog.Grade
	kind  string
}

type Model struct {
	ctx         context.Context
	cancel      context.CancelFunc
	scan        ScanFunc
	snapshot    *report.Snapshot
	stack       []Route
	loading     bool
	scanErr     string
	actionErr   string
	width       int
	height      int
	purgeAction func(string) tea.Cmd
}

func NewModel(ctx context.Context, scan ScanFunc) *Model {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	model := &Model{
		ctx: ctx, cancel: cancel, scan: scan, stack: []Route{{Screen: ScreenScanning}},
		loading: true, width: 80, height: 24,
	}
	model.purgeAction = model.runPurge
	return model
}

// NewModelWithSnapshot opens an existing report without starting a scan.
func NewModelWithSnapshot(ctx context.Context, snapshot report.Snapshot, scan ScanFunc) *Model {
	model := NewModel(ctx, scan)
	model.snapshot = &snapshot
	model.stack = []Route{{Screen: ScreenOverview}}
	model.loading = false
	return model
}

func (m *Model) Init() tea.Cmd {
	if m.snapshot != nil {
		return nil
	}
	return m.runScan()
}

func (m *Model) runScan() tea.Cmd {
	if m.scan == nil {
		return func() tea.Msg { return scanResult{err: fmt.Errorf("audit scan is unavailable")} }
	}
	ctx, scan := m.ctx, m.scan
	return func() tea.Msg {
		snapshot, err := scan(ctx)
		return scanResult{snapshot: snapshot, err: err}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case scanResult:
		m.loading = false
		if msg.err != nil {
			m.scanErr = msg.err.Error()
			if len(m.stack) > 0 && m.stack[len(m.stack)-1].Screen == ScreenScanning {
				m.stack = []Route{{Screen: ScreenOverview}}
			}
			return m, nil
		}
		m.snapshot = &msg.snapshot
		m.scanErr = ""
		if len(m.stack) == 0 || m.stack[0].Screen == ScreenScanning {
			m.stack = []Route{{Screen: ScreenOverview}}
		}
		return m, nil
	case purgeResult:
		if msg.err != nil {
			m.actionErr = msg.err.Error()
			return m, nil
		}
		m.actionErr = ""
		m.loading = true
		return m, m.runScan()
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if len(m.stack) == 0 {
			m.stack = []Route{{Screen: ScreenOverview}}
		}
		route := &m.stack[len(m.stack)-1]
		if route.Filtering {
			switch msg.Type {
			case tea.KeyEsc:
				route.Filtering = false
				route.Filter = ""
			case tea.KeyEnter:
				route.Filtering = false
			case tea.KeyBackspace, tea.KeyDelete:
				if runes := []rune(route.Filter); len(runes) > 0 {
					route.Filter = string(runes[:len(runes)-1])
				}
			case tea.KeyRunes:
				route.Filter += string(msg.Runes)
			}
			m.clampCursor()
			return m, nil
		}
		switch msg.String() {
		case "q":
			if len(m.stack) > 1 {
				m.pop()
				return m, nil
			}
			if m.loading {
				m.cancelScan()
			}
			return m, tea.Quit
		case "esc", "left", "h":
			m.pop()
			return m, nil
		case "up", "k":
			route.Cursor--
			m.clampCursor()
		case "down", "j":
			route.Cursor++
			m.clampCursor()
		case "enter", "right", "l":
			m.enterSelected()
		case "/":
			route.Filter = ""
			route.Filtering = true
		case "r":
			if !m.loading {
				m.loading = true
				return m, m.runScan()
			}
		case "p":
			if !m.loading && route.Screen == ScreenApp && m.purgeAction != nil {
				if app := m.app(route.ID); app != nil && app.Grade == catalog.GradeRed && app.ID != "" {
					m.actionErr = ""
					return m, m.purgeAction(app.ID)
				}
			}
		case "n":
			m.push(Route{Screen: ScreenNetwork})
		case "c":
			m.push(Route{Screen: ScreenCoverage})
		case "?":
			m.push(Route{Screen: ScreenHelp})
		}
	}
	return m, nil
}

func (m *Model) runPurge(entry string) tea.Cmd {
	executable, err := os.Executable()
	if err != nil {
		return func() tea.Msg { return purgeResult{err: err} }
	}
	command := exec.CommandContext(m.ctx, executable, "purge", entry)
	return tea.ExecProcess(command, func(err error) tea.Msg { return purgeResult{err: err} })
}

func (m *Model) cancelScan() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Model) push(route Route) {
	if route.Screen == "" {
		return
	}
	m.stack = append(m.stack, route)
}

func (m *Model) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	} else if len(m.stack) == 0 {
		m.stack = []Route{{Screen: ScreenOverview}}
	}
}

func (m *Model) clampCursor() {
	if len(m.stack) == 0 {
		return
	}
	route := &m.stack[len(m.stack)-1]
	max := len(m.visibleRows(*route)) - 1
	if route.Cursor < 0 {
		route.Cursor = 0
	}
	if route.Cursor > max {
		route.Cursor = max
	}
}

func (m *Model) enterSelected() {
	if len(m.stack) == 0 || m.snapshot == nil {
		return
	}
	route := m.stack[len(m.stack)-1]
	rows := m.visibleRows(route)
	if route.Cursor < 0 || route.Cursor >= len(rows) {
		return
	}
	selected := rows[route.Cursor]
	switch route.Screen {
	case ScreenOverview:
		m.push(Route{Screen: Screen(selected.kind), ID: selected.id})
	case ScreenCategories:
		m.push(Route{Screen: ScreenCategory, ID: selected.id})
	case ScreenCategory:
		if selected.kind == "app" {
			m.push(Route{Screen: ScreenApp, ID: selected.id})
		}
	case ScreenUnmatched:
		if selected.kind == "unmatched" {
			m.push(Route{Screen: ScreenApp, ID: selected.id})
		}
	case ScreenFindings:
		m.push(Route{Screen: ScreenFinding, ID: selected.id})
	}
}

func (m *Model) visibleRows(route Route) []row {
	var rows []row
	if m.snapshot == nil {
		return rows
	}
	switch route.Screen {
	case ScreenOverview:
		rows = []row{
			{id: "categories", label: "Browse categories", kind: string(ScreenCategories)},
			{id: "findings", label: fmt.Sprintf("Findings (%d)", m.snapshot.Overall.Findings), kind: string(ScreenFindings)},
			{id: "network", label: "VPN and network", kind: string(ScreenNetwork)},
			{id: "coverage", label: "Coverage and reduced checks", kind: string(ScreenCoverage)},
		}
		if len(m.snapshot.Unmatched) > 0 {
			rows = append(rows, row{id: "unmatched", label: fmt.Sprintf("Unmatched apps (%d)", m.snapshot.UnmatchedTotal), kind: string(ScreenUnmatched)})
		}
		rows = append(rows, row{id: "help", label: "Help", kind: string(ScreenHelp)})
	case ScreenCategories:
		for _, category := range m.snapshot.Categories {
			rows = append(rows, row{id: category.ID, label: category.Name, grade: category.Grade, kind: "category"})
		}
	case ScreenCategory:
		if category := m.category(route.ID); category != nil {
			for _, app := range category.Installed {
				rows = append(rows, row{id: appRouteID(app), label: app.Name, grade: app.Grade, kind: "app"})
			}
			for _, expectation := range category.Expectations {
				state := "met"
				if !expectation.Judged {
					state = "unknown"
				} else if !expectation.Met {
					state = "missing"
				}
				rows = append(rows, row{id: expectation.ID, label: "Expectation " + state + ": " + expectation.Why, grade: expectation.Grade, kind: "expectation"})
			}
			if category.InstalledTruncated {
				rows = append(rows, row{label: fmt.Sprintf("Showing %d of %d installed items", category.InstalledShown, category.InstalledTotal), kind: "note"})
			}
		}
	case ScreenFindings:
		for _, finding := range m.snapshot.Findings {
			rows = append(rows, row{id: finding.ID, label: finding.Title, grade: finding.Grade, kind: "finding"})
		}
		if m.snapshot.FindingsTruncated {
			rows = append(rows, row{label: fmt.Sprintf("Showing %d of %d findings", len(m.snapshot.Findings), m.snapshot.FindingsTotal), kind: "note"})
		}
	case ScreenUnmatched:
		for _, app := range m.snapshot.Unmatched {
			rows = append(rows, row{id: unmatchedRouteID(app), label: app.Name, grade: catalog.GradeUnknown, kind: "unmatched"})
		}
		if m.snapshot.UnmatchedTruncated {
			rows = append(rows, row{label: fmt.Sprintf("Showing %d of %d unmatched apps", len(m.snapshot.Unmatched), m.snapshot.UnmatchedTotal), kind: "note"})
		}
	}
	if route.Filter != "" {
		filtered := rows[:0]
		needle := strings.ToLower(route.Filter)
		for _, item := range rows {
			if strings.Contains(strings.ToLower(item.label), needle) {
				filtered = append(filtered, item)
			}
		}
		rows = filtered
	}
	return rows
}

func (m *Model) category(id string) *report.Category {
	if m.snapshot == nil {
		return nil
	}
	for i := range m.snapshot.Categories {
		if m.snapshot.Categories[i].ID == id {
			return &m.snapshot.Categories[i]
		}
	}
	return nil
}

func (m *Model) View() string {
	if m.height <= 0 {
		return ""
	}
	if len(m.stack) == 0 {
		return "privacybit\nNo screen is available.\n"
	}
	route := m.stack[len(m.stack)-1]
	title := screenTitle(route.Screen)
	if route.ID != "" && route.Screen == ScreenCategory {
		if category := m.category(route.ID); category != nil {
			title = category.Name
		}
	}
	if route.ID != "" && route.Screen == ScreenApp {
		if app := m.app(route.ID); app != nil {
			title = app.Name
		}
	}
	if route.ID != "" && route.Screen == ScreenFinding {
		if finding := m.finding(route.ID); finding != nil {
			title = finding.Title
		}
	}

	var out strings.Builder
	brand := lipgloss.NewStyle().Bold(true).Render("privacybit")
	fmt.Fprintf(&out, "%s  %s\n", brand, title)
	if m.loading {
		fmt.Fprintln(&out, "Scanning…")
	} else if m.scanErr != "" {
		fmt.Fprintf(&out, "Scan unavailable: %s\n", m.scanErr)
	}
	if m.actionErr != "" {
		fmt.Fprintf(&out, "Action unavailable: %s\n", m.actionErr)
	}
	if m.snapshot != nil {
		grade := report.GradeChannels(m.snapshot.Overall.Grade)
		fmt.Fprintf(&out, "Overall: %s %s\n", grade.Glyph, grade.Word)
		fmt.Fprintf(&out, "As of %s\n", m.snapshot.GeneratedAt.Local().Format("15:04:05, 02 Jan 2006"))
		if m.snapshot.Overall.Score != nil {
			fmt.Fprintf(&out, "Progress %d/100 (not a quality score)\n", *m.snapshot.Overall.Score)
		}
	}
	if route.Filtering {
		fmt.Fprintf(&out, "Filter: %s▌\n", route.Filter)
	} else if route.Filter != "" {
		fmt.Fprintf(&out, "Filter: %s  (/ to edit)\n", route.Filter)
	}

	content := m.screenContent(route)
	if content != "" {
		fmt.Fprintln(&out, content)
	}
	fmt.Fprintln(&out, lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("↑/↓ move  Enter open  Esc back  p review purge on a red app  r refresh  n network  c coverage  ? help  q quit"))
	return out.String()
}

func (m *Model) screenContent(route Route) string {
	if m.snapshot == nil {
		if m.loading {
			return "Waiting for the first audit…"
		}
		return "No audit snapshot is available. Press r to retry."
	}
	switch route.Screen {
	case ScreenOverview, ScreenCategories, ScreenCategory, ScreenFindings, ScreenUnmatched:
		return m.listView(route)
	case ScreenApp:
		return m.appView(route.ID)
	case ScreenFinding:
		return m.findingView(route.ID)
	case ScreenNetwork:
		return m.networkView()
	case ScreenCoverage:
		return m.coverageView()
	case ScreenHelp:
		return "Grades use colour, shape, and word. Unknown means the check could not judge.\n" +
			"Findings are evidence with instructions; p on a red app opens a separate reviewed purge flow.\n" +
			"Press / to filter a list, Enter to inspect, Esc to return."
	default:
		return "No screen is available."
	}
}

func (m *Model) listView(route Route) string {
	rows := m.visibleRows(route)
	if len(rows) == 0 {
		if route.Screen == ScreenOverview {
			return "No categories or findings are available. Coverage explains what could be checked."
		}
		return "No rows match this view. Unknown and empty are different states."
	}
	maxRows := max(m.height-9, 3)
	start := max(route.Cursor-maxRows+1, 0)
	end := min(start+maxRows, len(rows))
	var out strings.Builder
	if start > 0 {
		fmt.Fprintf(&out, "… %d earlier\n", start)
	}
	for i := start; i < end; i++ {
		item := rows[i]
		prefix := "  "
		if i == route.Cursor {
			prefix = "› "
		}
		label := item.label
		if item.grade != "" {
			grade := report.GradeChannels(item.grade)
			colorIndex := "8"
			if grade.ANSI > 0 {
				colorIndex = fmt.Sprint(grade.ANSI - 30)
			}
			color := lipgloss.Color(colorIndex)
			label = lipgloss.NewStyle().Foreground(color).Render(grade.Glyph+" "+grade.Word) + "  " + label
		}
		style := lipgloss.NewStyle().MaxWidth(max(m.width-4, 20))
		if i == route.Cursor {
			style = style.Bold(true)
		}
		fmt.Fprintf(&out, "%s%s\n", prefix, style.Render(label))
	}
	if end < len(rows) {
		fmt.Fprintf(&out, "… %d more\n", len(rows)-end)
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func (m *Model) appView(id string) string {
	app := m.app(id)
	if app == nil {
		return "App details are unavailable."
	}
	var out strings.Builder
	grade := report.GradeChannels(app.Grade)
	fmt.Fprintf(&out, "%s %s\n", grade.Glyph, grade.Word)
	if app.Summary != "" {
		fmt.Fprintf(&out, "\n%s\n", app.Summary)
	}
	if len(app.Sources) > 0 {
		fmt.Fprintf(&out, "\nSources: %s\n", strings.Join(app.Sources, ", "))
	}
	if app.Scope != "" {
		fmt.Fprintf(&out, "Scope: %s\n", app.Scope)
	}
	for _, origin := range app.Origins {
		status := "not marked official"
		if origin.Official {
			status = "official"
		}
		fmt.Fprintf(&out, "Origin context: %s (%s)\n", origin.Source, status)
	}
	if app.OriginsTruncated {
		fmt.Fprintf(&out, "Showing %d of %d origins.\n", app.OriginsShown, app.OriginsTotal)
	}
	for _, why := range app.Why {
		fmt.Fprintf(&out, "\nWhy: %s\n", why)
	}
	for _, caveat := range app.Caveats {
		fmt.Fprintf(&out, "\nCaveat: %s\n", caveat)
	}
	if app.Grade == catalog.GradeRed {
		fmt.Fprintln(&out, "\nPress p to review a purge plan. The helper asks for exact confirmation.")
	}
	return strings.TrimSpace(out.String())
}

func (m *Model) app(id string) *report.App {
	if m.snapshot == nil {
		return nil
	}
	if strings.HasPrefix(id, "unmatched\x00") {
		parts := strings.Split(strings.TrimPrefix(id, "unmatched\x00"), "\x00")
		if len(parts) != 3 {
			return nil
		}
		for i := range m.snapshot.Unmatched {
			app := &m.snapshot.Unmatched[i]
			if app.Name == parts[0] && app.Scope == parts[1] && strings.Join(app.Sources, "\x00") == parts[2] {
				return app
			}
		}
		return nil
	}
	entryID, scope, _ := strings.Cut(id, "\x00")
	for i := range m.snapshot.Categories {
		for j := range m.snapshot.Categories[i].Installed {
			app := &m.snapshot.Categories[i].Installed[j]
			if app.ID == entryID && app.Scope == scope {
				return app
			}
		}
	}
	return nil
}

func appRouteID(app report.App) string {
	if app.Scope != "" {
		return app.ID + "\x00" + app.Scope
	}
	return app.ID
}

func unmatchedRouteID(app report.App) string {
	return "unmatched\x00" + app.Name + "\x00" + app.Scope + "\x00" + strings.Join(app.Sources, "\x00")
}

func (m *Model) finding(id string) *audit.Finding {
	if m.snapshot == nil {
		return nil
	}
	for i := range m.snapshot.Findings {
		if m.snapshot.Findings[i].ID == id {
			return &m.snapshot.Findings[i]
		}
	}
	return nil
}

func (m *Model) findingView(id string) string {
	finding := m.finding(id)
	if finding == nil {
		return "Finding details are unavailable."
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Severity: %s\n\n%s\n", finding.Severity, finding.Detail)
	for _, action := range finding.Actions {
		fmt.Fprintf(&out, "\nTier %d · %s: %s", action.Tier, action.Kind, action.Label)
		if action.Command != "" {
			fmt.Fprintf(&out, "\n  %s", action.Command)
		}
		if action.URL != "" {
			fmt.Fprintf(&out, "\n  %s", action.URL)
		}
	}
	return strings.TrimSpace(out.String())
}

func (m *Model) networkView() string {
	vpn := m.snapshot.Network.VPN
	var out strings.Builder
	fmt.Fprintf(&out, "VPN state: %s\n", vpn.State)
	if vpn.Client != "" {
		fmt.Fprintf(&out, "Client: %s\n", vpn.Client)
	}
	fmt.Fprintf(&out, "Default IPv4 route via tunnel: %s\n", boolText(vpn.DefaultRouteViaTunnel))
	fmt.Fprintf(&out, "DNS routed through tunnel: %s\n", boolText(vpn.DNSContained))
	fmt.Fprintf(&out, "IPv6 contained: %s\n", boolText(vpn.IPv6Contained))
	for _, defect := range vpn.Defects {
		fmt.Fprintf(&out, "\n%s\n%s\n", defect.Title, defect.Detail)
	}
	if vpn.KillSwitchNote != "" {
		fmt.Fprintf(&out, "\nKill-switch note: %s\n", vpn.KillSwitchNote)
	}
	if m.snapshot.Network.TelemetryConnectionsUnknown {
		fmt.Fprintln(&out, "\nLive telemetry connections: unavailable")
	}
	for _, reason := range vpn.Coverage {
		fmt.Fprintf(&out, "\nCoverage: %s\n", reason)
	}
	return strings.TrimSpace(out.String())
}

func boolText(value *bool) string {
	if value == nil {
		return "unknown"
	}
	if *value {
		return "yes"
	}
	return "no"
}

func (m *Model) coverageView() string {
	var out strings.Builder
	for _, source := range m.snapshot.Coverage {
		state := "ready"
		if !source.Ready {
			state = "unavailable: " + source.Reason
		} else if source.Empty {
			state = "ready, empty"
		}
		fmt.Fprintf(&out, "%s (%s): %s — %d observations\n", source.Name, source.Type, state, source.Observed)
	}
	for _, reduced := range m.snapshot.Reduced {
		fmt.Fprintf(&out, "\nReduced · %s: %s\n", reduced.Fact, reduced.Effect)
	}
	if m.snapshot.UnmatchedTruncated {
		fmt.Fprintf(&out, "\nUnmatched applications: showing %d of %d\n", len(m.snapshot.Unmatched), m.snapshot.UnmatchedTotal)
	}
	return strings.TrimSpace(out.String())
}

func screenTitle(screen Screen) string {
	switch screen {
	case ScreenScanning:
		return "Scanning"
	case ScreenOverview:
		return "Overview"
	case ScreenCategories:
		return "Categories"
	case ScreenCategory:
		return "Category"
	case ScreenApp:
		return "Application"
	case ScreenFindings:
		return "Findings"
	case ScreenFinding:
		return "Finding"
	case ScreenNetwork:
		return "VPN and network"
	case ScreenCoverage:
		return "Coverage"
	case ScreenUnmatched:
		return "Unmatched applications"
	case ScreenHelp:
		return "Help"
	default:
		return "Privacy audit"
	}
}

// Run starts the full-screen reader. The supplied scan function is called only
// on startup and when the user presses r.
func Run(ctx context.Context, in io.Reader, out io.Writer, scan ScanFunc) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return runModel(ctx, in, out, NewModel(ctx, scan))
}

// RunWithSnapshot opens a stored snapshot and refreshes only when the user
// requests a fresh audit.
func RunWithSnapshot(ctx context.Context, in io.Reader, out io.Writer, snapshot report.Snapshot, scan ScanFunc) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return runModel(ctx, in, out, NewModelWithSnapshot(ctx, snapshot, scan))
}

func runModel(ctx context.Context, in io.Reader, out io.Writer, model *Model) error {
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithInput(in), tea.WithOutput(out))
	_, err := program.Run()
	model.cancelScan()
	return err
}
