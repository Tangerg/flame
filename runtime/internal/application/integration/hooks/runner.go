// Package hooks executes a bound set of trusted hooks and folds their external
// command outcomes into one typed Application decision.
package hooks

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	domain "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/samber/lo"
)

// DefaultTimeout bounds a single hook command — a hook must not wedge execution
// it gates. A hook that exceeds it is killed and treated as a non-blocking error
// (the action proceeds), so a hung hook degrades to "no hook" rather than a
// frozen agent.
const defaultTimeout = 30 * time.Second

// blockExitCode is the one non-zero exit a hook uses to BLOCK the action
// (stderr becomes the reason fed to the model). Every other non-zero exit is a
// non-blocking error: the hook is broken, but a broken hook must not brick the
// session, so the action proceeds. Explicit deny is the only hard stop — same
// contract as Claude Code.
const blockExitCode = 2

// CommandRequest is the shell-command work a [CommandRunner] executes. The
// domain prepares stdin and timeout; the runner owns how the command runs.
type CommandRequest struct {
	Ordinal int
	Command string
	CWD     string
	Input   hookdata.Input
	Timeout time.Duration
}

// CommandResult is the process-level outcome returned by [CommandRunner].
type CommandResult struct {
	Decision hookdata.CommandDecision
	Stderr   string
	ExitCode int
	Err      error
	TimedOut bool
}

func domainVerdict(c hookdata.CommandVerdict) domain.Verdict {
	switch c {
	case hookdata.CommandDeny:
		return domain.VerdictBlock
	case hookdata.CommandAsk:
		return domain.VerdictAsk
	default:
		return domain.VerdictAllow
	}
}

// CommandRunner executes hook commands with borrowed request data. It neither
// mutates the request nor retains it after the synchronous call returns.
type commandRunner interface {
	RunHookCommand(ctx context.Context, req CommandRequest) CommandResult
}

// Runner executes a trust-filtered hook set for one event and folds their
// outcomes into a single Decision.
type Runner struct {
	commands commandRunner
	// onError, when set, is called for a hook that failed to run (spawn error,
	// timeout, or a non-blocking non-zero exit). The caller owns diagnostics;
	// lifecycle hooks can run without an active tracing span.
	onError func(ctx context.Context, source string, err error)
}

// NewRunner builds a Runner. commands executes imperative hooks; onError may be
// nil. A nil commands runner means declarative inject hooks still work and
// command hooks degrade to non-blocking errors.
func NewRunner(commands commandRunner, onError func(ctx context.Context, source string, err error)) *Runner {
	if lo.IsNil(commands) {
		commands = nil
	}
	return &Runner{commands: commands, onError: onError}
}

// Run fires every hook matching in's event (and, for tool events, its tool
// name) and returns the combined Decision. A declarative `inject` hook
// contributes context with no process spawn; a `command` hook is exec'd with in
// as JSON on stdin. Hooks run in list order (loader order: global before
// project), so the combine rules (first-block-wins, first-rewrite-wins) are
// deterministic. Hooks and input are borrowed until the call returns.
func (r *Runner) Run(ctx context.Context, hooks []hookdata.Hook, in hookdata.Input) domain.Decision {
	var dec domain.Decision
	var commandInput hookdata.Input
	var commandInputErr error
	commandInputReady := false
	for ordinal, h := range hooks {
		if !h.Matches(in) {
			continue
		}
		if h.Command == "" {
			// Declarative: a literal context injection, no exec.
			dec.Fold(domain.VerdictAllow, "", strings.TrimSpace(h.Inject), "")
			continue
		}
		if !commandInputReady {
			commandInput, commandInputErr = in.CommandProjection()
			commandInputReady = true
		}
		if commandInputErr != nil {
			r.fail(ctx, h.Source, commandInputErr)
			continue
		}
		r.runOne(ctx, ordinal, h, commandInput, &dec)
	}
	return dec
}

