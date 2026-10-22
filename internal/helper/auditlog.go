package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const auditLogName = "actions.jsonl"

type AuditLog struct {
	mu   sync.Mutex
	file *os.File
}

// OpenAuditLog opens the invoking user's private action log. The uid comes
// from sudo's SUDO_UID, never from the caller's HOME or XDG variables.
func OpenAuditLog(uid int) (*AuditLog, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("audit log must be opened by the privileged helper")
	}
	account, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return nil, fmt.Errorf("lookup invoking user %d: %w", uid, err)
	}
	return openAuditLogAt(account.HomeDir, uid)
}

func openAuditLogAt(home string, uid int) (*AuditLog, error) {
	if uid < 0 || !filepath.IsAbs(home) || filepath.Clean(home) == "/" {
		return nil, errors.New("invalid audit log home or uid")
	}
	root, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(root)
	current := root
	var opened []int
	defer func() {
		for _, fd := range opened {
			_ = syscall.Close(fd)
		}
	}()
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(home), "/"), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.New("invalid audit log home path")
		}
		fd, err := syscall.Openat(current, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return nil, fmt.Errorf("open audit log path component %q: %w", part, err)
		}
		opened = append(opened, fd)
		var stat syscall.Stat_t
		if err := syscall.Fstat(fd, &stat); err != nil {
			return nil, err
		}
		if !safeDirectoryOwner(stat, uint32(uid)) {
			return nil, fmt.Errorf("unsafe owner or permissions on audit log path component %q", part)
		}
		current = fd
	}

	for _, part := range []string{".local", "state", "privacybit"} {
		fd, err := openPrivateDir(current, part, uint32(uid))
		if err != nil {
			return nil, err
		}
		opened = append(opened, fd)
		current = fd
	}
	fd, err := openPrivateLog(current, uint32(uid))
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), auditLogName)
	return &AuditLog{file: file}, nil
}

func safeDirectoryOwner(stat syscall.Stat_t, uid uint32) bool {
	if stat.Mode&syscall.S_IFMT != syscall.S_IFDIR {
		return false
	}
	if stat.Uid != 0 && stat.Uid != uid {
		return false
	}
	if stat.Mode&0o022 == 0 {
		return true
	}
	return stat.Uid == 0 && stat.Mode&syscall.S_ISVTX != 0
}

func openPrivateDir(parent int, name string, uid uint32) (int, error) {
	flags := syscall.O_RDONLY | syscall.O_DIRECTORY | syscall.O_CLOEXEC | syscall.O_NOFOLLOW
	fd, err := syscall.Openat(parent, name, flags, 0)
	if err == syscall.ENOENT {
		if err := syscall.Mkdirat(parent, name, 0o700); err != nil && err != syscall.EEXIST {
			return -1, fmt.Errorf("create audit log directory %q: %w", name, err)
		}
		fd, err = syscall.Openat(parent, name, flags, 0)
		if err != nil {
			return -1, fmt.Errorf("open new audit log directory %q: %w", name, err)
		}
		if os.Geteuid() == 0 {
			if err := syscall.Fchown(fd, int(uid), int(uid)); err != nil {
				_ = syscall.Close(fd)
				return -1, fmt.Errorf("set owner on audit log directory %q: %w", name, err)
			}
		}
		if err := syscall.Fchmod(fd, 0o700); err != nil {
			_ = syscall.Close(fd)
			return -1, fmt.Errorf("set permissions on audit log directory %q: %w", name, err)
		}
	} else if err != nil {
		return -1, fmt.Errorf("open audit log directory %q: %w", name, err)
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		_ = syscall.Close(fd)
		return -1, err
	}
	if stat.Uid != uid || stat.Mode&syscall.S_IFMT != syscall.S_IFDIR || stat.Mode&0o022 != 0 {
		_ = syscall.Close(fd)
		return -1, fmt.Errorf("unsafe owner or permissions on audit log directory %q", name)
	}
	return fd, nil
}

func openPrivateLog(parent int, uid uint32) (int, error) {
	flags := syscall.O_WRONLY | syscall.O_APPEND | syscall.O_CLOEXEC | syscall.O_NOFOLLOW
	fd, err := syscall.Openat(parent, auditLogName, flags|syscall.O_CREAT|syscall.O_EXCL, 0o600)
	if err == nil {
		if os.Geteuid() == 0 {
			if err := syscall.Fchown(fd, int(uid), int(uid)); err != nil {
				_ = syscall.Close(fd)
				return -1, fmt.Errorf("set audit log owner: %w", err)
			}
		}
	} else if err == syscall.EEXIST {
		fd, err = syscall.Openat(parent, auditLogName, flags, 0)
		if err != nil {
			return -1, fmt.Errorf("open existing audit log: %w", err)
		}
	} else {
		return -1, fmt.Errorf("create audit log: %w", err)
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		_ = syscall.Close(fd)
		return -1, err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Uid != uid || stat.Nlink != 1 {
		_ = syscall.Close(fd)
		return -1, errors.New("audit log must be a single-link regular file owned by the invoking user")
	}
	if err := syscall.Fchmod(fd, 0o600); err != nil {
		_ = syscall.Close(fd)
		return -1, fmt.Errorf("set audit log permissions: %w", err)
	}
	return fd, nil
}

func (l *AuditLog) Record(record AuditRecord) error {
	if l == nil || l.file == nil {
		return errors.New("audit log is closed")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	n, err := l.file.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return err
	}
	return l.file.Sync()
}

func (l *AuditLog) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
