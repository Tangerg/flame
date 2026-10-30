package main

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	"github.com/Tangerg/flame/packages/go-sdk/procgroup"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/flame/qimu/jsonvalue"
)

const (
	maxHookCommandInputBytes  = hookprovider.MaxCommandInputBytes
	maxHookCommandOutputBytes = hookprovider.MaxCommandOutputBytes
	hookProcessWaitDelay      = 2 * time.Second
)

// Shell executes hook commands with the host shell.
type Shell struct{}

// RunHookCommand runs req.Command via the host shell, encoding the typed domain
// input into the external hook JSON contract at this adapter boundary.
func (Shell) RunHookCommand(ctx context.Context, req commandRequest) commandResult {
	if err := cancellation.Err(ctx); err != nil {
		return commandResult{
			Err: err, ExitCode: -1, TimedOut: ctx.Err() == context.DeadlineExceeded,
		}
	}
	if err := req.Input.ValidateCommandMaterial(); err != nil {
		return failedHookCommandInput(err)
	}
	if !hookInputMaterialWithinLimit(req.Input, maxHookCommandInputBytes) {
		return failedHookCommandInput(fmt.Errorf(
			"raw material exceeds %d bytes",
			maxHookCommandInputBytes,
		))
	}
	stdin, err := json.Marshal(req.Input)
	if err != nil {
		return failedHookCommandInput(fmt.Errorf("encode: %w", err))
	}
	if len(stdin) > maxHookCommandInputBytes {
		return failedHookCommandInput(fmt.Errorf(
			"encoded input uses %d bytes, maximum %d",
			len(stdin),
			maxHookCommandInputBytes,
		))
	}

	cctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	cmd := hookShellCommand(cctx, req.Command)
	cmd.Stdin = bytes.NewReader(stdin)
	if req.CWD != "" {
		cmd.Dir = req.CWD
	}
	stdout := capture.NewWriter(maxHookCommandOutputBytes)
	stderr := capture.NewWriter(maxHookCommandOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = hookProcessWaitDelay
	procgroup.Prepare(cmd)
	cmd.Cancel = func() error { return procgroup.Stop(cmd) }

	runErr := cmd.Run()
	cancellationErr := cancellation.Err(cctx)
	timedOut := cctx.Err() == context.DeadlineExceeded
	cleanupErr := procgroup.Stop(cmd)
	if errors.Is(cleanupErr, os.ErrProcessDone) {
		cleanupErr = nil
	}
	result := commandResult{
		Stderr:   stderr.Diagnostic(),
		ExitCode: exitCodeOf(runErr),
		Err:      errors.Join(runErr, cancellationErr, cleanupErr),
		TimedOut: timedOut,
	}
	if stdout.Truncated() {
		result.Err = errors.Join(
			result.Err,
			fmt.Errorf("hooks: command stdout exceeds %d bytes", maxHookCommandOutputBytes),
		)
		return result
	}
	result.Decision, err = hookDecisionFromWire(stdout.Bytes())
	result.Err = errors.Join(result.Err, err)
	return result
}

func failedHookCommandInput(err error) commandResult {
	return commandResult{
		Err: fmt.Errorf("hooks: command input: %w", err), ExitCode: -1,
	}
}

func hookInputMaterialWithinLimit(input hook.Input, limit int) bool {
	remaining := limit
	consume := func(value string) bool {
		if len(value) > remaining {
			return false
		}
		remaining -= len(value)
		return true
	}
	if !consume(string(input.Event)) ||
		!consume(input.SessionID) ||
		!consume(input.CWD) ||
		!consume(input.Prompt) ||
		!consume(input.Reason) {
		return false
	}
	if input.Tool != nil &&
		(!consume(input.Tool.Name) ||
			!consume(input.Tool.Arguments) ||
			!consume(input.Tool.Result)) {
		return false
	}
	return true
}

type hookDecisionWire struct {
	Decision         *hook.CommandVerdict `json:"decision,omitzero"`
	Reason           string               `json:"reason,omitempty"`
	InjectContext    string               `json:"injectContext,omitempty"`
	RewriteArguments string               `json:"rewriteArguments,omitempty"`
}

func hookDecisionFromWire(stdout []byte) (hook.CommandDecision, error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return hook.CommandDecision{Verdict: hook.CommandAllow}, nil
	}
	if trimmed[0] != '{' {
		return hook.CommandDecision{}, errors.New("hooks: command decision must be a JSON object")
	}
	var wire hookDecisionWire
	if err := jsonvalue.UnmarshalNonNull(stdout, &wire); err != nil {
		return hook.CommandDecision{}, fmt.Errorf("hooks: decode command decision: %w", err)
	}
	verdict, err := hookVerdictFromWire(wire.Decision)
	if err != nil {
		return hook.CommandDecision{}, err
	}
	return hook.CommandDecision{
		Verdict: verdict, Reason: wire.Reason,
		InjectContext: wire.InjectContext, RewriteArguments: wire.RewriteArguments,
	}, nil
}

func hookVerdictFromWire(verdict *hook.CommandVerdict) (hook.CommandVerdict, error) {
	if verdict == nil {
		return hook.CommandAllow, nil
	}
	if !verdict.Valid() {
		return "", fmt.Errorf("hooks: unsupported command decision %q", *verdict)
	}
	return *verdict, nil
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	return -1
}

type commandRequest struct {
	Command string
	CWD     string
	Input   hook.Input
	Timeout time.Duration
}

type commandResult struct {
	Decision hook.CommandDecision
	Stderr   string
	ExitCode int
	Err      error
	TimedOut bool
}

func (r commandResult) project() hookprovider.Result {
	result := hookprovider.Result{Stderr: r.Stderr, ExitCode: r.ExitCode}
	if r.Decision.Verdict.Valid() {
		result.Decision = &r.Decision
	}
	if r.Err != nil {
		kind := hookprovider.Failed
		if r.TimedOut {
			kind = hookprovider.TimedOut
		} else if errors.Is(r.Err, context.Canceled) {
			kind = hookprovider.Canceled
		}
		result.Failure = &hookprovider.Failure{Kind: kind, Message: r.Err.Error()}
	}
	return result
}
