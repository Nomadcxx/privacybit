// Package installer installs privacybit's user-scoped systemd watcher or timer.
package installer

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
)

//go:embed units/*
var unitFiles embed.FS

type SystemctlFunc func(context.Context, ...string) error

func ConfigHome() (string, error) {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		if !filepath.IsAbs(value) {
			return "", errors.New("XDG_CONFIG_HOME must be an absolute path")
		}
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}

func FindPrivacybit() (string, error) {
	path, err := exec.LookPath("privacybit")
	if err != nil {
		return "", fmt.Errorf("find privacybit on PATH: %w", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve privacybit path: %w", err)
	}
	return path, nil
}

func Install(ctx context.Context, configHome, binaryPath, mode string, systemctl SystemctlFunc) error {
	if mode != "watcher" && mode != "timer" {
		return fmt.Errorf("unknown mode %q (want watcher or timer)", mode)
	}
	if !filepath.IsAbs(configHome) {
		return errors.New("configuration home must be an absolute path")
	}
	if !filepath.IsAbs(binaryPath) {
		return errors.New("privacybit path must be absolute")
	}
	binaryInfo, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("stat privacybit executable: %w", err)
	}
	if !binaryInfo.Mode().IsRegular() || binaryInfo.Mode().Perm()&0o111 == 0 {
		return errors.New("privacybit path is not an executable file")
	}
	execPath, err := quoteSystemdPath(binaryPath)
	if err != nil {
		return err
	}
	unitDir := filepath.Join(configHome, "systemd", "user")
	if err := ensureUnitDir(unitDir); err != nil {
		return err
	}
	for _, name := range []string{"privacybit-daemon.service", "privacybit-scan.service", "privacybit-scan.timer"} {
		data, err := fs.ReadFile(unitFiles, "units/"+name)
		if err != nil {
			return fmt.Errorf("read embedded unit %s: %w", name, err)
		}
		data = []byte(strings.ReplaceAll(string(data), "@PRIVACYBIT@", execPath))
		if err := writeUnit(unitDir, name, data); err != nil {
			return fmt.Errorf("install %s: %w", name, err)
		}
	}
	if systemctl == nil {
		systemctl = CommandSystemctl
	}
	if err := systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if mode == "watcher" {
		if err := systemctl(ctx, "disable", "--now", "privacybit-scan.timer"); err != nil {
			return err
		}
		return systemctl(ctx, "enable", "--now", "privacybit-daemon.service")
	}
	if err := systemctl(ctx, "disable", "--now", "privacybit-daemon.service"); err != nil {
		return err
	}
	return systemctl(ctx, "enable", "--now", "privacybit-scan.timer")
}

func Disable(ctx context.Context, configHome string, systemctl SystemctlFunc) error {
	if !filepath.IsAbs(configHome) {
		return errors.New("configuration home must be an absolute path")
	}
	unitDir := filepath.Join(configHome, "systemd", "user")
	dirInfo, err := os.Lstat(unitDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect user unit directory: %w", err)
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(dirInfo) {
		return errors.New("user unit directory must be a real directory owned by the current user")
	}
	var units []string
	for _, name := range []string{"privacybit-daemon.service", "privacybit-scan.timer"} {
		info, err := os.Lstat(filepath.Join(unitDir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect user unit %s: %w", name, err)
		}
		if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
			return fmt.Errorf("user unit %s must be a regular file owned by the current user", name)
		}
		units = append(units, name)
	}
	if len(units) == 0 {
		return nil
	}
	if systemctl == nil {
		systemctl = CommandSystemctl
	}
	args := append([]string{"disable", "--now"}, units...)
	if err := systemctl(ctx, args...); err != nil {
		return fmt.Errorf("disable privacybit background work: %w", err)
	}
	return nil
}

func CommandSystemctl(ctx context.Context, args ...string) error {
	commandArgs := append([]string{"--user"}, args...)
	command := exec.CommandContext(ctx, "systemctl", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl --user %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func quoteSystemdPath(path string) (string, error) {
	for _, r := range path {
		if unicode.IsControl(r) {
			return "", errors.New("privacybit path contains a control character")
		}
	}
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(path)
	return `"` + escaped + `"`, nil
}

func ensureUnitDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create user unit directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("user unit path must be a real directory")
	}
	if !ownedByCurrentUser(info) {
		return errors.New("user unit directory is not owned by the current user")
	}
	return nil
}

func writeUnit(dir, name string, data []byte) error {
	path := filepath.Join(dir, name)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
			return errors.New("existing unit must be a regular file owned by the current user")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".privacybit-unit-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
