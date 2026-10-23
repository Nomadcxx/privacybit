// Command privacybit-install builds privacybit from the checkout and installs
// it for the current user.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Nomadcxx/privacybit/internal/installer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type step int

const (
	stepChoose step = iota
	stepReview
	stepBuilding
	stepServices
	stepDone
	stepFailed
)

type cliResult struct {
	path string
	err  error
}

type servicesResult struct{ err error }

type cliInstall func(context.Context) (string, error)
type serviceInstall func(context.Context, string, string) error

type model struct {
	ctx             context.Context
	installCLI      cliInstall
	installServices serviceInstall
	step            step
	selected        int
	installedPath   string
	problem         string
	width           int
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#176B4D", Dark: "#8BE3B7"})
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#5D6670", Dark: "#9CA3AF"})
	selectStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#175CD3", Dark: "#83B7FF"})
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B42318", Dark: "#FF938A"})
	frameStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
)

var asciiHeaderLines = []string{
	"\033[0;37m██▀██ ██▀██ ▀██▀ ██ ██ ██▀██ ██▀██ ██ ██ ██▀█▄ ▀██▀ ▀██▀\033[0m",
	"\033[0;37m██▄█▀ ██▄█▀  ██  ██ ██ ██▄██ ██    ██▄██ ██▄█▀  ██   ██ \033[0m",
	"\033[0;37m█▓░   █▓░█▓  █▓░ █▓░█▓ █▓░█▓ █▓░▄▄ ▄▄░█▓ █▓ █▓  █▓░  █▓░\033[0m",
	"\033[0;37m▀▀    ▀▀ ▀▀ ▀▀▀▀  ▀▀▀  ▀▀ ▀▀ ▀▀▀▀▀ ▀▀▀▀▀ ▀▀▀▀  ▀▀▀▀  ▀▀ \033[0m",
}

