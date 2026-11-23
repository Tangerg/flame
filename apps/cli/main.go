package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Tangerg/flame/apps/cli/internal/delivery/cmd"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
)

const signalExitBase = 128

func main() {
	os.Exit(run())
}

func run() int {
	flameHome, err := localruntime.ProductRoot()
	if err != nil {
		return reportFailure(err)
	}
	owner, err := newRuntimeOwnerAt(flameHome)
	if err != nil {
		return reportFailure(err)
	}
	return runWithDependencies(runtimeDependencies(owner, filepath.Join(flameHome, "cli")), owner)
}

func runWithDependencies(dependencies cmd.Dependencies, closer runtimeCloser) int {
	ctx, stop := processSignalContext(context.Background())
	defer stop()

	root, err := cmd.NewRoot(dependencies)
	if err != nil {
		return reportFailure(errors.Join(err, closeRuntimeOwner(closer)))
	}
	root.SetIn(os.Stdin)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	err = errors.Join(root.ExecuteContext(ctx), closeRuntimeOwner(closer))
	if cause := context.Cause(ctx); cause != nil {
		err = errors.Join(cause, err)
	}
	if err == nil {
		return 0
	}
	return reportFailure(err)
}

func reportFailure(err error) int {
	_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
	return exitCode(err)
}

type runtimeCloser interface {
	Close() error
}

func closeRuntimeOwner(owner runtimeCloser) error {
	if owner == nil {
		return nil
	}
	if err := owner.Close(); err != nil {
		return fmt.Errorf("close runtime: %w", err)
	}
	return nil
}

type exitCoder interface {
	error
	ExitCode() int
}

func exitCode(err error) int {
	if coded, ok := errors.AsType[exitCoder](err); ok {
		return coded.ExitCode()
	}
	if errors.Is(err, context.Canceled) {
		return signalExitCode(syscall.SIGINT)
	}
	return 1
}

// Shells report a process ended by signal N with exit status 128+N.
func signalExitCode(signal syscall.Signal) int { return signalExitBase + int(signal) }

type processSignalError struct{ signal os.Signal }

func (p processSignalError) Error() string { return fmt.Sprintf("terminated by %s", p.signal) }

func (p processSignalError) Unwrap() error { return context.Canceled }

func (p processSignalError) ExitCode() int {
	switch p.signal {
	case os.Interrupt:
		return signalExitCode(syscall.SIGINT)
	case syscall.SIGTERM:
		return signalExitCode(syscall.SIGTERM)
	default:
		return 1
	}
}

func processSignalContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case received := <-signals:
			// Restore the default handler after the first request so a second signal
			// can still terminate a process whose graceful shutdown is stuck.
			signal.Stop(signals)
			cancel(processSignalError{signal: received})
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(signals)
		cancel(context.Canceled)
		<-done
	}
}
