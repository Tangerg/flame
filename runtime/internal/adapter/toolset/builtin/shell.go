package builtin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/content"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/toolfailure"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
	"github.com/Tangerg/flame/runtime/internal/optional"
)

// Shell tools over a shared [exec.Shells]: the primary `shell` tool plus
// `read_shell_output` / `stop_shell` for the jobs it leaves running.
//
// Every command — foreground or explicitly backgrounded — starts as a detached
// job in that shell set. A foreground command races its completion against an
// auto-background window: finishing in time yields its output inline and the
// job is removed; outliving the window leaves it running, addressable by the
// same shell id, so the lifecycle tools work on it unchanged. This is the
// auto-background design — flame selects on the per-shell done channel
// instead of polling. cwd is read per call (executionctx.CWD) so a command runs in the
// session's working directory.

// defaultAutoBackgroundSeconds is how long a foreground shell command may run
// before it is moved to the background (so the Run isn't blocked on a build /
// dev server). Overridable per call via
// auto_background_after_seconds.
const defaultAutoBackgroundSeconds = 60

type shellArgs struct {
	Command                    string `json:"command" jsonschema:"minLength=1" jsonschema_description:"Shell command line, run by /bin/sh -c. Each call starts a fresh shell; directory changes, variables, and shell options do not persist."`
	Description                string `json:"description" jsonschema:"minLength=1,maxLength=120" jsonschema_description:"Concise action phrase shown while the command runs, such as Run backend tests. Describe the command's purpose; do not copy the command or predict its result."`
	TimeoutMillis              *int   `json:"timeout_millis,omitzero" jsonschema:"minimum=1" jsonschema_description:"Hard execution timeout in milliseconds. Omit for no hard timeout."`
	RunInBackground            bool   `json:"run_in_background,omitzero" jsonschema_description:"Return immediately with a shell_id while the command keeps running. Use for servers and watchers."`
	AutoBackgroundAfterSeconds *int   `json:"auto_background_after_seconds,omitzero" jsonschema:"minimum=1" jsonschema_description:"Move a foreground command to the background after this many seconds. Defaults to 60."`
}

func (s shellArgs) validate() error {
	if s.Command == "" {
		return errors.New("shell: command is required")
	}
	if strings.TrimSpace(s.Description) == "" {
		return errors.New("shell: description is required")
	}
	if strings.TrimSpace(s.Description) != s.Description {
		return errors.New("shell: description must not have surrounding whitespace")
	}
	if s.RunInBackground && s.AutoBackgroundAfterSeconds != nil {
		return errors.New("shell: auto_background_after_seconds cannot be used when run_in_background=true")
	}
	return nil
}

func (s shellArgs) timeout() (exec.Timeout, error) {
	return optionalShellTimeout(s.TimeoutMillis, time.Millisecond, "timeout_millis")
}

func (s shellArgs) autoBackgroundAfter() (time.Duration, error) {
	after := optional.Value(s.AutoBackgroundAfterSeconds, defaultAutoBackgroundSeconds)
	if int64(after) > math.MaxInt64/int64(time.Second) {
		return 0, errors.New("shell: auto_background_after_seconds exceeds duration range")
	}
	return time.Duration(after) * time.Second, nil
}

type shellOutputArgs struct {
	ShellID       string `json:"shell_id" jsonschema:"required" jsonschema_description:"Background shell id returned by shell when a long-running command was moved to the background."`
	Wait          bool   `json:"wait,omitzero" jsonschema_description:"Wait for the shell to exit before returning new output. Use this instead of sleep polling; avoid waiting indefinitely on a server or watcher."`
	TimeoutMillis *int   `json:"timeout_millis,omitzero" jsonschema:"minimum=1" jsonschema_description:"When wait=true, maximum milliseconds to wait before returning current output. Omit to wait until exit. Do not pass when wait=false."`
}

func (s shellOutputArgs) validate() error {
	if s.ShellID == "" {
		return errors.New("read_shell_output: shell_id is required")
	}
	if !s.Wait && s.TimeoutMillis != nil {
		return errors.New("read_shell_output: timeout_millis requires wait=true")
	}
	return nil
}

type shellIDArgs struct {
	ShellID string `json:"shell_id" jsonschema:"required" jsonschema_description:"Background shell id returned by shell when a long-running command was moved to the background."`
}

func (s shellIDArgs) validate() error {
	if s.ShellID == "" {
		return errors.New("stop_shell: shell_id is required")
	}
	return nil
}

type commandTools struct {
	shells *exec.Shells
}

