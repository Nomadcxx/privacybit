package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInstallerHeaderMatchesOfficialWordmark(t *testing.T) {
	asset, err := os.ReadFile("../../assets/privacybit-header.conf")
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "ansi_style=true\nascii_1=\"\"\"\n"
	const suffix = "\n\"\"\"\n"
	if !strings.HasPrefix(string(asset), prefix) || !strings.HasSuffix(string(asset), suffix) {
		t.Fatal("official wordmark is not a valid ANSI header profile")
	}
	body := strings.TrimSuffix(strings.TrimPrefix(string(asset), prefix), suffix)
	want := strings.ReplaceAll(strings.Join(wordmarkLines, "\n"), "\x1b", `\033`)
	if body != want {
		t.Fatalf("installer header differs from assets/privacybit-header.conf:\n got: %q\nwant: %q", body, want)
	}
}

func TestInstallerWordmarkFallsBackOnNarrowTerminal(t *testing.T) {
	model := newModel(context.Background(), nil, nil)
	model.width = 50
	view := model.View()
	if strings.Contains(view, "██") {
		t.Fatalf("wide wordmark was rendered into a narrow terminal:\n%s", view)
	}
	titleIndex := strings.Index(view, "privacybit  /  INSTALL")
	if titleIndex < 0 || !strings.Contains(view[:titleIndex], "privacybit") {
		t.Fatalf("narrow header lost the product name:\n%s", view)
	}
}

func TestInstallerShowsWordmarkAtNormalTerminalWidth(t *testing.T) {
	model := newModel(context.Background(), nil, nil)
	model.width = 80
	if view := model.View(); !strings.Contains(view, wordmarkLines[0]) {
		t.Fatalf("normal-width installer is missing the official wordmark:\n%s", view)
	}
}

