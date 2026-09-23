package process

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/capture"
	"io"
	"os/exec"
	"time"
)

const (
	maxOutputBytes   = 64 << 20
	maxErrorBytes    = 64 << 10
	processWaitDelay = time.Second
)

// ErrOutputTooLarge reports a Git command whose stdout cannot enter a Runtime
// VCS read model as one complete value.
var ErrOutputTooLarge = errors.New("process: output too large")

// Result is one completed Git command. Non-zero process exits are data here so
// the Git use case can interpret documented predicate and diff exit codes.
type Result struct {
	Stdout   []byte
	Stderr   string
	ExitCode int
}

// Run executes Git with bounded stdout and a bounded-drain stderr. overrides
// are command-owned environment entries such as a stable parsing locale.
func Run(ctx context.Context, overrides []string, args ...string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	command := CommandContext(ctx, args...)
	if len(overrides) > 0 {
		command.Env = Environment(overrides...)
	}
	command.WaitDelay = processWaitDelay
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("process: stdout pipe: %w", err)
	}
	stderr := capture.NewWriter(maxErrorBytes)
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return Result{}, fmt.Errorf("process: start: %w", err)
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxOutputBytes+1))
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		return Result{}, fmt.Errorf("process: read stdout: %w", readErr)
	}
	if len(output) > maxOutputBytes {
		_ = command.Process.Kill()
		_ = command.Wait()
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		return Result{}, fmt.Errorf("%w: stdout exceeds %d bytes", ErrOutputTooLarge, maxOutputBytes)
	}
	waitErr := command.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	stderrText := stderr.String()
	if stderr.Truncated() {
		stderrText += "\n... [git stderr truncated] ..."
	}
	result := Result{Stdout: output, Stderr: stderrText}
	if waitErr == nil {
		return result, nil
	}
	if exit, ok := errors.AsType[*exec.ExitError](waitErr); ok {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	return Result{}, fmt.Errorf("process: wait: %w", waitErr)
}