func BuildShell(shells *exec.Shells) ([]toolcontract.Tool, error) {
	if shells == nil {
		return nil, errors.New("shell: shells is nil")
	}
	t := &commandTools{shells: shells}

	shellTool, err := toolcontract.NewFunc[shellArgs, string](
		toolcontract.FuncConfig{
			Name: string(tool.Shell),
			Description: "Execute a shell command via /bin/sh -c. Returns stdout/stderr, exit code, and duration. " +
				"Set description to a concise action label that explains the command's purpose while it runs. " +
				"Avoid `find`, `grep`, `cat`, `head`, `tail`, `sed`, `awk` here — use the dedicated `glob`, `grep`, and `read` tools instead; use `apply_patch` for file changes. Reserve `shell` for operations that genuinely need a shell (build commands, git, package managers, etc.). " +
				"Each invocation starts a fresh shell — `cd`, exported variables, and shell options do not persist between calls. " +
				"A command still running after auto_background_after_seconds (default 60) is moved to the background; continue with read_shell_output or stop_shell. Set run_in_background to background it immediately.",
		},
		t.run,
	)
	if err != nil {
		return nil, fmt.Errorf("shell: build shell tool: %w", err)
	}
	outputTool, err := toolcontract.NewFunc[shellOutputArgs, string](
		toolcontract.FuncConfig{
			Name:        string(tool.ReadShellOutput),
			Description: "Read only the new output produced by a background shell since the previous read and report whether it is still running. Reading the final output of a finished shell releases its shell_id. Set wait=true to wait event-first for exit instead of sleep polling; bound that wait with timeout_millis for servers or watchers.",
		},
		t.output,
	)
	if err != nil {
		return nil, fmt.Errorf("shell: build read_shell_output tool: %w", err)
	}
	killTool, err := toolcontract.NewFunc[shellIDArgs, string](
		toolcontract.FuncConfig{
			Name:        string(tool.StopShell),
			Description: "Stop one background shell by the shell_id returned from shell.",
		},
		t.kill,
	)
	if err != nil {
		return nil, fmt.Errorf("shell: build stop_shell tool: %w", err)
	}
	return []toolcontract.Tool{shellTool, outputTool, killTool}, nil
}

func (c *commandTools) run(ctx context.Context, a shellArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", toolfailure.Definite(err)
	}
	timeout, err := a.timeout()
	if err != nil {
		return "", toolfailure.Definite(err)
	}
	autoBackgroundAfter, err := a.autoBackgroundAfter()
	if err != nil {
		return "", toolfailure.Definite(err)
	}

	cwd, attached := executionctx.CWD(ctx)
	if !attached {
		return "", errors.New("shell: no attached Run workspace")
	}
	id, err := c.shells.Launch(ctx, executionctx.SessionID(ctx), cwd, a.Command, timeout, executionctx.Isolated(ctx))
	if err != nil {
		return "", err
	}
	sh, ok := c.shells.Get(executionctx.SessionID(ctx), id)
	if !ok { // just launched — unreachable
		return "", fmt.Errorf("shell: background shell %s vanished", id)
	}
	if a.RunInBackground {
		// A command that failed to start is published already finished. Handing
		// back a background handle for it would send the model to poll a shell
		// that never ran, so a shell that is already done reports what it did.
		select {
		case <-sh.Done():
			return c.completed(executionctx.SessionID(ctx), id, nil)
		default:
			return backgroundedJSON(id)
		}
	}
	timer := time.NewTimer(autoBackgroundAfter)
	defer timer.Stop()
	select {
	case <-sh.Done():
		return c.completed(executionctx.SessionID(ctx), id, nil)
	case <-timer.C:
		return backgroundedJSON(id) // still running — leave it
	case <-ctx.Done():
		return c.cancelForeground(ctx, id, sh)
	}
}

func (c *commandTools) completed(sessionID, id string, cause error) (string, error) {
	output, readErr := c.shells.Read(sessionID, id)
	if !output.Finished {
		return "", errors.Join(cause, readErr, errors.New("shell: completion has no finished output"))
	}
	result, resultErr := completedJSON(output)
	return shellResult(result, errors.Join(cause, readErr, resultErr))
}

func (c *commandTools) cancelForeground(ctx context.Context, id string, sh *exec.Shell) (string, error) {
	// The command may have finished in the same instant the Run was canceled;
	// select picks a ready case at random, so check Done() before discarding a
	// completed result the user can still use.
	select {
	case <-sh.Done():
		return c.completed(executionctx.SessionID(ctx), id, nil)
	default:
		// Join cleanup before consuming the final output; a cleanup failure must
		// leave its resource in the owner ledger for teardown.
		if _, err := c.shells.Kill(executionctx.SessionID(ctx), id); err != nil && !errors.Is(err, exec.ErrShellNotFound) {
			return "", errors.Join(ctx.Err(), fmt.Errorf("shell: stop canceled foreground command %q: %w", id, err))
		}
		<-sh.Done()
		return c.completed(executionctx.SessionID(ctx), id, ctx.Err())
	}
}

