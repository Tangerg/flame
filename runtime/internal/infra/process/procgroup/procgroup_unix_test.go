//go:build unix

package procgroup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestStopReachesWhatTheChildSpawned is the guarantee the negative pid exists
// for. The shell exits immediately; the process it left behind is what a plain
// Process.Kill would strand, and it is the shape every consumer here has —
// an MCP server behind a package runner, a hook behind a shell.
func TestStopReachesWhatTheChildSpawned(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "grandchild.pid")
	command := exec.Command("/bin/sh", "-c",
		"sh -c 'echo $$ > "+marker+"; sleep 30' & echo started")
	Prepare(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("the launching shell did not exit cleanly: %v", err)
	}
	var pid int
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		body, err := os.ReadFile(marker)
		if err == nil {
			if _, scanErr := fmt.Sscanf(string(body), "%d", &pid); scanErr == nil && pid > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid <= 0 {
		t.Skip("the spawned process never reported its pid on this system")
	}
	if err := Stop(command); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("process %d outlived its group", pid)
}

// TestStopReportsAnAlreadyFinishedProcess keeps the outcome a caller branches
// on: a group that is already gone is done, not an error to report.
func TestStopReportsAnAlreadyFinishedProcess(t *testing.T) {
	t.Parallel()

	if err := Stop(nil); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("Stop(nil) = %v, want os.ErrProcessDone", err)
	}
	command := exec.Command("/bin/sh", "-c", "exit 0")
	Prepare(command)
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	if err := Stop(command); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("Stop of a finished group = %v, want os.ErrProcessDone", err)
	}
}
