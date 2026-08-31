// Command flame is the terminal front end for the flame agent runtime.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Tangerg/flame/cli/internal/cmd"
)

const runtimeCloseAttempts = 3

func main() {
	os.Exit(run())
}

func run() int {
	flameHome, err := flameHomeDirectory()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
		return exitCode(err)
	}
	owner, err := newRuntimeOwnerAt(flameHome)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
		return exitCode(err)
	}
	return runWithDependencies(runtimeDependencies(owner, filepath.Join(flameHome, "cli")), owner)
}

func runWithDependencies(dependencies cmd.Dependencies, closer runtimeCloser) int {
	ctx, stop := processSignalContext(context.Background())
	defer stop()

	root := cmd.NewRoot(dependencies)
	root.SetIn(os.Stdin)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	err := root.ExecuteContext(ctx)
	err = errors.Join(err, closeRuntimeOwner(closer))
	if cause := context.Cause(ctx); cause != nil {
		err = errors.Join(cause, err)
	}
	if err == nil {
		return 0
	}
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
	errorsByAttempt := make([]error, 0, runtimeCloseAttempts)
	for range runtimeCloseAttempts {
		err := owner.Close()
		if err == nil {
			return nil
		}
		errorsByAttempt = append(errorsByAttempt, err)
	}
	return fmt.Errorf("close runtime after %d attempts: %w", runtimeCloseAttempts, errors.Join(errorsByAttempt...))
}

func flameHomeDirectory() (string, error) {
	flameHome := strings.TrimSpace(os.Getenv("FLAME_HOME"))
	if flameHome == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve flame home: %w", err)
		}
		flameHome = filepath.Join(userHome, ".flame")
	}
	if !filepath.IsAbs(flameHome) {
		return "", errors.New("FLAME_HOME must be an absolute path")
	}
	return filepath.Clean(flameHome), nil
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
		return 130
	}
	return 1
}

type processSignalError struct{ signal os.Signal }

func (p processSignalError) Error() string { return fmt.Sprintf("terminated by %s", p.signal) }

func (p processSignalError) Unwrap() error { return context.Canceled }

func (p processSignalError) ExitCode() int {
	switch p.signal {
	case os.Interrupt:
		return 130
	case syscall.SIGTERM:
		return 143
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
