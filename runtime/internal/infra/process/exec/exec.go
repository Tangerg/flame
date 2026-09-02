// Package exec is the background-process mechanism: it runs shell commands
// detached from the calling Run, buffers their output in a bounded ring so
// the model can read it incrementally, and kills them on demand. It is pure
// infra — no domain knowledge, no upward dependency.
//
// Every command the engine's shell tool runs starts here as a detached job:
// the foreground path races the command's completion ([Shell.Done]) against an
// auto-background window, removing the job ([Shells.Remove]) if it finishes in
// time and otherwise leaving it running and addressable by its shell id. So one
// mechanism backs both the synchronous shell result and the read_shell_output /
// stop_shell tools — the auto-background design.
//
// No PTY: plain pipes into a bounded ring buffer. Unix commands own an isolated
// process group so timeout, stop, natural parent exit, and shutdown reclaim
// their descendants. The base path is cross-platform; an opt-in [Sandbox]
// wraps each command in an in-place OS jail (macOS Seatbelt today, fail-closed
// elsewhere).
package exec

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
	"github.com/Tangerg/flame/runtime/internal/infra/process/sandbox"
)

// maxBuffer caps a background shell's retained output; once exceeded the
// oldest bytes are dropped (a poll that fell behind is told output was lost).
const (
	maxBuffer        = 256 * 1024
	processWaitDelay = time.Second
)

// Shells owns background shell commands and lets callers poll their output or
// stop them. The process handles and output buffers live here. The zero value is
// not usable; build one with [NewShells].
type Shells struct {
	lifecycle sync.Mutex
	mu        sync.Mutex
	epoch     string
	nextID    uint64
	shells    map[shellID]*Shell
	closed    bool
	closeOnce sync.Once
	closeErr  error
	// confiner jails each command in an in-place OS sandbox (workspace-write
	// only, network denied, $HOME hidden, env scrubbed) when non-nil; nil means
	// the host has no isolation backend. Built fail-closed at construction, so a
	// non-nil confiner is always a working backend.
	confiner *sandbox.Confiner
	// alwaysJail confines every command (the global sandbox.shell opt-in). When
	// false, only commands launched isolated=true are jailed — an isolated
	// session's shell is always confined regardless of the global opt-in.
	alwaysJail bool
}

var (
	// ErrShellsClosed reports a launch attempted after the shell owner shut down.
	ErrShellsClosed = errors.New("exec: shells closed")
	// ErrShellNotFound reports a command addressed outside this owner's shell set.
	ErrShellNotFound = errors.New("exec: shell not found")
	// ErrShellIdentityExhausted reports that one process-local Shells owner has
	// consumed every addressable sequence value.
	ErrShellIdentityExhausted = errors.New("exec: shell identity sequence exhausted")
)

const (
	shellIDPrefix     = "bg_"
	shellIDEpochBytes = 26
	shellIDSeparator  = "_"
)

type shellID struct{ value string }

func newShellID(epoch string, sequence uint64) shellID {
	return shellID{value: shellIDPrefix + epoch + shellIDSeparator + strconv.FormatUint(sequence, 10)}
}

func parseShellID(raw string) (shellID, bool) {
	rest, found := strings.CutPrefix(raw, shellIDPrefix)
	if !found || len(rest) <= shellIDEpochBytes || rest[shellIDEpochBytes:shellIDEpochBytes+1] != shellIDSeparator {
		return shellID{}, false
	}
	epoch := rest[:shellIDEpochBytes]
	for index := range len(epoch) {
		if character := epoch[index]; (character < 'A' || character > 'Z') && (character < '2' || character > '7') {
			return shellID{}, false
		}
	}
	sequenceText := rest[shellIDEpochBytes+1:]
	sequence, err := strconv.ParseUint(sequenceText, 10, 64)
	if err != nil || sequence == 0 || strconv.FormatUint(sequence, 10) != sequenceText {
		return shellID{}, false
	}
	return shellID{value: raw}, true
}

func (s shellID) String() string { return s.value }

// NewShells creates an empty background-shell set. confiner is the OS jail (nil
// when the host has no backend); alwaysJail confines every command (the global
// sandbox.shell opt-in). An isolated-session command is jailed even when
// alwaysJail is false.
func NewShells(confiner *sandbox.Confiner, alwaysJail bool) *Shells {
	return &Shells{epoch: rand.Text(), shells: map[shellID]*Shell{}, confiner: confiner, alwaysJail: alwaysJail}
}

