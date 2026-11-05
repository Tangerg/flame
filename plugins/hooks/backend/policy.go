package main

import (
	"context"
	"errors"
	"path"
	"strconv"
	"strings"
	"time"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/samber/lo"
)

type eventAdvice struct{ hookdata.CommandDecision }

// A stalled command must settle according to its captured failure policy.
const defaultTimeout = 30 * time.Second

// Exit 2 is the command protocol's explicit denial.
const blockExitCode = 2

type commandRunner interface {
	RunHookCommand(ctx context.Context, req commandRequest) commandResult
}

type policyEvaluator struct {
	commands commandRunner
	// onError, when set, is called for a hook that failed to run (spawn error,
	// timeout, or a non-blocking non-zero exit). The caller owns diagnostics;
	// lifecycle hooks can run without an active tracing span.
	onError func(ctx context.Context, source string, err error)
}

func newPolicyEvaluator(commands commandRunner, onError func(ctx context.Context, source string, err error)) *policyEvaluator {
	if lo.IsNil(commands) {
		commands = nil
	}
	return &policyEvaluator{commands: commands, onError: onError}
}

// Preserve authored order: the first denial and first eligible rewrite describe
// different user intent when competing rules both match.
func (r *policyEvaluator) Run(ctx context.Context, hooks []hookdata.Hook, in hookdata.Input) eventAdvice {
	dec := eventAdvice{CommandDecision: hookdata.CommandDecision{Verdict: hookdata.CommandAllow}}
	var commandInput hookdata.Input
	var commandInputErr error
	commandInputReady := false
	for ordinal, h := range hooks {
		if !matches(h, in) {
			continue
		}
		if h.Command == "" {
			dec.Fold(hookdata.CommandAllow, "", strings.TrimSpace(h.Inject), "")
			continue
		}
		if !commandInputReady {
			commandInput, commandInputErr = in.CommandProjection()
			commandInputReady = true
		}
		if commandInputErr != nil {
			r.fail(ctx, h, commandInputErr, &dec)
			continue
		}
		r.runOne(ctx, ordinal, h, commandInput, &dec)
	}
	return dec
}

func (r *policyEvaluator) runOne(ctx context.Context, ordinal int, h hookdata.Hook, in hookdata.Input, dec *eventAdvice) {
	if r.commands == nil {
		r.fail(ctx, h, errors.New("hook command runner is not configured"), dec)
		return
	}
	timeout := defaultTimeout
	if h.TimeoutMillis > 0 {
		timeout = time.Duration(h.TimeoutMillis) * time.Millisecond
	}
	result := r.commands.RunHookCommand(ctx, commandRequest{
		Ordinal: ordinal, Command: h.Command,
		CWD:     in.CWD,
		Input:   in,
		Timeout: timeout,
	})
	if result.TimedOut {
		r.fail(ctx, h, errors.Join(errors.New("hook timed out"), result.Err), dec)
		return
	}

	out := result.Decision

	switch {
	case result.ExitCode == blockExitCode:
		reason := out.Reason
		if reason == "" {
			reason = strings.TrimSpace(result.Stderr)
		}
		dec.Fold(hookdata.CommandDeny, reason, strings.TrimSpace(out.InjectContext), "")
	case result.Err == nil && result.ExitCode == 0:
		// Exit 0 must carry a complete decoded decision. Treat a missing or
		// future verdict as a broken hook instead of silently granting it.
		if !out.Verdict.Valid() {
			r.fail(ctx, h, errors.New("hook returned an invalid command verdict"), dec)
			return
		}
		verdict := out.Verdict
		reason := out.Reason
		if verdict == hookdata.CommandDeny && reason == "" {
			reason = strings.TrimSpace(result.Stderr)
		}
		dec.Fold(verdict, reason, strings.TrimSpace(out.InjectContext), strings.TrimSpace(out.RewriteArguments))
	default:
		r.fail(ctx, h, hookError(result.ExitCode, result.Stderr, result.Err), dec)
	}
}

func (r *policyEvaluator) fail(ctx context.Context, rule hookdata.Hook, err error, dec *eventAdvice) {
	if r.onError != nil {
		r.onError(ctx, rule.Source, err)
	}
	if rule.FailurePolicy.Effective() == hookdata.FailureRequired {
		dec.Fold(hookdata.CommandDeny, "Required hook did not return a valid decision: "+rule.Source, "", "")
	}
}

func hookError(exit int, stderr string, runErr error) error {
	if s := strings.TrimSpace(stderr); s != "" {
		return errors.Join(runErr, errors.New(s))
	}
	if runErr != nil {
		return runErr
	}
	return errors.New("hook exited with code " + strconv.Itoa(exit))
}

func (d *eventAdvice) Fold(verdict hookdata.CommandVerdict, reason, inject, rewrite string) {
	switch {
	case verdict == hookdata.CommandDeny && d.Verdict != hookdata.CommandDeny:
		// A denial is final, so a rewrite an earlier hook asked for no longer
		// applies. Folding in a different order must not decide the call
		// differently.
		d.Verdict, d.Reason, d.RewriteArguments = hookdata.CommandDeny, reason, ""
	case verdict == hookdata.CommandAsk && d.Verdict != hookdata.CommandDeny:
		d.Verdict = hookdata.CommandAsk
		if d.Reason == "" {
			d.Reason = reason
		}
	}
	if inject != "" {
		if d.InjectContext != "" {
			d.InjectContext += "\n"
		}
		d.InjectContext += inject
	}
	if rewrite != "" && d.RewriteArguments == "" && d.Verdict != hookdata.CommandDeny {
		d.RewriteArguments = rewrite
	}
}

func matches(h hookdata.Hook, in hookdata.Input) bool {
	if h.Event != in.Event {
		return false
	}
	if in.Event != hookdata.PreToolUse && in.Event != hookdata.PostToolUse {
		return true // non-tool events: matcher is irrelevant
	}
	if h.Matcher == "" {
		return true
	}
	name := ""
	if in.Tool != nil {
		name = in.Tool.Name
	}
	ok, err := path.Match(h.Matcher, name)
	return err == nil && ok
}
