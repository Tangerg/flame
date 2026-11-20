// Package exec is the background-process mechanism: it runs shell commands
// detached from the calling Run, spools their output in a bounded private file so
// the model can read it incrementally, and kills them on demand. It is pure
// infra — no domain knowledge, no upward dependency.
//
// Every command the engine's shell tool runs starts here as a detached job:
// the foreground path races the command's completion ([Shell.Done]) against an
// auto-background window, consuming the job's output if it finishes in
// time and otherwise leaving it running and addressable by its shell id. So one
// mechanism backs both the synchronous shell result and the read_shell_output /
// stop_shell tools — the auto-background design.
//
// No PTY: plain pipes into a bounded output spool. Unix commands own an isolated
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
	"maps"
	"math"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/procgroup"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
)

// Output envelopes bound one page, one command, and this process owner's entire
// retained log set. Exceeding a spool bound stops the original command with its
// admitted output intact; an old prefix is never silently replaced by a tail.
const (
	maxOutputPageBytes     = 256 << 10
	maxShellOutputBytes    = 8 << 20
	maxRetainedOutputBytes = 64 << 20
	maxRetainedShells      = 256
	processWaitDelay       = time.Second
)

// Shells owns background shell commands and lets callers poll their output or
// stop them. The process handles and output buffers live here. The zero value is
// not usable; build one with [NewShells].
type Shells struct {
	lifecycle     sync.Mutex
	mu            sync.Mutex
	epoch         string
	nextID        uint64
	shells        map[shellID]*Shell
	closed        bool
	closeOnce     sync.Once
	closeErr      error
	retainedBytes atomic.Int64
	// confiner jails each command in an in-place OS sandbox (workspace-write
	// only, network denied, $HOME hidden, env scrubbed) when non-nil. Built
	// fail-closed at construction, so a non-nil confiner is always a working
	// backend; when it is nil, unconfinable is why, and every command that must
	// be jailed reports that cause rather than a generic absence.
	confiner     *sandbox.Confiner
	unconfinable error
	// alwaysJail confines every command (the global sandbox.shell opt-in). When
	// false, only commands launched isolated=true are jailed — an isolated
	// session's shell is always confined regardless of the global opt-in.
	alwaysJail bool
}

var (
	// errShellsClosed reports a launch attempted after the shell owner shut down.
	errShellsClosed = errors.New("exec: shells closed")
	// ErrShellNotFound reports a command addressed outside this owner's shell set.
	ErrShellNotFound = errors.New("exec: shell not found")
	// errShellIdentityExhausted reports that one process-local Shells owner has
	// consumed every addressable sequence value.
	errShellIdentityExhausted = errors.New("exec: shell identity sequence exhausted")
	ErrShellOutputLimit       = errors.New("exec: shell output spool capacity reached; command stopped with retained output")
	errShellCapacity          = errors.New("exec: retained shell capacity reached; read completed output or stop owned shells")
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

// NewShells creates an empty background-shell set from the outcome of
// [sandbox.NewConfiner]: exactly one of confiner and unconfinable is non-nil.
// alwaysJail confines every command (the global sandbox.shell opt-in). An
// isolated-session command is jailed even when alwaysJail is false.
func NewShells(confiner *sandbox.Confiner, unconfinable error, alwaysJail bool) (*Shells, error) {
	if (confiner == nil) == (unconfinable == nil) {
		return nil, errors.New("exec: shells need either a confiner or the reason there is none")
	}
	if alwaysJail && confiner == nil {
		return nil, fmt.Errorf("exec: confine every shell: %w", unconfinable)
	}
	return &Shells{
		epoch:        rand.Text(),
		shells:       map[shellID]*Shell{},
		confiner:     confiner,
		unconfinable: unconfinable,
		alwaysJail:   alwaysJail,
	}, nil
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
		return "", nil, nil, fmt.Errorf("exec: isolated shell cannot be confined: %w", s.unconfinable)
	}
	confined, err := s.confiner.Confine(cwd, command)
	if err != nil {
		return "", nil, nil, err
	}
	return confined.Name, confined.Args, confined.Env, nil
}

