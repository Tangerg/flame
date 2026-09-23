package hooks

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
	"unicode/utf8"

	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/infra/process/procgroup"
)

const (
	maxHookCommandInputBytes  = 512 << 10
	maxHookCommandOutputBytes = 64 << 10
	hookProcessWaitDelay      = 2 * time.Second
)

// Shell executes hook commands with the host shell.
type Shell struct{}

// RunHookCommand runs req.Command via the host shell, encoding the typed domain
// input into the external hook JSON contract at this adapter boundary.
func (Shell) RunHookCommand(ctx context.Context, req apphooks.CommandRequest) apphooks.CommandResult {
	if err := ctx.Err(); err != nil {
		return apphooks.CommandResult{
			Err: err, ExitCode: -1,
			TimedOut: errors.Is(err, context.DeadlineExceeded),
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
	stdin, err := json.Marshal(hookInputWireFrom(req.Input))
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
	stdout := newHookOutputBuffer(maxHookCommandOutputBytes)
	stderr := newHookOutputBuffer(maxHookCommandOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = hookProcessWaitDelay
	procgroup.Prepare(cmd)
	cmd.Cancel = func() error { return procgroup.Stop(cmd) }

	runErr := cmd.Run()
	cleanupErr := procgroup.Stop(cmd)
	if errors.Is(cleanupErr, os.ErrProcessDone) {
		cleanupErr = nil
	}
	result := apphooks.CommandResult{
		Stderr:   stderr.String(),
		ExitCode: exitCodeOf(runErr),
		Err:      errors.Join(runErr, cleanupErr),
		TimedOut: cctx.Err() == context.DeadlineExceeded,
	}
	if stdout.overflow {
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

type hookInputWire struct {
	Event           domainhooks.Event  `json:"event"`
	SessionID       string             `json:"sessionId,omitempty"`
	CWD             string             `json:"cwd,omitempty"`
	Tool            *hookToolInputWire `json:"tool,omitempty"`
	Prompt          string             `json:"prompt,omitempty"`
	PromptTruncated bool               `json:"promptTruncated,omitempty"`
	Reason          string             `json:"reason,omitempty"`
}

type hookToolInputWire struct {
	Name            string `json:"name"`
	Arguments       string `json:"arguments,omitempty"`
	Result          string `json:"result,omitempty"`
	ResultTruncated bool   `json:"resultTruncated,omitempty"`
}

func hookInputWireFrom(input domainhooks.Input) hookInputWire {
	out := hookInputWire{
		Event: input.Event, SessionID: input.SessionID, CWD: input.CWD,
		Prompt: input.Prompt, PromptTruncated: input.PromptTruncated, Reason: input.Reason,
	}
	if input.Tool != nil {
		out.Tool = &hookToolInputWire{
			Name: input.Tool.Name, Arguments: input.Tool.Arguments,
			Result: input.Tool.Result, ResultTruncated: input.Tool.ResultTruncated,
		}
	}
	return out
}

func failedHookCommandInput(err error) apphooks.CommandResult {
	return apphooks.CommandResult{
		Err: fmt.Errorf("hooks: command input: %w", err), ExitCode: -1,
	}
}

func hookInputMaterialWithinLimit(input domainhooks.Input, limit int) bool {
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
	Decision         apphooks.CommandVerdict `json:"decision,omitempty"`
	Reason           string                  `json:"reason,omitempty"`
	InjectContext    string                  `json:"injectContext,omitempty"`
	RewriteArguments string                  `json:"rewriteArguments,omitempty"`
}

func hookDecisionFromWire(stdout []byte) (apphooks.CommandDecision, error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return apphooks.CommandDecision{Verdict: apphooks.CommandAllow}, nil
	}
	if !utf8.Valid(stdout) {
		return apphooks.CommandDecision{}, errors.New("hooks: command decision is not valid UTF-8")
	}
	if trimmed[0] != '{' {
		return apphooks.CommandDecision{}, errors.New("hooks: command decision must be a JSON object")
	}
	var wire hookDecisionWire
	if err := json.Unmarshal(stdout, &wire, json.RejectUnknownMembers(true)); err != nil {
		return apphooks.CommandDecision{}, fmt.Errorf("hooks: decode command decision: %w", err)
	}
	verdict, err := hookVerdictFromWire(wire.Decision)
	if err != nil {
		return apphooks.CommandDecision{}, err
	}
	return apphooks.CommandDecision{
		Verdict: verdict, Reason: wire.Reason,
		InjectContext: wire.InjectContext, RewriteArguments: wire.RewriteArguments,
	}, nil
}

func hookVerdictFromWire(verdict apphooks.CommandVerdict) (apphooks.CommandVerdict, error) {
	if verdict == "" {
		return apphooks.CommandAllow, nil
	}
	if !verdict.Valid() {
		return "", fmt.Errorf("hooks: unsupported command decision %q", verdict)
	}
	return verdict, nil
}

type hookOutputBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func newHookOutputBuffer(limit int) *hookOutputBuffer {
	return &hookOutputBuffer{limit: limit}
}

func (h *hookOutputBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := h.limit - h.buffer.Len()
	if remaining > 0 {
		_, _ = h.buffer.Write(value[:min(len(value), remaining)])
	}
	if len(value) > remaining {
		h.overflow = true
	}
	return written, nil
}

func (h *hookOutputBuffer) Bytes() []byte  { return h.buffer.Bytes() }
func (h *hookOutputBuffer) String() string { return h.buffer.String() }

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	return -1
}
