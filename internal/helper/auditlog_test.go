package helper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestAuditLogIsPrivateAndUserOwned(t *testing.T) {
	home := t.TempDir()
	uid := os.Getuid()
	log, err := openAuditLogAt(home, uid)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	if err := log.Record(AuditRecord{At: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), Op: "purge-entry", Entry: "chrome", Result: "started"}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(home, ".local", "state", "privacybit", "actions.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("audit log mode = %04o, want 0600", info.Mode().Perm())
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != uid {
		t.Fatalf("audit log owner = %+v, want uid %d", info.Sys(), uid)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record AuditRecord
	if err := json.Unmarshal(data, &record); err != nil || record.Op != "purge-entry" || record.Entry != "chrome" {
		t.Fatalf("audit record = %+v, decode error %v", record, err)
	}
}

func TestAuditLogRejectsUserControlledSymlink(t *testing.T) {
	home, target := t.TempDir(), t.TempDir()
	if err := os.Symlink(target, filepath.Join(home, ".local")); err != nil {
		t.Fatal(err)
	}
	if _, err := openAuditLogAt(home, os.Getuid()); err == nil {
		t.Fatal("audit log followed a symlink in the home directory")
	}
	if _, err := os.Stat(filepath.Join(target, "state", "privacybit", "actions.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("symlink target was modified: %v", err)
	}
}
