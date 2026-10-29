//go:build unix

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
)

func TestRunFormatterRetainsDeadlineAndProcessFailure(t *testing.T) {
	cause := errors.New("formatting expired")
	ctx, cancel := context.WithTimeoutCause(t.Context(), 100*time.Millisecond, cause)
	defer cancel()
	started := time.Now()
	_, err := runFormatter(ctx, nil, "/bin/sh", "-c", "sleep 3 & wait")
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, cause) {
		t.Errorf("formatter error = %v, want deadline and owner cause", err)
	}
	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Errorf("formatter error = %v, want observed process failure", err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Errorf("formatter took %v after a 100ms deadline", elapsed)
	}
}

func TestRunFormatterRetainsCapacityAndExitFailure(t *testing.T) {
	output, err := runFormatter(t.Context(), nil, "/bin/sh", "-c", fmt.Sprintf(
		"/usr/bin/head -c %d /dev/zero; exit 17", codeintel.MaxDocumentBytes+1))
	if output != nil {
		t.Errorf("formatter admitted %d bytes after an output refusal", len(output))
	}
	if !errors.Is(err, errFormattedTooLarge) {
		t.Errorf("formatter error = %v, want output admission refusal", err)
	}
	if exited, ok := errors.AsType[*exec.ExitError](err); !ok || exited.ExitCode() != 17 {
		t.Errorf("formatter error = %v, want observed exit 17", err)
	}
}

func TestFormatPathRefusesImplicitWorkingDirectoryFormatter(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	t.Setenv("PATH", ".")
	t.Setenv("GODEBUG", "execerrdot=1")
	if err := os.WriteFile(filepath.Join(directory, "prettier"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "file.ts")
	input := []byte("const value = 1;\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := codeintel.NewDocument(directory, "file.ts", string(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := formatDocument(t.Context(), source); !errors.Is(err, exec.ErrDot) {
		t.Fatalf("formatter lookup error = %v, want refusal of implicit executable", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, input) {
		t.Fatalf("source = %q, %v; want preserved source", got, err)
	}
}

func TestRunFormatterRetiresDescendantsAfterSuccessfulRootExit(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	input := []byte("complete\n")
	output, err := runFormatter(t.Context(), input, "/bin/sh", "-c",
		"sleep 10 >/dev/null 2>&1 & printf '%s' \"$!\" >\"$1\"; cat", "formatter", pidFile)
	if err != nil || !bytes.Equal(output, input) {
		t.Fatalf("formatter result = %q, %v; want complete source", output, err)
	}
	encoded, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(encoded))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid child pid %q: %v", encoded, err)
	}
	live := func() bool {
		state, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		if exited, ok := errors.AsType[*exec.ExitError](err); ok && exited.ExitCode() == 1 {
			return false
		}
		if err != nil {
			t.Fatalf("inspect formatter child: %v", err)
		}
		status := strings.TrimSpace(string(state))
		return status != "" && !strings.HasPrefix(status, "Z")
	}
	t.Cleanup(func() {
		if live() {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	deadline := time.Now().Add(time.Second)
	for live() {
		if time.Now().After(deadline) {
			t.Fatalf("successful formatter exit left child %d running", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