func (c *commandTools) output(ctx context.Context, a shellOutputArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", toolfailure.Definite(err)
	}
	sh, ok := c.shells.Get(executionctx.SessionID(ctx), a.ShellID)
	if !ok {
		return fmt.Sprintf("No background shell %s.", a.ShellID), nil
	}
	if a.Wait {
		timeout, err := optionalShellTimeout(a.TimeoutMillis, time.Millisecond, "timeout_millis")
		if err != nil {
			return "", toolfailure.Definite(err)
		}
		if err := waitForShell(ctx, sh, timeout); err != nil {
			return "", err
		}
	}
	output, readErr := c.shells.Read(executionctx.SessionID(ctx), a.ShellID)
	if errors.Is(readErr, exec.ErrShellNotFound) {
		return fmt.Sprintf("No background shell %s.", a.ShellID), nil
	}
	state := "still running"
	if output.Finished {
		state = "finished (" + output.Info + ")"
	}
	encoded, err := json.Marshal(struct {
		ShellID string          `json:"shell_id"`
		Status  string          `json:"status"`
		Stdout  content.Content `json:"stdout"`
		Dropped bool            `json:"output_dropped,omitzero"`
	}{ShellID: a.ShellID, Status: state, Stdout: content.New([]byte(output.Text)), Dropped: output.Dropped})
	if err != nil {
		return "", errors.Join(readErr, fmt.Errorf("shell: encode background command output: %w", err))
	}
	return shellResult(string(encoded), readErr)
}

// A failed cleanup or canceled wait does not erase observed process output.
// CallError preserves that evidence without declaring the command retry-safe.
func shellResult(result string, cause error) (string, error) {
	if cause == nil || result == "" {
		return result, cause
	}
	callErr, err := toolcontract.NewCallError(toolcontract.CallErrorConfig{
		Cause: cause, Evidence: chat.NewTextToolOutput(result),
	})
	if err != nil {
		return "", errors.Join(cause, err)
	}
	return "", callErr
}

func (c *commandTools) kill(ctx context.Context, a shellIDArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", toolfailure.Definite(err)
	}
	running, err := c.shells.Kill(executionctx.SessionID(ctx), a.ShellID)
	switch {
	case errors.Is(err, exec.ErrShellNotFound):
		return fmt.Sprintf("No background shell %s.", a.ShellID), nil
	case err != nil:
		return "", fmt.Errorf("shell: kill background shell %q: %w", a.ShellID, err)
	case running:
		return fmt.Sprintf("Killed background shell %s.", a.ShellID), nil
	default:
		return fmt.Sprintf("Background shell %s had already exited.", a.ShellID), nil
	}
}

// completedJSON shapes a finished foreground command's result. The combined
// stdout+stderr goes in "stdout" because the execution ring preserves their
// combined arrival order. exit_code is always present for a finished command.
func completedJSON(output exec.Output) (string, error) {
	out := output.Text
	if output.Dropped {
		out = "[earlier output dropped — buffer overflowed]\n" + out
	}
	// A command with no exit status never reported one: it failed to start, or
	// could not be waited on, and info carries the only account of why. An
	// ordinary exit restates exit_code there, which is worth nothing here.
	if output.ExitCode == exec.NoExitStatus && strings.TrimSpace(output.Info) != "" {
		out = strings.TrimLeft(output.Info+"\n"+out, "\n")
	}
	b, err := json.Marshal(struct {
		Stdout   content.Content `json:"stdout"`
		ExitCode int             `json:"exit_code"`
		Killed   bool            `json:"killed,omitzero"`
		Duration string          `json:"duration"`
	}{Stdout: content.New([]byte(out)), ExitCode: output.ExitCode, Killed: output.Killed, Duration: output.Duration.String()})
	if err != nil {
		return "", fmt.Errorf("shell: encode completed command result: %w", err)
	}
	return string(b), nil
}

// backgroundedJSON is the result for a command left running (explicit
// run_in_background or auto-backgrounded). It omits exit_code — the command
// has not exited and therefore has no exit status.
func backgroundedJSON(id string) (string, error) {
	b, err := json.Marshal(struct {
		ShellID string          `json:"shell_id"`
		Stdout  content.Content `json:"stdout"`
	}{ShellID: id, Stdout: content.New([]byte(fmt.Sprintf(
		"Command running in background as shell %s. Continue with read_shell_output {\"shell_id\":%q} or stop_shell {\"shell_id\":%q}.",
		id, id, id)))})
	if err != nil {
		return "", fmt.Errorf("shell: encode background command result: %w", err)
	}
	return string(b), nil
}

// waitForShell blocks until sh exits, ctx is canceled, or an enabled timeout
// elapses. It reuses the same per-shell done channel the shell
// foreground path selects on (no polling). A timeout is NOT an error: the
// caller then reports the current still-running output, just as if wait were
// off. Returns ctx.Err() when the caller context ends.
func waitForShell(ctx context.Context, sh *exec.Shell, timeout exec.Timeout) error {
	duration, enabled := timeout.Duration()
	if !enabled {
		select {
		case <-sh.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-sh.Done():
	case <-timer.C: // still running — fall through to report current state
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func optionalShellTimeout(value *int, unit time.Duration, field string) (exec.Timeout, error) {
	if value == nil {
		return exec.Timeout{}, nil
	}
	if *value <= 0 {
		return exec.Timeout{}, fmt.Errorf("shell: %s must be positive", field)
	}
	if int64(*value) > math.MaxInt64/int64(unit) {
		return exec.Timeout{}, fmt.Errorf("shell: %s exceeds duration range", field)
	}
	timeout, err := exec.NewTimeout(time.Duration(*value) * unit)
	if err != nil {
		return exec.Timeout{}, fmt.Errorf("shell: %s: %w", field, err)
	}
	return timeout, nil
}
