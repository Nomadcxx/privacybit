package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const modulePath = "github.com/Nomadcxx/privacybit"

func UserBinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find user home: %w", err)
	}
	if !filepath.IsAbs(home) {
		return "", errors.New("user home directory must be an absolute path")
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	info, err := os.Lstat(home)
	if err != nil {
		return "", fmt.Errorf("inspect user home: %w", err)
	}
	if !info.IsDir() || !ownedByCurrentUser(info) {
		return "", errors.New("user home must be a directory owned by the current user")
	}
	return filepath.Join(home, ".local", "bin"), nil
}

func BuildAndInstallCLI(ctx context.Context, checkout, binDir string) (string, error) {
	if !filepath.IsAbs(checkout) || !filepath.IsAbs(binDir) {
		return "", errors.New("checkout and install directory must be absolute paths")
	}
	moduleFile, err := os.ReadFile(filepath.Join(checkout, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read privacybit go.mod: %w", err)
	}
	if !strings.HasPrefix(string(moduleFile), "module "+modulePath+"\n") {
		return "", fmt.Errorf("%s is not a privacybit source checkout", checkout)
	}
	mainFile, err := os.Lstat(filepath.Join(checkout, "cmd", "privacybit", "main.go"))
	if err != nil || !mainFile.Mode().IsRegular() {
		return "", fmt.Errorf("%s does not contain cmd/privacybit/main.go", checkout)
	}
	expectedBinDir, err := UserBinDir()
	if err != nil {
		return "", err
	}
	if filepath.Clean(binDir) != filepath.Clean(expectedBinDir) {
		return "", errors.New("application install directory must be ~/.local/bin")
	}

	buildDir, err := os.MkdirTemp("", "privacybit-build-*")
	if err != nil {
		return "", fmt.Errorf("create temporary build directory: %w", err)
	}
	defer os.RemoveAll(buildDir)
	binary := filepath.Join(buildDir, "privacybit")
	command := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-o", binary, "./cmd/privacybit")
	command.Dir = checkout
	command.Env = offlineBuildEnv(os.Environ())
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build privacybit offline: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := installBuiltCLI(ctx, binary, binDir); err != nil {
		return "", fmt.Errorf("install privacybit: %w", err)
	}
	return filepath.Join(binDir, "privacybit"), nil
}

func installBuiltCLI(ctx context.Context, binary, binDir string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("installation canceled before replacing the CLI: %w", err)
	}
	if err := ensureUserBinDir(binDir); err != nil {
		return err
	}
	return InstallBinary(ctx, binary, binDir)
}

func InstallBinary(ctx context.Context, source, binDir string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("install binary canceled: %w", err)
	}
	if !filepath.IsAbs(source) || !filepath.IsAbs(binDir) {
		return errors.New("binary source and install directory must be absolute paths")
	}
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect built privacybit binary: %w", err)
	}
	if !sourceInfo.Mode().IsRegular() || sourceInfo.Mode().Perm()&0o111 == 0 {
		return errors.New("built privacybit path is not a regular executable file")
	}
	if !ownedByCurrentUser(sourceInfo) {
		return errors.New("built privacybit binary is not owned by the current user")
	}
	dirInfo, err := os.Lstat(binDir)
	if err != nil {
		return fmt.Errorf("inspect binary install directory: %w", err)
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("binary install path must be a real directory")
	}
	if !ownedByCurrentUser(dirInfo) {
		return errors.New("binary install directory is not owned by the current user")
	}
	if dirInfo.Mode().Perm()&0o022 != 0 {
		return errors.New("binary install directory must not be group- or world-writable")
	}
	target := filepath.Join(binDir, "privacybit")
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
			return errors.New("existing privacybit target must be a regular file owned by the current user")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect existing privacybit binary: %w", err)
	}

	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open built privacybit binary: %w", err)
	}
	defer input.Close()
	temporary, err := os.CreateTemp(binDir, ".privacybit-install-*")
	if err != nil {
		return fmt.Errorf("create temporary install file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, input); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("copy privacybit binary: %w", err)
	}
	if err := temporary.Chmod(0o755); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set installed binary permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync privacybit binary: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary install file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("install binary canceled: %w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("replace privacybit binary: %w", err)
	}
	// ponytail: sync file contents before the atomic rename; reinstalling this
	// recoverable executable is enough after sudden power loss. Add a directory
	// sync if crash-durable rename metadata becomes a project requirement.
	return nil
}

func ensureUserBinDir(binDir string) error {
	expected, err := UserBinDir()
	if err != nil {
		return err
	}
	if filepath.Clean(binDir) != filepath.Clean(expected) {
		return errors.New("application install directory must be ~/.local/bin")
	}
	localDir := filepath.Dir(binDir)
	for _, path := range []string{localDir, binDir} {
		if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create user binary directory %s: %w", path, err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect user binary directory %s: %w", path, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("user binary path %s must be a real directory", path)
		}
		if !ownedByCurrentUser(info) {
			return fmt.Errorf("user binary directory %s is not owned by the current user", path)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("user binary directory %s must not be group- or world-writable", path)
		}
	}
	return nil
}

func offlineBuildEnv(env []string) []string {
	for _, setting := range []string{"GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS="} {
		key := strings.SplitN(setting, "=", 2)[0] + "="
		filtered := env[:0]
		for _, entry := range env {
			if !strings.HasPrefix(entry, key) {
				filtered = append(filtered, entry)
			}
		}
		env = append(filtered, setting)
	}
	return env
}
