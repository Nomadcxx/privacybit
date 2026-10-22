package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const (
	reportName       = "report.json"
	unreadName       = "report.unread"
	scanLockName     = ".scan.lock"
	maxSnapshotBytes = 4 << 20
)

// Store keeps the latest private audit snapshot and its unread marker.
type Store struct {
	Dir string
}

func UserStore() (Store, error) {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome != "" && !filepath.IsAbs(stateHome) {
		return Store{}, errors.New("XDG_STATE_HOME must be an absolute path")
	}
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Store{}, fmt.Errorf("find home directory: %w", err)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return Store{Dir: filepath.Join(stateHome, "privacybit")}, nil
}

// Lock serializes report-producing scans for this user's state directory.
// Call the returned release function once the scan and its save have finished.
func (s Store) Lock() (func(), error) {
	if err := ensurePrivateDir(s.Dir); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(filepath.Join(s.Dir, scanLockName),
		syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open report lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), scanLockName)
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect report lock: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ok || stat.Uid != uint32(os.Geteuid()) {
		_ = file.Close()
		return nil, errors.New("report lock must be a private regular file owned by the current user")
	}
	for err = syscall.Flock(fd, syscall.LOCK_EX); errors.Is(err, syscall.EINTR); err = syscall.Flock(fd, syscall.LOCK_EX) {
	}
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock report state: %w", err)
	}
	return func() { _ = file.Close() }, nil
}

func (s Store) Save(snapshot Snapshot, unread bool) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode report snapshot: %w", err)
	}
	if len(data) > maxSnapshotBytes {
		return errors.New("report snapshot exceeds the storage limit")
	}
	if _, err := ParseSnapshot(data); err != nil {
		return fmt.Errorf("refuse invalid report snapshot: %w", err)
	}
	if err := ensurePrivateDir(s.Dir); err != nil {
		return err
	}
	if err := writePrivate(s.Dir, reportName, data); err != nil {
		return fmt.Errorf("write report snapshot: %w", err)
	}
	if unread {
		if err := writePrivate(s.Dir, unreadName, []byte("1\n")); err != nil {
			return fmt.Errorf("mark report unread: %w", err)
		}
		return nil
	}
	return s.MarkRead()
}

func (s Store) Load() (*Snapshot, error) {
	data, err := readPrivate(s.Dir, reportName, maxSnapshotBytes)
	if err != nil {
		return nil, fmt.Errorf("read report snapshot: %w", err)
	}
	snapshot, err := ParseSnapshot(data)
	if err != nil {
		return nil, fmt.Errorf("parse report snapshot: %w", err)
	}
	return snapshot, nil
}

func (s Store) IsUnread() (bool, error) {
	data, err := readPrivate(s.Dir, unreadName, 16)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read report marker: %w", err)
	}
	if !bytes.Equal(data, []byte("1\n")) {
		return false, errors.New("invalid report unread marker")
	}
	return true, nil
}

func (s Store) MarkRead() error {
	if err := ensurePrivateDir(s.Dir); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(s.Dir, unreadName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear report unread marker: %w", err)
	}
	return nil
}

func ensurePrivateDir(dir string) error {
	if dir == "" {
		return errors.New("report state directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create report state directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("report state path must be a real directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("report state directory is not owned by the current user")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("restrict report state directory: %w", err)
	}
	return nil
}

func writePrivate(dir, name string, data []byte) error {
	temp, err := os.CreateTemp(dir, ".privacybit-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, filepath.Join(dir, name)); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func readPrivate(dir, name string, limit int64) ([]byte, error) {
	fd, err := syscall.Open(filepath.Join(dir, name), syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("report file must be regular and private")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("report file is not owned by the current user")
	}
	if info.Size() > limit {
		return nil, errors.New("report file exceeds the storage limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("report file exceeds the storage limit")
	}
	return data, nil
}