func TestInstallWaitsForReviewAndUsesSelectedServiceMode(t *testing.T) {
	var buildCalls, serviceCalls int
	model := newModel(context.Background(),
		func(context.Context) (string, error) {
			buildCalls++
			return "/home/test/.local/bin/privacybit", nil
		},
		func(_ context.Context, _, mode string) error {
			serviceCalls++
			if mode != "watcher" {
				t.Fatalf("service mode = %q, want watcher", mode)
			}
			return nil
		},
	)

	model, command := update(model, tea.KeyMsg{Type: tea.KeyDown})
	if command != nil {
		t.Fatal("selection changed before install review")
	}
	model, command = update(model, tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil || buildCalls != 0 || serviceCalls != 0 {
		t.Fatal("install work started before review confirmation")
	}
	model, command = update(model, tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil || buildCalls != 0 || serviceCalls != 0 {
		t.Fatal("review confirmation did not schedule install work")
	}
	model, command = update(model, command())
	if buildCalls != 1 || serviceCalls != 0 || command == nil {
		t.Fatalf("after build: build calls=%d service calls=%d command=%v", buildCalls, serviceCalls, command)
	}
	model, command = update(model, command())
	if model.step != stepDone || buildCalls != 1 || serviceCalls != 1 || command != nil {
		t.Fatalf("final state: step=%v build calls=%d service calls=%d command=%v", model.step, buildCalls, serviceCalls, command)
	}
}

func TestNoBackgroundChoiceDisablesUnitsAfterBuild(t *testing.T) {
	var buildCalls, serviceCalls int
	model := newModel(context.Background(),
		func(context.Context) (string, error) {
			buildCalls++
			return "/home/test/.local/bin/privacybit", nil
		},
		func(_ context.Context, _, mode string) error {
			serviceCalls++
			if mode != "none" {
				t.Fatalf("service mode = %q, want none", mode)
			}
			return nil
		},
	)
	model, _ = update(model, tea.KeyMsg{Type: tea.KeyEnter})
	model, command := update(model, tea.KeyMsg{Type: tea.KeyEnter})
	model, command = update(model, command())
	if buildCalls != 1 || serviceCalls != 0 || command == nil {
		t.Fatalf("after build: build calls=%d service calls=%d command=%v", buildCalls, serviceCalls, command)
	}
	model, command = update(model, command())
	if model.step != stepDone || serviceCalls != 1 || command != nil {
		t.Fatalf("after disabling: step=%v service calls=%d command=%v", model.step, serviceCalls, command)
	}
}

func TestServiceFailureReportsInstalledBinaryAsPartialResult(t *testing.T) {
	model := newModel(context.Background(),
		func(context.Context) (string, error) { return "/home/test/.local/bin/privacybit", nil },
		func(context.Context, string, string) error { return errors.New("systemctl unavailable") },
	)
	model, _ = update(model, tea.KeyMsg{Type: tea.KeyDown})
	model, _ = update(model, tea.KeyMsg{Type: tea.KeyEnter})
	model, command := update(model, tea.KeyMsg{Type: tea.KeyEnter})
	model, command = update(model, command())
	model, command = update(model, command())
	if command != nil || model.step != stepFailed {
		t.Fatalf("state after service failure: step=%v command=%v", model.step, command)
	}
	view := model.View()
	for _, expected := range []string{"privacybit installed at:", "/home/test/.local/bin/privacybit", "service setup is incomplete", "systemctl unavailable"} {
		if !strings.Contains(view, expected) {
			t.Errorf("failure view missing %q:\n%s", expected, view)
		}
	}
}

func TestCtrlCCancelsBuildAndReportsCLIState(t *testing.T) {
	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	started := make(chan struct{})
	serviceCalled := false
	model := newModel(rootCtx,
		func(ctx context.Context) (string, error) {
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		},
		func(context.Context, string, string) error {
			serviceCalled = true
			return nil
		},
	)
	model, _ = update(model, tea.KeyMsg{Type: tea.KeyEnter})
	model, command := update(model, tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("review confirmation did not start the build")
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- command() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("build callback did not start")
	}

	model, command = update(model, tea.KeyMsg{Type: tea.KeyCtrlC})
	if command != nil || !strings.Contains(strings.ToLower(model.View()), "cancel") {
		t.Fatalf("Ctrl+C did not show cancellation progress:\n%s", model.View())
	}
	var message tea.Msg
	select {
	case message = <-result:
	case <-time.After(time.Second):
		t.Fatal("Ctrl+C did not cancel the active build")
	}
	model, _ = update(model, message)
	if model.step != stepFailed || serviceCalled {
		t.Fatalf("canceled build state = %v, service called=%v", model.step, serviceCalled)
	}
	if view := strings.ToLower(model.View()); !strings.Contains(view, "canceled") {
		t.Fatalf("final view does not report cancellation:\n%s", model.View())
	}
}

func TestCtrlCCancelsServiceSetupAndReportsPartialState(t *testing.T) {
	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	started := make(chan struct{})
	model := newModel(rootCtx,
		func(context.Context) (string, error) { return "/home/test/.local/bin/privacybit", nil },
		func(ctx context.Context, _, _ string) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	)
	model, _ = update(model, tea.KeyMsg{Type: tea.KeyDown})
	model, _ = update(model, tea.KeyMsg{Type: tea.KeyEnter})
	model, command := update(model, tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("review confirmation did not start the build")
	}
	model, command = update(model, command())
	if command == nil || model.step != stepServices {
		t.Fatalf("successful build state = %v, command=%v", model.step, command)
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- command() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("service callback did not start")
	}

	model, command = update(model, tea.KeyMsg{Type: tea.KeyCtrlC})
	if command != nil || !strings.Contains(strings.ToLower(model.View()), "cancel") {
		t.Fatalf("Ctrl+C did not show cancellation progress:\n%s", model.View())
	}
	select {
	case message := <-result:
		model, _ = update(model, message)
	case <-time.After(time.Second):
		t.Fatal("Ctrl+C did not cancel service setup")
	}
	if model.step != stepFailed || model.installedPath != "/home/test/.local/bin/privacybit" {
		t.Fatalf("canceled service state = %v, installed path=%q", model.step, model.installedPath)
	}
	view := strings.ToLower(model.View())
	for _, expected := range []string{"service setup was canceled", "service state may be partial", model.installedPath} {
		if !strings.Contains(view, strings.ToLower(expected)) {
			t.Errorf("final view missing %q:\n%s", expected, model.View())
		}
	}
}

func TestCtrlCAfterBuildCompletionDoesNotStartServices(t *testing.T) {
	const installedPath = "/home/test/.local/bin/privacybit"
	serviceCalled := false
	model := newModel(context.Background(),
		func(context.Context) (string, error) { return installedPath, nil },
		func(context.Context, string, string) error {
			serviceCalled = true
			return nil
		},
	)
	model, _ = update(model, tea.KeyMsg{Type: tea.KeyEnter})
	model, command := update(model, tea.KeyMsg{Type: tea.KeyEnter})
	model, _ = update(model, tea.KeyMsg{Type: tea.KeyCtrlC})
	model, command = update(model, command())
	if command != nil || model.step != stepFailed || serviceCalled {
		t.Fatalf("late build completion state = %v, command=%v, service called=%v", model.step, command, serviceCalled)
	}
	view := model.View()
	for _, expected := range []string{installedPath, "Background service setup did not start."} {
		if !strings.Contains(view, expected) {
			t.Errorf("final view missing %q:\n%s", expected, view)
		}
	}
}

func TestRootInstallIsRejected(t *testing.T) {
	if err := requireUnprivileged(0); err == nil {
		t.Fatal("root execution was accepted")
	}
	if err := requireUnprivileged(1000); err != nil {
		t.Fatalf("unprivileged execution was rejected: %v", err)
	}
}

func update(m *model, message tea.Msg) (*model, tea.Cmd) {
	updated, command := m.Update(message)
	return updated.(*model), command
}