// Shell is one background process: its handle, its combined stdout+stderr spool,
// and its completion state. Read its output with
// [Shells.Read], wait for it with [Shell.Done], inspect its terminal state with
// [Shell.Status] / [Shell.Outcome]; the [Shells] set owns its lifecycle.
type Shell struct {
	cancel    context.CancelFunc
	process   *shellProcessOwner
	started   time.Time
	sessionID string // session that launched it; scopes RetainedForSession
	cwd       string // physical working-tree identity, established by Launch
	command   string // the shell command, for a session's live-state readout
	// done is the one completion signal: it closes after the terminal fields
	// below are written, so a reader that observed it reads a settled outcome.
	done chan struct{}

	mu          sync.Mutex
	log         *os.File
	logPath     string // empty for an unlinked temporary file
	logBytes    int64
	readPos     int64
	spoolBytes  *atomic.Int64 // the original Shells owner's aggregate charge
	logReleased bool
	outputErr   error
	exit        Exit  // written once by finish, before done closes
	cleanup     error // terminal process-tree cleanup diagnostic
}

// Exit is a finished shell's terminal account. Only a finished shell has one,
// so a running shell cannot present an exit code or duration.
type Exit struct {
	Info     string        // "exit 0", "exit 2", "signal: killed" or the start/wait failure
	Code     int           // NoExitStatus when the process never reported one
	Killed   bool          // ended by timeout or kill rather than exiting on its own
	Duration time.Duration // wall time from launch to completion
}

// NoExitStatus is the exit code of a command that never reported one: it failed
// to start, or its wait failed. Its [Exit.Info] is then the only account of
// what happened.
const NoExitStatus = -1

// Launch starts command under cwd in the background and returns its shell id.
// sessionID scopes the shell to its owning session so [Shells.RetainedForSession]
// can report a session's retained jobs (e.g. for a post-compaction
// live-state reminder) without leaking another session's shells; "" is allowed
// for callers with no session.
//
// It is detached from the tool-call's cancellation so it outlives the Run —
// via context.WithoutCancel(ctx), which drops cancellation but KEEPS ctx's
// values, so the launching Run's trace span still propagates (full-link)
// rather than being severed by a bare context.Background(). An enabled timeout
// hard-kills the command when it elapses; a disabled [Timeout] lets it run until
// it exits or is killed.
// admitShellLocked reserves the next identity and an output spool for sh.
func (s *Shells) admitShellLocked(ctx context.Context, sh *Shell) (shellID, error) {
	if s.closed {
		return shellID{}, errShellsClosed
	}
	if cause := context.Cause(ctx); cause != nil {
		return shellID{}, cause
	}
	if s.nextID == math.MaxUint64 {
		return shellID{}, errShellIdentityExhausted
	}
	if len(s.shells) >= maxRetainedShells {
		return shellID{}, errShellCapacity
	}
	log, logPath, err := openShellLog()
	if err != nil {
		return shellID{}, fmt.Errorf("exec: admit shell output spool: %w", err)
	}
	sh.log, sh.logPath, sh.spoolBytes = log, logPath, &s.retainedBytes
	s.nextID++
	return newShellID(s.epoch, s.nextID), nil
}

// exitStatus reports how the leader exited. A leader that exited while a
// descendant held a copied pipe keeps its real status; process-group cleanup
// owns that descendant, and the pipe backstop must not rewrite the status.
func exitStatus(cmd *exec.Cmd, err error) (string, int) {
	if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		return "exit " + strconv.Itoa(code), code
	}
	if err == nil {
		return "exit 0", 0
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return "exit " + strconv.Itoa(exitErr.ExitCode()), exitErr.ExitCode()
	}
	return err.Error(), NoExitStatus
}

