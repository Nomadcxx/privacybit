package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Nomadcxx/privacybit/internal/helper"
)

func TestSessionWaitsForReadyThenExchangesOneBoundedRequest(t *testing.T) {
	script := `#!/bin/sh
printf '{"t":"ready","protocol":1,"catalog_rev":"cat-test"}\n'
IFS= read -r request || exit 2
test "$request" = '{"op":"ping"}' || exit 3
printf '{"t":"result","op":"ping","ok":true}\n'
`
	sudo := filepath.Join(t.TempDir(), "sudo")
	if err := os.WriteFile(sudo, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	session, err := Start(context.Background(), sudo, "privacybit-helper", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if got := session.Ready().CatalogRev; got != "cat-test" {
		t.Fatalf("ready catalogue revision = %q", got)
	}
	result, err := session.Request(helper.Request{Op: "ping"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "result" || result.Op != "ping" || !result.OK {
		t.Fatalf("ping response = %+v", result)
	}
}

func TestStopKillsHelperDescendantsAfterSudoExits(t *testing.T) {
	childFile := filepath.Join(t.TempDir(), "child.pid")
	script := fmt.Sprintf(`#!/bin/sh
sleep 30 &
echo "$!" > %q
printf '{"t":"ready","protocol":1,"catalog_rev":"cat-test"}\n'
wait
`, childFile)
	sudo := filepath.Join(t.TempDir(), "sudo")
	if err := os.WriteFile(sudo, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	session, err := Start(context.Background(), sudo, "privacybit-helper", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	data, err := os.ReadFile(childFile)
	if err != nil {
		t.Fatal(err)
	}
	var childPID int
	if _, err := fmt.Sscanf(string(data), "%d", &childPID); err != nil {
		t.Fatal(err)
	}
	if err := session.Stop(); err != nil {
		t.Fatal(err)
	}
	if processRunning(childPID) {
		t.Fatalf("helper descendant %d survived session stop", childPID)
	}
}

func TestProcessGroupKillTargetRefusesCallerGroup(t *testing.T) {
	callerGroup, err := syscall.Getpgid(0)
	if err != nil {
		t.Fatal(err)
	}
	target, err := processGroupKillTarget(callerGroup, callerGroup)
	if err == nil || target != 0 {
		t.Fatalf("target = %d, error = %v; want refusal", target, err)
	}
}

func processRunning(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	state := strings.Fields(string(data[strings.LastIndexByte(string(data), ')')+1:]))
	return len(state) > 0 && state[0] != "Z"
}