// command returns the program, args, and environment to spawn for a shell
// command in cwd. It is jailed when the global opt-in is on OR the command runs
// in an isolated session; otherwise it is the plain `/bin/sh -c command` with a
// nil env (the child inherits the parent's). An isolated command with no
// backend fails closed — an isolated shell must never run unconfined.
func (s *Shells) command(cwd, command string, isolated bool) (name string, args, env []string, err error) {
	if !s.alwaysJail && !isolated {
		return "/bin/sh", []string{"-c", command}, nil, nil
	}
	if s.confiner == nil {
		return "", nil, nil, fmt.Errorf("exec: isolated shell requires a sandbox backend: %w", sandbox.ErrUnavailable)
	}
	confined, err := s.confiner.Confine(cwd, command)
	if err != nil {
		return "", nil, nil, err
	}
	return confined.Name, confined.Args, confined.Env, nil
}

// Shell is one background process: its handle, the tail of its combined
// stdout+stderr (capped), and its completion state. Read its output with
// [Shell.Read], wait for it with [Shell.Done], inspect its terminal state with
// [Shell.Status] / [Shell.Outcome]; the [Shells] set owns its lifecycle.
type Shell struct {
	cancel    context.CancelFunc
	cmd       *exec.Cmd
	process   *shellProcessOwner
	started   time.Time
	id        shellID       // the owner-map key, mirrored here for RunningForSession
	sessionID string        // session that launched it; scopes RunningForSession
	cwd       string        // canonical working-tree identity used by lifecycle cleanup
	command   string        // the shell command, for a session's live-state readout
	done      chan struct{} // closed once the process finishes

	mu       sync.Mutex
	buf      []byte // tail of stdout+stderr, capped at maxBuffer
	total    int    // absolute bytes ever written (buf holds the last len(buf))
	readPos  int    // absolute offset already returned to the caller
	finished bool
	exitInfo string        // "exit 0" / "exit 2" / "signal: killed" — set on completion
	exitCode int           // process exit code; -1 when it never ran / wasn't an exit
	killed   bool          // terminated by ctx/timeout/kill rather than exiting on its own
	duration time.Duration // wall time from launch to completion
	cleanup  error         // terminal process-tree cleanup diagnostic
}

// Launch starts command under cwd in the background and returns its shell id.
// sessionID scopes the shell to its owning session so [Shells.RunningForSession]
// can report a session's still-running jobs (e.g. for a post-compaction
// live-state reminder) without leaking another session's shells; "" is allowed
// for callers with no session.
//
// It is detached from the tool-call's cancellation so it outlives the Run —
// via context.WithoutCancel(ctx), which drops cancellation but KEEPS ctx's
// values, so the launching Run's trace span still propagates (full-link)
// rather than being severed by a bare context.Background(). An enabled timeout
// hard-kills the command when it elapses; a disabled [Timeout] lets it run until
// it exits or is killed.
func (s *Shells) Launch(ctx context.Context, sessionID, cwd, command string, timeout Timeout, isolated bool) (string, error) {
	if err := timeout.Validate(); err != nil {
		return "", err
	}
	base := context.WithoutCancel(ctx)
	var (
		runCtx context.Context
		cancel context.CancelFunc
	)
	if duration, enabled := timeout.Duration(); enabled {
		runCtx, cancel = context.WithTimeout(base, duration)
	} else {
		runCtx, cancel = context.WithCancel(base)
	}
	name, args, env, err := s.command(cwd, command, isolated)
	if err != nil {
		cancel()
		return "", err
	}
	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.Dir = cwd
	// env is nil when unconfined (inherit the parent environment); the sandbox
	// jail supplies a scrubbed environment instead.
	cmd.Env = env
	// On kill/timeout, force-close the pipes shortly after so the Wait goroutine
	// (and thus Done) returns promptly even when a child the shell spawned still
	// holds them — otherwise Wait blocks until that child exits.
	cmd.WaitDelay = processWaitDelay
	process := newShellProcessOwner(cmd)
	sh := &Shell{
		cancel: cancel, cmd: cmd, process: process, started: time.Now(),
		sessionID: sessionID, cwd: cwd, command: command, done: make(chan struct{}),
	}
	cmd.Stdout = sh
	cmd.Stderr = sh

	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return "", ErrShellsClosed
	}
	if s.nextID == math.MaxUint64 {
		s.mu.Unlock()
		cancel()
		return "", ErrShellIdentityExhausted
	}
	s.nextID++
	id := newShellID(s.epoch, s.nextID)
	sh.id = id
	// Start while holding the owner lock so shutdown cannot observe a Shell
	// whose exec.Cmd is only partly initialized. Once the shell is published,
	// cmd.Process is immutable and Kill/KillAll may safely use it.
	startErr := cmd.Start()
	if startErr != nil {
		cancel()
		sh.finish("start failed: "+startErr.Error(), -1, false, nil)
		s.shells[id] = sh
		s.mu.Unlock()
		return id.String(), nil
	}
	s.shells[id] = sh
	s.mu.Unlock()
	go func() {
		err := cmd.Wait()
		killed := runCtx.Err() != nil // ctx done = timeout or an explicit Kill
		cancel()
		cleanupErr := process.stop()
		code, info := 0, "exit 0"
		if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil {
			// The leader exited successfully but one of its descendants retained a
			// copied pipe. Final process-group cleanup above owns that descendant;
			// the pipe backstop must not rewrite the leader's real exit status.
			code = cmd.ProcessState.ExitCode()
			info = "exit " + strconv.Itoa(code)
		} else if err != nil {
			if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
				code = exitErr.ExitCode()
				info = "exit " + strconv.Itoa(code)
			} else {
				code, info = -1, err.Error()
			}
		}
		sh.finish(info, code, killed, cleanupErr)
	}()
	return id.String(), nil
}

