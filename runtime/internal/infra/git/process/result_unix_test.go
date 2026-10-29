//go:build unix

package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

func gitFixture(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRunDeadlineStopsDescendantsAndPreservesCause(t *testing.T) {
	gitFixture(t, "sleep 3 &\nwait\n")
	cause := errors.New("git observation expired")
	ctx, cancel := context.WithTimeoutCause(t.Context(), 100*time.Millisecond, cause)
	defer cancel()
	started := time.Now()
	_, err := Run(ctx, nil, "status")
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, cause) {
		t.Errorf("Run error = %v, want deadline and owner cause", err)
	}
	if elapsed := time.Since(started); elapsed >= 2*time.Second {
		t.Errorf("Run took %v after a 100ms deadline; descendant retained stdout", elapsed)
	}
}

func TestRunKeepsDocumentedNonzeroExitAsData(t *testing.T) {
	gitFixture(t, "printf 'observed output'\nprintf 'observed diagnostic' >&2\nexit 17\n")
	result, err := Run(t.Context(), nil, "status")
	if err != nil || result.ExitCode != 17 || string(result.Stdout) != "observed output" || result.Stderr != "observed diagnostic" {
		t.Fatalf("Run = %+v, %v; want the actual process result", result, err)
	}
}

func TestRootExitDoesNotWaitForInheritedPipes(t *testing.T) {
	for _, redirect := range []string{"2>/dev/null", ">/dev/null", ""} {
		t.Run(redirect, func(t *testing.T) {
			gitFixture(t, "sleep 3 "+redirect+" &\nprintf 'complete'\nprintf 'diagnostic' >&2\nexit 17\n")
			started := time.Now()
			result, err := Run(t.Context(), nil, "status")
			if err != nil || result.ExitCode != 17 || string(result.Stdout) != "complete" || result.Stderr != "diagnostic" {
				t.Fatalf("root result = %+v, %v", result, err)
			}
			if elapsed := time.Since(started); elapsed >= 2*time.Second {
				t.Fatalf("root completion waited %v for inherited pipes", elapsed)
			}
		})
	}
}

func TestDrainBudgetDoesNotLimitAnExecutingRoot(t *testing.T) {
	gitFixture(t, "sleep 1.2\nprintf 'complete'\n")
	result, err := Run(t.Context(), nil, "status")
	if err != nil || string(result.Stdout) != "complete" {
		t.Fatalf("long-running root = %+v, %v", result, err)
	}
}

func TestRunKeepsBinaryDiagnosticPrintable(t *testing.T) {
	gitFixture(t, "printf '\\377' >&2\nexit 17\n")
	result, err := Run(t.Context(), nil, "status")
	if err != nil || result.ExitCode != 17 || !utf8.ValidString(result.Stderr) || !strings.Contains(result.Stderr, `\xff`) {
		t.Fatalf("binary diagnostic = %+v, %v; want printable process evidence", result, err)
	}
}

func TestRunRetiresDescendantsAfterSuccessfulRootExit(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	gitFixture(t, "sleep 10 >/dev/null 2>&1 &\nprintf '%s' \"$!\" >\"$FLAME_GIT_CHILD_PID_FILE\"\nprintf 'complete'\n")
	result, err := Run(t.Context(), []string{"FLAME_GIT_CHILD_PID_FILE=" + pidFile}, "status")
	if err != nil || result.ExitCode != 0 || string(result.Stdout) != "complete" {
		t.Fatalf("root result = %+v, %v", result, err)
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
			t.Fatalf("inspect child: %v", err)
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
			t.Fatalf("successful root exit left child %d running", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
