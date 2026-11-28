package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
	"github.com/Tangerg/flame/packages/go-sdk/procgroup"
)

const (
	maxOutputBytes   = 64 << 20
	maxErrorBytes    = 64 << 10
	processWaitDelay = time.Second
)

var ErrOutputTooLarge = errors.New("process: output too large")

type Result struct {
	Stdout   []byte
	Stderr   string
	ExitCode int
}

func Run(ctx context.Context, overrides []string, input io.Reader, args ...string) (Result, error) {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return Result{}, err
	}
	command := exec.CommandContext(ctx, "git", args...)
	command.Stdin = input
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
		return Result{}, fmt.Errorf("process: start: %w", errors.Join(err, errors.Join(ctx.Err(), context.Cause(ctx))))
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
	cancelErr := errors.Join(ctx.Err(), context.Cause(ctx))
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
	contextReader, err := capture.NewReader(ctx, source)
	if err != nil {
		return nil, err
	}
	output, err := io.ReadAll(io.LimitReader(contextReader, maxOutputBytes+1))
	if err != nil {
		err = fmt.Errorf("process: read stdout: %w", err)
	}
	if len(output) > maxOutputBytes {
		err = errors.Join(err, fmt.Errorf("%w: stdout exceeds %d bytes", ErrOutputTooLarge, maxOutputBytes))
	}
	return output, err
}

var parsingLocale = []string{"LC_ALL=C", "LANG=C"}

func At(ctx context.Context, dir string, args ...string) (Result, error) {
	if len(args) > 0 {
		switch args[0] {
		case "status", "diff", "ls-files":
			return ObserveWorktree(ctx, dir, func(query WorktreeQuery) (Result, error) {
				return query(nil, args...)
			})
		}
	}
	prefix, env := worktreeInvocation(dir)
	return Run(ctx, env, nil, append(prefix, args...)...)
}

func ApplyIndexPatch(ctx context.Context, dir string, patch string) (Result, error) {
	prefix, env := worktreeInvocation(dir)
	// Git checks that affected files match the index before writing, and local
	// apply.whitespace settings must not rewrite the reviewed patch.
	args := append(prefix, "apply", "--index", "--whitespace=nowarn", "-")
	return Run(ctx, env, strings.NewReader(patch), args...)
}

func PrepareCheckoutTree(ctx context.Context, repository, head, patch, index string) (string, error) {
	if patch == "" {
		return head, nil
	}
	prefix, env := worktreeInvocation(repository)
	env = append(env, "GIT_INDEX_FILE="+index)
	run := func(input io.Reader, args ...string) (string, error) {
		result, err := Run(ctx, env, input, append(prefix, args...)...)
		if err != nil {
			return "", err
		}
		if result.ExitCode != 0 {
			return "", fmt.Errorf("git %s exited %d: %s", args[0], result.ExitCode, strings.TrimSpace(result.Stderr))
		}
		return strings.TrimSuffix(string(result.Stdout), "\n"), nil
	}
	if _, err := run(nil, "read-tree", head); err != nil {
		return "", err
	}
	if _, err := run(strings.NewReader(patch), "apply", "--cached", "--whitespace=nowarn", "-"); err != nil {
		return "", err
	}
	return run(nil, "write-tree")
}

func worktreeInvocation(dir string) (prefix, env []string) {
	prefix = []string{"--no-pager", "--no-optional-locks", "-C", dir, "-c", "core.quotepath=false", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + os.DevNull, "-c", "diff.ignoreSubmodules=dirty", "-c", "submodule.recurse=false"}
	env = append([]string{"GIT_NO_LAZY_FETCH=1", "GIT_ALLOW_PROTOCOL=", "GIT_NO_REPLACE_OBJECTS=1"}, parsingLocale...)
	return prefix, env
}
