package detect

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// commandTimeout bounds one package-manager query. The slow tier is
// deliberately not given long: -Qq is ~100ms across 2,882 packages, and a
// package manager that has stopped responding is a coverage gap to report
// rather than a scan to hang on.
const commandTimeout = 20 * time.Second

// Runner executes external commands. Every source takes one rather than
// calling exec directly, so tests can substitute recorded output for a real
// pacman on a machine that may not have one.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout string, err error)
	LookPath(name string) (string, error)
}

// ExecRunner runs commands for real.
type ExecRunner struct{}

// Run executes name with args, bounded by commandTimeout, returning stdout.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// LookPath resolves name on PATH.
func (ExecRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }

// MapRunner replays canned output. Used by tests, and by the fixture-driven
// cross-distro CI that D6 §11 requires, so a Debian backend is exercised
// without a Debian host.
type MapRunner struct {
	// Output maps "name arg arg" to stdout. A key with no entry is an error,
	// never empty output, so an unfixtured call cannot masquerade as a
	// source with nothing to report.
	Output map[string]string
	// Err maps the same key to an error.
	Err map[string]error
	// Missing names LookPath should fail for. A name present in Output
	// resolves even when absent from Missing.
	Missing map[string]bool
}

// Run returns the canned stdout for the joined command.
func (m MapRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	if err, ok := m.Err[key]; ok && err != nil {
		return "", err
	}
	out, ok := m.Output[key]
	if !ok {
		return "", errUnfixtured(key)
	}
	return out, nil
}

// LookPath resolves name unless Missing says otherwise.
func (m MapRunner) LookPath(name string) (string, error) {
	if _, ok := m.Output[name]; ok && !strings.Contains(name, " ") {
		return "/usr/bin/" + name, nil
	}
	if m.Missing[name] {
		return "", exec.ErrNotFound
	}
	// A multi-word key is never a bare binary, so this is a miss.
	if strings.Contains(name, " ") {
		return "", exec.ErrNotFound
	}
	if _, err := os.Stat("/usr/bin/" + name); err == nil {
		return "/usr/bin/" + name, nil
	}
	return "", exec.ErrNotFound
}

// unfixturedError reports a command the fixture set does not describe.
type unfixturedError string

func (e unfixturedError) Error() string {
	return "command not fixtured: " + string(e)
}

func errUnfixtured(key string) error { return unfixturedError(key) }

// lines splits command output into non-empty trimmed lines.
func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