// runOne execs a single command hook and folds its outcome.
func (r *Runner) runOne(ctx context.Context, ordinal int, h hookdata.Hook, in hookdata.Input, dec *domain.Decision) {
	if r.commands == nil {
		r.fail(ctx, h.Source, errors.New("hook command runner is not configured"))
		return
	}
	timeout := defaultTimeout
	if h.TimeoutMillis > 0 {
		timeout = time.Duration(h.TimeoutMillis) * time.Millisecond
	}
	result := r.commands.RunHookCommand(ctx, CommandRequest{
		Ordinal: ordinal, Command: h.Command,
		CWD:     in.CWD,
		Input:   in,
		Timeout: timeout,
	})
	if result.TimedOut {
		r.fail(ctx, h.Source, errors.Join(errors.New("hook timed out"), result.Err))
		return
	}

	out := result.Decision

	switch {
	case result.ExitCode == blockExitCode:
		// Exit 2: block. Reason is the stdout JSON's, else stderr.
		reason := out.Reason
		if reason == "" {
			reason = strings.TrimSpace(result.Stderr)
		}
		dec.Fold(domain.VerdictBlock, reason, strings.TrimSpace(out.InjectContext), "")
	case result.Err == nil && result.ExitCode == 0:
		// Exit 0 must carry a complete decoded decision. Treat a missing or
		// future verdict as a broken hook instead of silently granting it.
		if !out.Verdict.Valid() {
			r.fail(ctx, h.Source, errors.New("hook returned an invalid command verdict"))
			return
		}
		verdict := domainVerdict(out.Verdict)
		reason := out.Reason
		if verdict == domain.VerdictBlock && reason == "" {
			reason = strings.TrimSpace(result.Stderr)
		}
		dec.Fold(verdict, reason, strings.TrimSpace(out.InjectContext), strings.TrimSpace(out.RewriteArguments))
	default:
		// Any other non-zero exit (or spawn failure): a broken hook. Non-blocking
		// — the action proceeds — but surfaced via onError so it's observable.
		r.fail(ctx, h.Source, hookError(result.ExitCode, result.Stderr, result.Err))
	}
}

func (r *Runner) fail(ctx context.Context, source string, err error) {
	if r.onError != nil {
		r.onError(ctx, source, err)
	}
}

// hookError builds a descriptive error for a non-blocking hook failure.
func hookError(exit int, stderr string, runErr error) error {
	if s := strings.TrimSpace(stderr); s != "" {
		return errors.Join(runErr, errors.New(s))
	}
	if runErr != nil {
		return runErr
	}
	return errors.New("hook exited with code " + strconv.Itoa(exit))
}

// Bound is the resolved hook set for one cwd, ready to fire events.
type Bound struct {
	hooks  []hookdata.Hook
	runner *Runner
	close  func() error
}

// NewBound owns a snapshot of the hook configuration retained for later events.
func NewBound(hooks []hookdata.Hook, runner *Runner, close func() error) *Bound {
	return &Bound{hooks: slices.Clone(hooks), runner: runner, close: close}
}

// Run fires the bound hooks for in's event and returns the combined Decision.
// Nil-safe: a nil Bound returns the zero Decision, so every seam can call
// st.hooks.Run(...) unguarded.
func (b *Bound) Run(ctx context.Context, in hookdata.Input) domain.Decision {
	if b == nil || b.runner == nil || len(b.hooks) == 0 {
		return domain.Decision{}
	}
	return b.runner.Run(ctx, b.hooks, in)
}

// Inspection is the read-only view of a cwd's hooks for a management surface
// (hooks.list): every discovered hook (trusted or not), the project
// root that gates the project-scope ones, and whether it's currently trusted.
type Inspection struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []hookdata.Hook
}

func (b *Bound) Close() error {
	if b != nil && b.close != nil {
		return b.close()
	}
	return nil
}
