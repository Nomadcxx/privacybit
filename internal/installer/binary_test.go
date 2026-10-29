package installer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type cancelAfterSecondErrCheck struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *cancelAfterSecondErrCheck) Err() error {
	c.checks++
	if c.checks == 2 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestInstallBinaryReplacesOwnedExecutable(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "built-privacybit")
	if err := os.WriteFile(source, []byte("new executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(binDir, "privacybit")
	if err := os.WriteFile(target, []byte("old executable"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := InstallBinary(context.Background(), source, binDir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new executable" {
		t.Fatalf("installed contents = %q, want new executable", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("installed mode = %o, want 755", info.Mode().Perm())
	}
}

func TestInstallBinaryRejectsSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "built-privacybit")
	if err := os.WriteFile(source, []byte("new executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	referent := filepath.Join(root, "outside")
	if err := os.WriteFile(referent, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(referent, filepath.Join(binDir, "privacybit")); err != nil {
		t.Fatal(err)
	}

	if err := InstallBinary(context.Background(), source, binDir); err == nil {
		t.Fatal("symlink target was accepted")
	}
	data, err := os.ReadFile(referent)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep me" {
		t.Fatalf("symlink referent changed to %q", data)
	}
}

func TestInstallBinaryCancellationPreservesTarget(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "built-privacybit")
	if err := os.WriteFile(source, []byte("new executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(binDir, "privacybit")
	if err := os.WriteFile(target, []byte("existing install"), 0o755); err != nil {
		t.Fatal(err)
	}
	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelAfterSecondErrCheck{Context: baseCtx, cancel: cancel}

	if err := InstallBinary(ctx, source, binDir); err == nil {
		t.Fatal("install completed after cancellation before replacing the CLI")
	}
	if ctx.checks < 2 {
		t.Fatalf("install checked cancellation %d times, want a check before replacement", ctx.checks)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing install" {
		t.Fatalf("canceled install changed the CLI to %q", data)
	}
}

func TestInstallBuiltCLICancellationPreservesExistingBinary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "built-privacybit")
	if err := os.WriteFile(source, []byte("new executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(binDir, "privacybit")
	if err := os.WriteFile(target, []byte("existing install"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := installBuiltCLI(ctx, source, binDir); err == nil {
		t.Fatal("canceled install unexpectedly replaced the CLI")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing install" {
		t.Fatalf("canceled install changed the CLI to %q", data)
	}
}

func TestBuildAndInstallCLIFailurePreservesExistingBinary(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	checkout := filepath.Join(root, "checkout")
	commandDir := filepath.Join(checkout, "cmd", "privacybit")
	if err := os.MkdirAll(commandDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "go.mod"), []byte("module github.com/Nomadcxx/privacybit\n\ngo 1.26.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandDir, "main.go"), []byte("package main\nfunc main( {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(binDir, "privacybit")
	if err := os.WriteFile(target, []byte("existing install"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := BuildAndInstallCLI(context.Background(), checkout, binDir); err == nil {
		t.Fatal("invalid checkout unexpectedly built")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing install" {
		t.Fatalf("failed build changed installed binary to %q", data)
	}
}

func TestBuildAndInstallCLIInstallsRunnableBinaryOffline(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("find test source path")
	}
	checkout := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	moduleCache, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatalf("find module cache: %v", err)
	}
	t.Setenv("GOMODCACHE", strings.TrimSpace(string(moduleCache)))
	home := t.TempDir()
	t.Setenv("HOME", home)
	binDir, err := UserBinDir()
	if err != nil {
		t.Fatal(err)
	}
	goModBefore, err := os.ReadFile(filepath.Join(checkout, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goSumBefore, err := os.ReadFile(filepath.Join(checkout, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}

	installed, err := BuildAndInstallCLI(context.Background(), checkout, binDir)
	if err != nil {
		t.Fatal(err)
	}
	if installed != filepath.Join(binDir, "privacybit") {
		t.Fatalf("installed path = %q", installed)
	}
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("installed mode = %o, want 755", info.Mode().Perm())
	}
	output, err := exec.Command(installed, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("run installed CLI: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "privacybit") {
		t.Fatalf("help output does not identify privacybit:\n%s", output)
	}
	goModAfter, err := os.ReadFile(filepath.Join(checkout, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goSumAfter, err := os.ReadFile(filepath.Join(checkout, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if string(goModAfter) != string(goModBefore) || string(goSumAfter) != string(goSumBefore) {
		t.Fatal("offline install changed go.mod or go.sum")
	}
}

func TestEnsureUserBinDirRejectsSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".local")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	binDir, err := UserBinDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureUserBinDir(binDir); err == nil {
		t.Fatal("symlinked .local directory was accepted")
	}
	if _, err := os.Lstat(filepath.Join(outside, "bin")); !os.IsNotExist(err) {
		t.Fatalf("symlink target bin directory stat error = %v, want not exist", err)
	}
}

func TestOfflineBuildEnvDisablesNetworkAndOverridesGoSettings(t *testing.T) {
	env := offlineBuildEnv([]string{
		"GOPROXY=https://proxy.invalid",
		"GOSUMDB=sum.golang.org",
		"GOTOOLCHAIN=auto",
		"GOWORK=/tmp/go.work",
		"GOFLAGS=-mod=mod",
	})
	values := make(map[string]string)
	for _, entry := range env {
		key, value, found := strings.Cut(entry, "=")
		if found {
			values[key] = value
		}
	}
	for key, want := range map[string]string{
		"GOPROXY":     "off",
		"GOSUMDB":     "off",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
		"GOFLAGS":     "",
	} {
		if got := values[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}