func (s *Shells) Launch(ctx context.Context, sessionID, cwd, command string, timeout Timeout, isolated bool) (string, error) {
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}
	if err := timeout.Validate(); err != nil {
		return "", err
	}
	// The stored identity decides which tree this shell belongs to when a
	// destructive restore quiesces one. A caller spells a workspace the way its
	// Session does, and two Sessions can spell one tree differently, so the
	// identity is established here rather than assumed of every caller. The
	// process still runs in the path it was given, and an empty cwd inherits
	// this process's directory while claiming no tree — the state
	// StopWorkspace already skips.
	treeIdentity := ""
	if cwd != "" {
		resolved, resolveErr := filesystem.ResolvePath("", cwd)
		if resolveErr != nil {
			return "", fmt.Errorf("exec: resolve shell working tree %q: %w", cwd, resolveErr)
		}
		treeIdentity = resolved
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
	name, args, env, commandErr := s.command(cwd, command, isolated)
	if commandErr != nil {
		cancel()
		return "", commandErr
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
		cancel: cancel, process: process, started: time.Now(),
		sessionID: sessionID, cwd: treeIdentity, command: command, done: make(chan struct{}),
	}
	cmd.Stdout = sh
	cmd.Stderr = sh

	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.Lock()
	id, err := s.admitShellLocked(ctx, sh)
	if err != nil {
		s.mu.Unlock()
		cancel()
		return "", err
	}
	// Start while holding the owner lock so shutdown cannot observe a Shell
	// whose exec.Cmd is only partly initialized. Once the shell is published,
	// cmd.Process is immutable and Kill/KillAll may safely use it.
	startErr := cmd.Start()
	if startErr != nil {
		cancel()
		sh.finish("start failed: "+startErr.Error(), NoExitStatus, false, nil)
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
		info, code := exitStatus(cmd, err)
		sh.finish(info, code, killed, cleanupErr)
	}()
	return id.String(), nil
}

