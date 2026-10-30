package helper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/privacybit/internal/catalog"
)

type call struct {
	name string
	args []string
}

type fakeRunner struct {
	calls  []call
	prints []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	r.calls = append(r.calls, call{name: name, args: append([]string(nil), args...)})
	joined := strings.Join(append([]string{name}, args...), " ")
	switch {
	case joined == "pacman -Qq":
		return "google-chrome-stable\nextra-dep\nnew-dep\n", nil
	case strings.HasPrefix(joined, "pacman -Rs --print --print-format=%n --"):
		if len(r.prints) == 0 {
			return "google-chrome-stable\nextra-dep\n", nil
		}
		output := r.prints[0]
		r.prints = r.prints[1:]
		return output, nil
	case strings.HasPrefix(joined, "pacman -Qi --"):
		name := args[len(args)-1]
		return fmt.Sprintf("Name            : %s\nInstalled Size  : 10.00 MiB\nRequired By     : None\n", name), nil
	case strings.HasPrefix(joined, "pacman -Rn --noconfirm --"):
		return "", nil
	default:
		return "", fmt.Errorf("unexpected command %q", joined)
	}
}

func TestStatusReportsDevelopmentBuildVersionAndCatalogueRevision(t *testing.T) {
	set := testCatalog(t)
	var output bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader("{\"op\":\"status\"}\n"), &output, Options{Catalog: set}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("responses = %d, want ready and status: %s", len(lines), output.String())
	}
	var status Event
	if err := json.Unmarshal([]byte(lines[1]), &status); err != nil {
		t.Fatal(err)
	}
	if status.Type != "result" || status.Op != "status" || !status.OK {
		t.Fatalf("status response = %+v", status)
	}
	if status.Version != "devel" {
		t.Errorf("version = %q, want devel for a source build", status.Version)
	}
	if status.CatalogRev != set.Revision {
		t.Errorf("catalogue revision = %q, want %q", status.CatalogRev, set.Revision)
	}
}

func TestServeRejectsUnknownFieldBeforeExecutingAnything(t *testing.T) {
	set := testCatalog(t)
	runner := &fakeRunner{}
	var output bytes.Buffer
	input := "{\"op\":\"set-unit\",\"unit\":\"fwupd-refresh.timer\",\"state\":\"disabled\",\"argv\":[]}\n"
	err := Serve(context.Background(), strings.NewReader(input), &output, Options{
		Catalog: set, Runner: runner, IsRoot: func() bool { return true },
	})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Serve error = %v, want unknown field", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("invalid request executed commands: %#v", runner.calls)
	}
}

func TestParseSizeRejectsNaN(t *testing.T) {
	if _, err := parseSize("NaN KiB"); err == nil {
		t.Fatal("parseSize accepted a non-finite installed size")
	}
}

