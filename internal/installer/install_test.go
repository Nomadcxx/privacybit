package installer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallWritesUserUnitsAndEnablesExactlyOneMode(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	binary := filepath.Join(root, "bin space%", "privacybit")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	var calls []string
	systemctl := func(_ context.Context, args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}
	if err := Install(context.Background(), configHome, binary, "watcher", systemctl); err != nil {
		t.Fatal(err)
	}
	wantWatcher := []string{
		"daemon-reload",
		"disable --now privacybit-scan.timer",
		"enable --now privacybit-daemon.service",
	}
	if strings.Join(calls, "\n") != strings.Join(wantWatcher, "\n") {
		t.Fatalf("watcher systemctl calls = %q, want %q", calls, wantWatcher)
	}
	unitDir := filepath.Join(configHome, "systemd", "user")
	for _, name := range []string{"privacybit-daemon.service", "privacybit-scan.service", "privacybit-scan.timer"} {
		info, err := os.Stat(filepath.Join(unitDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("%s mode = %o, want 644", name, info.Mode().Perm())
		}
	}
	watcher, err := os.ReadFile(filepath.Join(unitDir, "privacybit-daemon.service"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`ExecStart="` + filepath.Join(root, "bin space%%", "privacybit") + `" daemon`,
		"Conflicts=privacybit-scan.service privacybit-scan.timer",
		"ProtectHome=read-only",
		"ReadWritePaths=%S/privacybit",
		"NoNewPrivileges=yes",
	} {
		if !strings.Contains(string(watcher), expected) {
			t.Errorf("watcher unit missing %q:\n%s", expected, watcher)
		}
	}
	timer, err := os.ReadFile(filepath.Join(unitDir, "privacybit-scan.timer"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"OnCalendar=monthly", "Persistent=true", "Conflicts=privacybit-daemon.service"} {
		if !strings.Contains(string(timer), expected) {
			t.Errorf("timer unit missing %q:\n%s", expected, timer)
		}
	}

	calls = nil
	if err := Install(context.Background(), configHome, binary, "timer", systemctl); err != nil {
		t.Fatal(err)
	}
	wantTimer := []string{
		"daemon-reload",
		"disable --now privacybit-daemon.service",
		"enable --now privacybit-scan.timer",
	}
	if strings.Join(calls, "\n") != strings.Join(wantTimer, "\n") {
		t.Fatalf("timer systemctl calls = %q, want %q", calls, wantTimer)
	}
}

func TestInstallRejectsInvalidModeAndSymlinkUnitDirectory(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "privacybit")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runner := func(context.Context, ...string) error { calls++; return nil }
	if err := Install(context.Background(), filepath.Join(root, "invalid-mode"), binary, "other", runner); err == nil {
		t.Fatal("invalid mode was accepted")
	}
	if calls != 0 {
		t.Fatalf("invalid mode called systemctl %d times", calls)
	}

	configHome := filepath.Join(root, "config")
	if err := os.MkdirAll(configHome, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configHome, "systemd"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(configHome, "systemd", "user")); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), configHome, binary, "watcher", runner); err == nil {
		t.Fatal("symlink unit directory was accepted")
	}
	if calls != 0 {
		t.Fatalf("rejected install called systemctl %d times", calls)
	}
}

func TestInstallRejectsGroupWritableUnitDirectory(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	unitDir := filepath.Join(configHome, "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unitDir, 0o777); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "privacybit")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runner := func(context.Context, ...string) error { calls++; return nil }
	if err := Install(context.Background(), configHome, binary, "watcher", runner); err == nil {
		t.Fatal("group- and world-writable user unit directory was accepted")
	}
	if calls != 0 {
		t.Fatalf("rejected install called systemctl %d times", calls)
	}
}

func TestInstallRejectsGroupWritableUnitDirectoryAncestor(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	systemdDir := filepath.Join(configHome, "systemd")
	unitDir := filepath.Join(systemdDir, "user")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(systemdDir, 0o777); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "privacybit")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runner := func(context.Context, ...string) error { calls++; return nil }
	if err := Install(context.Background(), configHome, binary, "watcher", runner); err == nil {
		t.Fatal("group- and world-writable unit directory ancestor was accepted")
	}
	if calls != 0 {
		t.Fatalf("rejected install called systemctl %d times", calls)
	}
}

func TestValidateUnitDirReturnsCanonicalPathForOwnedSymlink(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config-real")
	unitDir := filepath.Join(configHome, "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configLink := filepath.Join(root, "config-link")
	if err := os.Symlink(configHome, configLink); err != nil {
		t.Fatal(err)
	}
	got, err := validateUnitDir(filepath.Join(configLink, "systemd", "user"))
	if err != nil {
		t.Fatal(err)
	}
	if got != unitDir {
		t.Fatalf("validated unit directory = %q, want canonical path %q", got, unitDir)
	}
}

func TestQuoteSystemdPathEscapesDollarExpansion(t *testing.T) {
	got, err := quoteSystemdPath("/home/user/$HOME/bin%/privacybit")
	if err != nil {
		t.Fatal(err)
	}
	if want := `"/home/user/$$HOME/bin%%/privacybit"`; got != want {
		t.Fatalf("quoted path = %q, want %q", got, want)
	}
}

func TestDisableStopsInstalledWatcherAndTimer(t *testing.T) {
	configHome := t.TempDir()
	unitDir := filepath.Join(configHome, "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"privacybit-daemon.service", "privacybit-scan.timer"} {
		if err := os.WriteFile(filepath.Join(unitDir, name), []byte("unit"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var calls []string
	runner := func(_ context.Context, args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}
	if err := Disable(context.Background(), configHome, runner); err != nil {
		t.Fatal(err)
	}
	want := "disable --now privacybit-daemon.service privacybit-scan.timer"
	if strings.Join(calls, "\n") != want {
		t.Fatalf("systemctl calls = %q, want %q", calls, want)
	}
}

func TestDisableDoesNothingWithoutUnitsAndRejectsSymlinkUnit(t *testing.T) {
	configHome := t.TempDir()
	calls := 0
	runner := func(context.Context, ...string) error { calls++; return nil }
	if err := Disable(context.Background(), configHome, runner); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("empty install called systemctl %d times", calls)
	}

	unitDir := filepath.Join(configHome, "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.service")
	if err := os.WriteFile(outside, []byte("unit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(unitDir, "privacybit-daemon.service")); err != nil {
		t.Fatal(err)
	}
	if err := Disable(context.Background(), configHome, runner); err == nil {
		t.Fatal("symlink unit was accepted")
	}
	if calls != 0 {
		t.Fatalf("rejected unit called systemctl %d times", calls)
	}
}

func TestDisableRejectsGroupWritableUnitDirectory(t *testing.T) {
	configHome := t.TempDir()
	unitDir := filepath.Join(configHome, "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unitDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "privacybit-daemon.service"), []byte("unit"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runner := func(context.Context, ...string) error { calls++; return nil }
	if err := Disable(context.Background(), configHome, runner); err == nil {
		t.Fatal("group- and world-writable user unit directory was accepted")
	}
	if calls != 0 {
		t.Fatalf("rejected directory called systemctl %d times", calls)
	}
}