func (s *Shells) Get(sessionID, id string) (*Shell, bool) {
	identity, valid := parseShellID(id)
	if !valid {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.shells[identity]
	if !ok || sh.sessionID != sessionID {
		return nil, false
	}
	return sh, true
}

// RetainedShell identifies an addressable command, including completed commands
// whose final output has not yet been consumed.
type RetainedShell struct {
	ID      string
	Command string
}

// RetainedForSession snapshots handles that a compaction must preserve until
// their results are read or session teardown releases them.
func (s *Shells) RetainedForSession(sessionID string) []RetainedShell {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []RetainedShell
	for id, sh := range s.shells {
		if sh.sessionID == sessionID {
			out = append(out, RetainedShell{ID: id.String(), Command: sh.command})
		}
	}
	slices.SortFunc(out, func(a, b RetainedShell) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Missing and foreign Session handles have the same ErrShellNotFound outcome.
// A process that exits between lookup and stop remains an idempotent success.
func (s *Shells) Kill(sessionID, id string) (running bool, err error) {
	sh, ok := s.Get(sessionID, id)
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrShellNotFound, id)
	}
	if sh.finished() {
		return false, nil
	}
	sh.cancel()
	if err := sh.process.stop(); err != nil {
		return true, fmt.Errorf("exec: kill shell %q: %w", id, err)
	}
	return true, nil
}

// Output is one observation of the incremental bytes and completion boundary.
type Output struct {
	ShellID    string // retained handle; empty after the final page releases it
	Text       string
	Dropped    bool
	Offset     int64
	NextOffset int64
	HasMore    bool
	Exit       *Exit // nil while the shell still runs
}

// Read consumes available output and retires a successfully cleaned, finished
// shell in the same owner operation. Cleanup failure preserves both the unread
// bytes and the handle so teardown cannot lose its resource or diagnostic.
func (s *Shells) Read(sessionID, id string) (Output, error) {
	identity, valid := parseShellID(id)
	if !valid {
		return Output{}, fmt.Errorf("%w: %q", ErrShellNotFound, id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, found := s.shells[identity]
	if !found || sh.sessionID != sessionID {
		return Output{}, fmt.Errorf("%w: %q", ErrShellNotFound, id)
	}
	output, retire, err := sh.read()
	if retire {
		if releaseErr := sh.releaseLog(); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		} else {
			delete(s.shells, identity)
			return output, err
		}
	}
	output.ShellID = id
	return output, err
}

// StopSession stops, joins, and removes every shell owned by sessionID while
// leaving other Sessions' jobs addressable. It is the process-lifecycle
// boundary used before replacing or deleting a Session.
func (s *Shells) StopSession(sessionID string) error {
	return s.stopMatching("for Session "+strconv.Quote(sessionID), func(_ shellID, sh *Shell) (bool, error) {
		return sh.sessionID == sessionID, nil
	})
}

// StopWorkspace stops, joins, and removes every shell whose tree overlaps root
// across all Sessions. A destructive working-tree restore must not race a
// detached process that can immediately rewrite the restored files.
func (s *Shells) StopWorkspace(root string) error {
	if root == "" {
		return errors.New("exec: workspace root is required")
	}
	// Match on the same physical identity Launch stored: a Session restoring its
	// tree names it the way that Session does, and a sibling Session's shell may
	// hold the same tree through another spelling.
	physicalRoot, err := filesystem.ResolvePath("", root)
	if err != nil {
		return fmt.Errorf("exec: resolve workspace root %q: %w", root, err)
	}
	return s.stopMatching("for workspace "+strconv.Quote(root), func(id shellID, sh *Shell) (bool, error) {
		if sh.cwd == "" {
			return false, nil
		}
		inside, err := filesystem.PathsOverlap(physicalRoot, sh.cwd)
		if err != nil {
			return false, fmt.Errorf("exec: compare shell %q workspace: %w", id, err)
		}
		return inside, nil
	})
}

func (s *Shells) stopMatching(action string, matches func(shellID, *Shell) (bool, error)) error {
	if s == nil {
		return nil
	}
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.Lock()
	selected := make(map[shellID]*Shell)
	var errs []error
	for id, sh := range s.shells {
		match, err := matches(id, sh)
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
		shells := maps.Clone(s.shells)
		s.mu.Unlock()
		stopped, err := stopDetachedShells(shells, "during shutdown")
		s.mu.Lock()
		for _, id := range stopped {
			delete(s.shells, id)
		}
		s.closeErr = err
		s.mu.Unlock()
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
		if sh.finished() {
			stopped = append(stopped, id)
			continue
		}
		sh.cancel()
		if err := sh.process.stop(); err != nil {
			errs = append(errs, fmt.Errorf("exec: kill shell %q %s: %w", id, action, err))
			continue
		}
		stopped = append(stopped, id)
	}
	released := make([]shellID, 0, len(stopped))
	for _, id := range stopped {
		sh := shells[id]
		<-sh.done
		if err := sh.cleanupFailure(); err != nil {
			errs = append(errs, fmt.Errorf("exec: clean shell %q %s: %w", id, action, err))
			continue
		}
		if err := sh.releaseLog(); err != nil {
			errs = append(errs, fmt.Errorf("exec: release shell %q output %s: %w", id, action, err))
			continue
		}
		released = append(released, id)
	}
	return released, errors.Join(errs...)
}

type shellProcessOwner struct {
	command *exec.Cmd
	once    sync.Once
	err     error
}

func newShellProcessOwner(command *exec.Cmd) *shellProcessOwner {
	procgroup.Prepare(command)
	owner := &shellProcessOwner{command: command}
	command.Cancel = owner.stop
	return owner
}

func (s *shellProcessOwner) stop() error {
	s.once.Do(func() {
		s.err = procgroup.Stop(s.command)
		if errors.Is(s.err, os.ErrProcessDone) {
			s.err = nil
		}
	})
	return s.err
}

// finish is called exactly once: by the start failure or by the waiter.
func (s *Shell) finish(info string, code int, killed bool, cleanup error) {
	s.mu.Lock()
	s.exit = Exit{Info: info, Code: code, Killed: killed, Duration: time.Since(s.started)}
	s.cleanup = cleanup
	s.mu.Unlock()
	close(s.done)
}

// Done is closed when the process finishes — the foreground shell race selects
// on it to detect completion without polling.
func (s *Shell) Done() <-chan struct{} { return s.done }

func (s *Shell) finished() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *Shell) cleanupFailure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanup
}
