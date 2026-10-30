//go:build unix

package lsp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFailedInitializationReclaimsServerDescendants(t *testing.T) {
	for _, test := range []struct {
		name string
		end  string
	}{
		{name: "launcher exits", end: "exit 0"},
		{name: "initialization canceled", end: "wait"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			pidFile := filepath.Join(root, "descendant.pid")
			t.Setenv("FLAME_LSP_DESCENDANT_PID", pidFile)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := startClient(ctx, ServerSpec{
					Name: "process-test", Command: "/bin/sh",
					Args: []string{"-c", `sleep 30 & echo $! > "$FLAME_LSP_DESCENDANT_PID"; ` + test.end},
				}, root)
				result <- err
			}()

			pid := awaitDescendantPID(t, pidFile)
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			cancel()
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("server without an initialize response was admitted")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("failed initialization did not join server cleanup")
			}
			for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
				if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatalf("language server descendant %d survived initialization cleanup", pid)
		})
	}
}

func awaitDescendantPID(t *testing.T, path string) int {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not publish its descendant PID")
	return 0
}