func TestPlanTokenIsSingleUseAndSessionBound(t *testing.T) {
	set := testCatalog(t)
	runner := &fakeRunner{}
	var output bytes.Buffer
	auditAt := "2026-10-20T12:00:00Z"
	input := "{\"op\":\"plan-purge\",\"entry\":\"chrome\",\"audit_generated_at\":\"" + auditAt + "\"}\n"
	err := Serve(context.Background(), strings.NewReader(input), &output, Options{
		Catalog: set, Runner: runner, IsRoot: func() bool { return true },
		Now:   func() time.Time { return time.Date(2026, 10, 20, 12, 1, 0, 0, time.UTC) },
		Token: func() (string, error) { return strings.Repeat("a", 64), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("responses = %d, want ready and plan: %s", len(lines), output.String())
	}
	var ready struct {
		Type string `json:"t"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &ready); err != nil || ready.Type != "ready" {
		t.Fatalf("first response = %s, error %v", lines[0], err)
	}
	var planned struct {
		Plan Plan `json:"plan"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &planned); err != nil {
		t.Fatal(err)
	}
	if planned.Plan.Token == "" || planned.Plan.ReclaimBytes == 0 {
		t.Fatalf("plan missing token or reclaim: %+v", planned.Plan)
	}
	if !contains(runner.calls, "pacman", "-Rs", "--print", "--print-format=%n", "--", "google-chrome-stable") {
		t.Fatalf("plan did not use read-only removal simulation: %#v", runner.calls)
	}
	if contains(runner.calls, "pacman", "-Rn", "--noconfirm", "--", "google-chrome-stable", "extra-dep") {
		t.Fatalf("planning removed packages: %#v", runner.calls)
	}

	// A token is meaningful only inside the session that created it. Replaying
	// this request in a new session must be rejected without invoking pacman.
	var replay bytes.Buffer
	request := fmt.Sprintf("{\"op\":\"purge-entry\",\"entry\":\"chrome\",\"token\":%q,\"audit_generated_at\":%q}\n", planned.Plan.Token, auditAt)
	if err := Serve(context.Background(), strings.NewReader(request), &replay, Options{
		Catalog: set, Runner: runner, IsRoot: func() bool { return true },
	}); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("replay made new command calls: %#v", runner.calls)
	}
}

func TestChangedPlanRefusesPurge(t *testing.T) {
	set := testCatalog(t)
	runner := &fakeRunner{prints: []string{
		"google-chrome-stable\nextra-dep\n",
		"google-chrome-stable\nextra-dep\nnew-dep\n",
	}}
	var output bytes.Buffer
	auditAt := "2026-10-20T12:00:00Z"
	planRequest := "{\"op\":\"plan-purge\",\"entry\":\"chrome\",\"audit_generated_at\":\"" + auditAt + "\"}\n"
	purgeRequest := "{\"op\":\"purge-entry\",\"entry\":\"chrome\",\"token\":\"" + strings.Repeat("b", 64) + "\",\"audit_generated_at\":\"" + auditAt + "\"}\n"
	err := Serve(context.Background(), strings.NewReader(planRequest+purgeRequest), &output, Options{
		Catalog: set, Runner: runner, IsRoot: func() bool { return true },
		Now:   func() time.Time { return time.Date(2026, 10, 20, 12, 1, 0, 0, time.UTC) },
		Token: func() (string, error) { return strings.Repeat("b", 64), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if contains(runner.calls, "pacman", "-Rn", "--noconfirm", "--", "google-chrome-stable", "extra-dep") {
		t.Fatalf("changed plan was executed: %#v", runner.calls)
	}
	if !strings.Contains(output.String(), "plan changed") {
		t.Fatalf("response does not explain refusal: %s", output.String())
	}
}

func TestConfirmedPurgeExecutesReviewedSetOnce(t *testing.T) {
	set := testCatalog(t)
	runner := &fakeRunner{}
	var output bytes.Buffer
	var records []AuditRecord
	auditAt := "2026-10-20T12:00:00Z"
	token := strings.Repeat("c", 64)
	planRequest := "{\"op\":\"plan-purge\",\"entry\":\"chrome\",\"audit_generated_at\":\"" + auditAt + "\"}\n"
	purgeRequest := "{\"op\":\"purge-entry\",\"entry\":\"chrome\",\"token\":\"" + token + "\",\"audit_generated_at\":\"" + auditAt + "\"}\n"
	err := Serve(context.Background(), strings.NewReader(planRequest+purgeRequest+purgeRequest), &output, Options{
		Catalog: set, Runner: runner, IsRoot: func() bool { return true },
		Now:   func() time.Time { return time.Date(2026, 10, 20, 12, 1, 0, 0, time.UTC) },
		Token: func() (string, error) { return token, nil },
		Log:   func(record AuditRecord) error { records = append(records, record); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(runner.calls, "pacman", "-Rn", "--noconfirm", "--", "extra-dep", "google-chrome-stable") {
		t.Fatalf("reviewed explicit package set was not executed: %#v", runner.calls)
	}
	if len(records) != 2 || records[0].Result != "started" || records[1].Result != "completed" {
		t.Fatalf("mutation log records = %+v", records)
	}
	if countCalls(runner.calls, "pacman", "-Rn", "--noconfirm", "--", "extra-dep", "google-chrome-stable") != 1 {
		t.Fatalf("one-use token executed more than once: %#v", runner.calls)
	}
}

func TestExpiredPlanDoesNotRecomputeOrExecute(t *testing.T) {
	set := testCatalog(t)
	runner := &fakeRunner{}
	var output bytes.Buffer
	callIndex := 0
	base := time.Date(2026, 10, 20, 12, 1, 0, 0, time.UTC)
	now := func() time.Time {
		callIndex++
		if callIndex <= 2 {
			return base
		}
		return base.Add(TokenWindow + time.Second)
	}
	auditAt := "2026-10-20T12:00:00Z"
	token := strings.Repeat("d", 64)
	planRequest := "{\"op\":\"plan-purge\",\"entry\":\"chrome\",\"audit_generated_at\":\"" + auditAt + "\"}\n"
	purgeRequest := "{\"op\":\"purge-entry\",\"entry\":\"chrome\",\"token\":\"" + token + "\",\"audit_generated_at\":\"" + auditAt + "\"}\n"
	err := Serve(context.Background(), strings.NewReader(planRequest+purgeRequest), &output, Options{
		Catalog: set, Runner: runner, IsRoot: func() bool { return true }, Now: now,
		Token: func() (string, error) { return token, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 4 || !strings.Contains(output.String(), "review token expired") {
		t.Fatalf("expired plan made extra calls or lacked explanation: calls=%#v output=%s", runner.calls, output.String())
	}
}

func TestDecodeRequestRejectsDuplicateKeys(t *testing.T) {
	_, err := decodeRequest([]byte(`{"op":"ping","op":"status"}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("decode error = %v, want duplicate-key rejection", err)
	}
}

func testCatalog(t *testing.T) *catalog.Set {
	t.Helper()
	set, err := catalog.Load(catalog.LoadOptions{UserFragments: false})
	if err != nil {
		t.Fatal(err)
	}
	if set.Entry("chrome") == nil {
		t.Fatal("test catalogue entry missing")
	}
	return set
}

func contains(calls []call, name string, args ...string) bool {
	for _, item := range calls {
		if item.name != name || len(item.args) != len(args) {
			continue
		}
		match := true
		for i := range args {
			if item.args[i] != args[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func countCalls(calls []call, name string, args ...string) int {
	count := 0
	for _, item := range calls {
		if contains([]call{item}, name, args...) {
			count++
		}
	}
	return count
}