var serviceModes = []struct {
	name        string
	label       string
	description string
}{
	{name: "none", label: "No background work", description: "Run audits when you choose."},
	{name: "watcher", label: "Event watcher", description: "Notice new applications that may affect privacy."},
	{name: "timer", label: "Monthly scan", description: "Run the user scan timer once a month."},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "privacybit-install:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := requireUnprivileged(os.Geteuid()); err != nil {
		return err
	}
	checkout, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("find current directory: %w", err)
	}
	binDir, err := installer.UserBinDir()
	if err != nil {
		return err
	}
	installCLI := func(ctx context.Context) (string, error) {
		return installer.BuildAndInstallCLI(ctx, checkout, binDir)
	}
	installServices := func(ctx context.Context, path, mode string) error {
		configHome, err := installer.ConfigHome()
		if err != nil {
			return err
		}
		if mode == "none" {
			return installer.Disable(ctx, configHome, nil)
		}
		return installer.Install(ctx, configHome, path, mode, nil)
	}
	program := tea.NewProgram(newModel(context.Background(), installCLI, installServices), tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func requireUnprivileged(euid int) error {
	if euid == 0 {
		return errors.New("run this as your user, without sudo")
	}
	return nil
}

func newModel(ctx context.Context, install cliInstall, services serviceInstall) *model {
	if ctx == nil {
		ctx = context.Background()
	}
	return &model{ctx: ctx, installCLI: install, installServices: services, step: stepChoose, width: 80}
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		return m, nil
	case cliResult:
		if message.err != nil {
			m.problem = message.err.Error()
			m.step = stepFailed
			return m, nil
		}
		m.installedPath = message.path
		m.step = stepServices
		mode := serviceModes[m.selected].name
		return m, func() tea.Msg {
			return servicesResult{err: m.installServices(m.ctx, message.path, mode)}
		}
	case servicesResult:
		if message.err != nil {
			m.problem = message.err.Error()
			m.step = stepFailed
			return m, nil
		}
		m.step = stepDone
		return m, nil
	case tea.KeyMsg:
		if m.step == stepBuilding || m.step == stepServices {
			return m, nil
		}
		switch m.step {
		case stepChoose:
			switch message.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "up", "k":
				if m.selected > 0 {
					m.selected--
				}
			case "down", "j":
				if m.selected < len(serviceModes)-1 {
					m.selected++
				}
			case "enter":
				m.step = stepReview
			}
		case stepReview:
			switch message.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "esc", "backspace":
				m.step = stepChoose
			case "enter":
				m.step = stepBuilding
				return m, func() tea.Msg {
					path, err := m.installCLI(m.ctx)
					return cliResult{path: path, err: err}
				}
			}
		case stepDone, stepFailed:
			if message.String() == "enter" || message.String() == "q" || message.String() == "esc" || message.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *model) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	if width > 88 {
		width = 88
	}
	var content strings.Builder
	headerWidth := width - 10
	if headerWidth < 1 {
		headerWidth = 1
	}
	content.WriteString(lipgloss.NewStyle().Width(headerWidth).Align(lipgloss.Center).Render(strings.Join(asciiHeaderLines, "\n")))
	content.WriteString("\n\n")
	content.WriteString(titleStyle.Render("privacybit  /  INSTALL"))
	content.WriteString("\n\n")
	content.WriteString("Build and install the CLI for your user.\n")
	content.WriteString("Target: ~/.local/bin/privacybit\n\n")

	switch m.step {
	case stepChoose:
		content.WriteString("Choose background activity:\n")
		for i, mode := range serviceModes {
			if i == m.selected {
				content.WriteString(selectStyle.Render("> " + mode.label))
				content.WriteString("\n  " + mutedStyle.Render(mode.description))
			} else {
				content.WriteString("  " + mode.label)
				content.WriteString("\n  " + mutedStyle.Render(mode.description))
			}
			content.WriteString("\n\n")
		}
		content.WriteString(mutedStyle.Render("↑/↓ choose   Enter review   q quit"))
	case stepReview:
		content.WriteString("Review before installing:\n")
		content.WriteString("  Build privacybit from this checkout, offline\n")
		content.WriteString("  Install under ~/.local/bin\n")
		content.WriteString("  Background activity: " + serviceModes[m.selected].label + "\n\n")
		content.WriteString(mutedStyle.Render("Enter install   Esc back"))
	case stepBuilding:
		content.WriteString("Building privacybit offline, then installing the CLI...\n")
		content.WriteString(mutedStyle.Render("This can take a few seconds."))
	case stepServices:
		content.WriteString("CLI installed at:\n  " + m.installedPath + "\n\n")
		if serviceModes[m.selected].name == "none" {
			content.WriteString("Disabling any existing privacybit watcher or timer...")
		} else {
			content.WriteString("Configuring user " + serviceModes[m.selected].label + "...")
		}
	case stepDone:
		content.WriteString(selectStyle.Render("Install complete") + "\n\n")
		content.WriteString("privacybit is installed at:\n  " + m.installedPath + "\n")
		if serviceModes[m.selected].name == "none" {
			content.WriteString("No privacybit background service is active.\n")
		} else {
			content.WriteString("Enabled: " + serviceModes[m.selected].label + "\n")
		}
		content.WriteString("Run it with ~/.local/bin/privacybit, or add ~/.local/bin to PATH.\n")
		content.WriteString("\n" + mutedStyle.Render("Enter or q to exit"))
	case stepFailed:
		if m.installedPath != "" {
			content.WriteString("privacybit installed at:\n  " + m.installedPath + "\n\n")
			if serviceModes[m.selected].name == "none" {
				content.WriteString(errorStyle.Render("Could not disable existing privacybit background work.") + "\n")
			} else {
				content.WriteString(errorStyle.Render("User service setup is incomplete.") + "\n")
			}
		} else {
			content.WriteString(errorStyle.Render("Installation failed.") + "\n")
		}
		content.WriteString(errorStyle.Render(m.problem) + "\n\n")
		content.WriteString(mutedStyle.Render("Enter or q to exit"))
	}
	return frameStyle.Width(width - 4).BorderForeground(lipgloss.AdaptiveColor{Light: "#4C9B78", Dark: "#63C99A"}).Render(content.String())
}
