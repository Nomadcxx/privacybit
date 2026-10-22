// Package runner starts the privileged helper without handling sudo passwords.
package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Nomadcxx/privacybit/internal/helper"
)

const (
	readyTimeout = 2 * time.Minute
	shutdownWait = 3 * time.Second
	maxMessage   = 64 << 10
)

type lineResult struct {
	line []byte
	err  error
}

type Session struct {
	ctx           context.Context
	cmd           *exec.Cmd
	processGroup  int
	input         io.WriteCloser
	events        chan lineResult
	done          chan struct{}
	readStop      chan struct{}
	waitErr       error
	ready         helper.Event
	requestMu     sync.Mutex
	stopOnce      sync.Once
	closeOnce     sync.Once
	groupKillOnce sync.Once
	readStopOnce  sync.Once
	groupKillErr  error
}

// Start launches sudo with a pipe for helper JSON. sudo itself prompts on the
// controlling terminal; privacybit never receives password bytes.
func Start(ctx context.Context, sudoPath, helperPath string, stderr io.Writer) (*Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if sudoPath == "" {
		var err error
		sudoPath, err = exec.LookPath("sudo")
		if err != nil {
			return nil, fmt.Errorf("find sudo: %w", err)
		}
	}
	if helperPath == "" {
		helperPath = "privacybit-helper"
	}
	command := exec.Command(sudoPath, "-k", "--", helperPath)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Stderr = stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start sudo helper: %w", err)
	}
	session := &Session{
		ctx: ctx, cmd: command, processGroup: command.Process.Pid, input: stdin,
		events: make(chan lineResult, 1), done: make(chan struct{}), readStop: make(chan struct{}),
	}
	go session.read(stdout)
	go func() {
		session.waitErr = command.Wait()
		close(session.done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Stop()
		case <-session.done:
		}
	}()

	readyCtx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	line, err := session.next(readyCtx)
	if err != nil {
		_ = session.Stop()
		return nil, fmt.Errorf("wait for helper readiness: %w", err)
	}
	if err := json.Unmarshal(line, &session.ready); err != nil || session.ready.Type != "ready" || session.ready.Protocol != helper.Protocol || session.ready.CatalogRev == "" {
		_ = session.Stop()
		if err != nil {
			return nil, fmt.Errorf("invalid helper readiness event: %w", err)
		}
		return nil, errors.New("helper readiness event has an unsupported protocol or missing catalogue revision")
	}
	return session, nil
}

func (s *Session) Ready() helper.Event { return s.ready }

func (s *Session) read(output io.Reader) {
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), maxMessage)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		select {
		case s.events <- lineResult{line: line}:
		case <-s.readStop:
			return
		}
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	select {
	case s.events <- lineResult{err: err}:
	case <-s.readStop:
	}
}

func (s *Session) next(ctx context.Context) ([]byte, error) {
	select {
	case result := <-s.events:
		return result.line, result.err
	default:
	}
	select {
	case result := <-s.events:
		return result.line, result.err
	case <-s.done:
		select {
		case result := <-s.events:
			return result.line, result.err
		default:
		}
		if s.waitErr != nil {
			return nil, fmt.Errorf("helper process exited: %w", s.waitErr)
		}
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Request sends one request only after Start has received and checked ready.
func (s *Session) Request(request helper.Request) (helper.Event, error) {
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	var event helper.Event
	data, err := json.Marshal(request)
	if err != nil {
		return event, err
	}
	if len(data) > maxMessage || strings.ContainsAny(string(data), "\r\n") {
		return event, errors.New("helper request exceeds the protocol limit")
	}
	if _, err := s.input.Write(append(data, '\n')); err != nil {
		return event, fmt.Errorf("write helper request: %w", err)
	}
	line, err := s.next(s.ctx)
	if err != nil {
		return event, fmt.Errorf("read helper response: %w", err)
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return event, fmt.Errorf("parse helper response: %w", err)
	}
	if event.Type == "error" || event.Type == "result" && !event.OK {
		if event.Error == "" {
			event.Error = "helper rejected the request"
		}
		return event, errors.New(event.Error)
	}
	if event.Type != "result" || event.Op != request.Op {
		return event, errors.New("helper response does not match the request")
	}
	return event, nil
}

// Close asks the helper to exit by closing its request stream.
func (s *Session) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		_ = s.input.Close()
		select {
		case <-s.done:
			closeErr = s.killGroupRemainder()
		case <-time.After(shutdownWait):
			closeErr = s.Stop()
		}
	})
	s.stopReader()
	if closeErr != nil {
		return closeErr
	}
	if s.waitErr != nil {
		return s.waitErr
	}
	return nil
}

// Stop follows sudo's cleanup path before force-killing its private process
// group. It never signals the caller's own process group.
func (s *Session) Stop() error {
	var stopErr error
	s.stopOnce.Do(func() {
		if s.cmd.Process == nil {
			return
		}
		_ = s.cmd.Process.Signal(syscall.SIGALRM)
		select {
		case <-s.done:
		case <-time.After(shutdownWait):
		}
		stopErr = s.killGroupRemainder()
		_ = s.cmd.Process.Kill()
		<-s.done
		s.stopReader()
	})
	return stopErr
}

func (s *Session) killGroupRemainder() error {
	s.groupKillOnce.Do(func() {
		own, err := syscall.Getpgid(0)
		if err != nil {
			s.groupKillErr = err
			return
		}
		target, err := processGroupKillTarget(s.processGroup, own)
		if err != nil {
			s.groupKillErr = err
			return
		}
		if err := syscall.Kill(target, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			s.groupKillErr = err
		}
	})
	return s.groupKillErr
}

func processGroupKillTarget(group, callerGroup int) (int, error) {
	if group <= 0 || group == callerGroup {
		return 0, errors.New("refusing to signal the caller's process group")
	}
	return -group, nil
}

func (s *Session) stopReader() {
	s.readStopOnce.Do(func() { close(s.readStop) })
}