// Get returns the shell with id and whether it exists.
func (s *Shells) Get(id string) (*Shell, bool) {
	identity, valid := parseShellID(id)
	if !valid {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.shells[identity]
	return sh, ok
}

// RunningShell identifies one background shell still executing: its id (for
// read_shell_output / stop_shell) and the command it runs.
type RunningShell struct {
	ID      string
	Command string
}

// RunningForSession returns sessionID's background shells that have not yet
// finished, in stable id order. Empty when the session has no live shells. Used
// to remind the model of live jobs a history compaction would otherwise drop.
func (s *Shells) RunningForSession(sessionID string) []RunningShell {
	s.mu.Lock()
	shells := make([]*Shell, 0, len(s.shells))
	for _, sh := range s.shells {
		if sh.sessionID == sessionID {
			shells = append(shells, sh)
		}
	}
	s.mu.Unlock()

	var out []RunningShell
	for _, sh := range shells {
		sh.mu.Lock()
		finished := sh.finished
		sh.mu.Unlock()
		if finished {
			continue
		}
		out = append(out, RunningShell{ID: sh.id.String(), Command: sh.command})
	}
	slices.SortFunc(out, func(a, b RunningShell) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Kill stops a background shell and reports whether it was still running.
// Missing ids have the stable [ErrShellNotFound] identity. A process that exits
// between the state snapshot and the kill is an idempotent success.
func (s *Shells) Kill(id string) (running bool, err error) {
	sh, ok := s.Get(id)
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrShellNotFound, id)
	}
	sh.mu.Lock()
	running = !sh.finished
	sh.mu.Unlock()
	if !running {
		return false, nil
	}
	sh.cancel()
	if err := sh.process.stop(); err != nil {
		return true, fmt.Errorf("exec: kill shell %q: %w", id, err)
	}
	return true, nil
}

// Remove drops a shell from the set without killing it. The foreground
// shell race calls it once a command completes within the auto-background
// window, so a finished command isn't left behind as a phantom background job.
// Killing instead would cancel the already-exited process context needlessly.
func (s *Shells) Remove(id string) {
	identity, valid := parseShellID(id)
	if !valid {
		return
	}
	s.mu.Lock()
	delete(s.shells, identity)
	s.mu.Unlock()
}

// StopSession stops, joins, and removes every shell owned by sessionID while
// leaving other Sessions' jobs addressable. It is the process-lifecycle
// boundary used before replacing or deleting a Session.
func (s *Shells) StopSession(sessionID string) error {
	return s.stopMatching("for Session "+strconv.Quote(sessionID), func(sh *Shell) (bool, error) {
		return sh.sessionID == sessionID, nil
	})
}

// StopWorkspace stops, joins, and removes every shell running at or below root
// across all Sessions. A destructive working-tree restore must not race a
// detached process that can immediately rewrite the restored files.
func (s *Shells) StopWorkspace(root string) error {
	if root == "" {
		return errors.New("exec: workspace root is required")
	}
	return s.stopMatching("for workspace "+strconv.Quote(root), func(sh *Shell) (bool, error) {
		if sh.cwd == "" {
			return false, nil
		}
		inside, err := pathidentity.Contains(root, sh.cwd)
		if err != nil {
			return false, fmt.Errorf("exec: compare shell %q workspace: %w", sh.id, err)
		}
		return inside, nil
	})
}

func (s *Shells) stopMatching(action string, matches func(*Shell) (bool, error)) error {
	if s == nil {
		return nil
	}
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.Lock()
	selected := make(map[shellID]*Shell)
	var errs []error
	for id, sh := range s.shells {
		match, err := matches(sh)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if match {
			selected[id] = sh
		}
	}
	s.mu.Unlock()
	stopped, stopErr := stopDetachedShells(selected, action)
	errs = append(errs, stopErr)
	s.mu.Lock()
	for _, id := range stopped {
		if s.shells[id] == selected[id] {
			delete(s.shells, id)
		}
	}
	s.mu.Unlock()
	return errors.Join(errs...)
}

// KillAll stops and joins every background shell in stable id order. It keeps
// every process-kill failure while still joining the complete set. Safe to call
// repeatedly; subsequent calls return the original shutdown result.
func (s *Shells) KillAll() error {
	s.closeOnce.Do(func() {
		s.lifecycle.Lock()
		defer s.lifecycle.Unlock()
		s.mu.Lock()
		s.closed = true
		shells := s.shells
		s.shells = map[shellID]*Shell{}
		s.mu.Unlock()
		_, s.closeErr = stopDetachedShells(shells, "during shutdown")
	})
	return s.closeErr
}

func stopDetachedShells(shells map[shellID]*Shell, action string) ([]shellID, error) {
	ids := make([]shellID, 0, len(shells))
	for id := range shells {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(left, right shellID) int {
		return strings.Compare(left.String(), right.String())
	})
	var errs []error
	stopped := make([]shellID, 0, len(ids))
	for _, id := range ids {
		sh := shells[id]
		select {
		case <-sh.done:
			stopped = append(stopped, id)
			continue
		default:
		}
		sh.cancel()
		if err := sh.process.stop(); err != nil {
			errs = append(errs, fmt.Errorf("exec: kill shell %q %s: %w", id, action, err))
			continue
		}
		stopped = append(stopped, id)
	}
	for _, id := range stopped {
		sh := shells[id]
		<-sh.done
		if err := sh.cleanupFailure(); err != nil {
			errs = append(errs, fmt.Errorf("exec: clean shell %q %s: %w", id, action, err))
		}
	}
	return stopped, errors.Join(errs...)
}

type shellProcessOwner struct {
	command *exec.Cmd
	once    sync.Once
	err     error
}

func newShellProcessOwner(command *exec.Cmd) *shellProcessOwner {
	configureShellProcess(command)
	owner := &shellProcessOwner{command: command}
	command.Cancel = owner.stop
	return owner
}

func (s *shellProcessOwner) stop() error {
	s.once.Do(func() {
		s.err = stopShellProcess(s.command)
		if errors.Is(s.err, os.ErrProcessDone) {
			s.err = nil
		}
	})
	return s.err
}

func (s *Shell) finish(info string, code int, killed bool, cleanup error) {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.exitInfo = info
	s.exitCode = code
	s.killed = killed
	s.duration = time.Since(s.started)
	s.cleanup = cleanup
	s.mu.Unlock()
	close(s.done)
}

// Done is closed when the process finishes — the foreground shell race selects
// on it to detect completion without polling.
func (s *Shell) Done() <-chan struct{} { return s.done }

// Outcome reports a finished shell's exit code, whether it was killed
// (timeout / explicit kill) rather than exiting on its own, its wall-clock
// duration, and any terminal process-tree cleanup failure. Meaningful only
// after [Shell.Done] is closed.
func (s *Shell) Outcome() (exitCode int, killed bool, duration time.Duration, cleanup error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode, s.killed, s.duration, s.cleanup
}

func (s *Shell) cleanupFailure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanup
}

// Read returns the output not yet returned to the caller and whether earlier
// output had to be dropped (the buffer overflowed before this poll).
func (s *Shell) Read() (out string, dropped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bufStart := s.total - len(s.buf)
	if s.readPos < bufStart {
		dropped = true
		s.readPos = bufStart
	}
	out = string(s.buf[s.readPos-bufStart:])
	s.readPos = s.total
	return out, dropped
}

// Status reports whether the shell finished and its exit info.
func (s *Shell) Status() (done bool, info string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finished, s.exitInfo
}

// Write funnels the shell's stdout/stderr into its capped ring buffer (the
// process's Stdout/Stderr point straight at the Shell). On overflow the oldest
// bytes are dropped — a poll that fell behind learns so via [Shell.Read].
func (s *Shell) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total += len(p)
	s.buf = append(s.buf, p...)
	if len(s.buf) > maxBuffer {
		s.buf = s.buf[len(s.buf)-maxBuffer:]
	}
	return len(p), nil
}
