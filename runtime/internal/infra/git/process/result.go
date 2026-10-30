package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
	"github.com/Tangerg/flame/packages/go-sdk/procgroup"
	"github.com/Tangerg/flame/qimu/cancellation"
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
	if err := cancellation.Err(ctx); err != nil {
		return Result{}, err
	}
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = environment(overrides...)
	command.WaitDelay = processWaitDelay
	procgroup.Prepare(command)
	command.Cancel = func() error { return procgroup.Stop(command) }
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		return Result{}, fmt.Errorf("process: stdout pipe: %w", err)
	}
	defer stdout.Close()
	defer stdoutWriter.Close()
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		return Result{}, fmt.Errorf("process: stderr pipe: %w", err)
	}
	defer stderrReader.Close()
	defer stderrWriter.Close()
	command.Stdout = stdoutWriter
	command.Stderr = stderrWriter
	if err := command.Start(); err != nil {
		return Result{}, fmt.Errorf("process: start: %w", errors.Join(err, cancellation.Err(ctx)))
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	stop := func() error {
		err := procgroup.Stop(command)
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
	var output []byte
	stdoutDone := make(chan error, 1)
	go func() {
		var err error
		output, err = readOutput(ctx, stdout)
		if err != nil {
			err = errors.Join(err, stop())
		}
		stdoutDone <- err
	}()
	stderr := capture.NewWriter(maxErrorBytes)
	stderrDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(stderr, stderrReader)
		if err != nil {
			err = errors.Join(err, stop())
		}
		stderrDone <- err
	}()

	// Files belong to this invocation, so Wait observes only the root process.
	// Its exit starts retirement and the drain budget, never an operation timeout.
	waitErr := command.Wait()
	cleanupErr := stop()
	drainDone := make(chan struct{})
	drain := time.AfterFunc(processWaitDelay, func() {
		defer close(drainDone)
		_ = stdout.Close()
		_ = stderrReader.Close()
	})
	readErr := errors.Join(<-stdoutDone, <-stderrDone)
	if !drain.Stop() {
		<-drainDone
	}
	cancelErr := cancellation.Err(ctx)
	if readErr != nil || cleanupErr != nil || cancelErr != nil {
		return Result{}, errors.Join(readErr, waitErr, cleanupErr, cancelErr)
	}
	stderrText := stderr.Diagnostic()
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

func readOutput(ctx context.Context, source io.Reader) ([]byte, error) {
	output, err := io.ReadAll(io.LimitReader(cancellation.Reader(ctx, source), maxOutputBytes+1))
	if err != nil {
		err = fmt.Errorf("process: read stdout: %w", err)
	}
	if len(output) > maxOutputBytes {
		err = errors.Join(err, fmt.Errorf("%w: stdout exceeds %d bytes", ErrOutputTooLarge, maxOutputBytes))
	}
	return output, err
}

// parsingLocale keeps Git's diagnostics and its collation in one language, so a
// caller reading them is reading the same text on every machine.
var parsingLocale = []string{"LC_ALL=C", "LANG=C"}

// At runs Git against the repository at dir with the flags that keep an
// invocation's behavior and its output independent of everything outside the
// request: no pager to block on, no optional lock taken for a read, and no
// path escaping chosen by the user's configuration. Callers pass only what they
// are asking Git to do.
//
// Assembling this prefix per call site is what let it drift: two sites read
// paths Git had escaped because they omitted core.quotepath, and one compared
// output across runs without pinning the locale that produced it.
func At(ctx context.Context, dir string, args ...string) (Result, error) {
	prefix := []string{
		"--no-pager",
		"--no-optional-locks",
		"-C", dir,
		"-c", "core.quotepath=false",
	}
	return Run(ctx, parsingLocale, append(prefix, args...)...)
}
