//go:build unix

package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestStdioLaunchResourceRetiresAfterTheProcessGroup(t *testing.T) {
	content := t.TempDir()
	marker := filepath.Join(content, "held")
	if err := os.WriteFile(marker, []byte("execution content"), 0600); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	config := ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName("server"), Transport: TransportStdio, Command: "logical-command"}
	var retired atomic.Int32
	var pid int
	input, err := NewLaunch(config, &Stdio{
		Command: os.Args[0],
		Dir:     content,
		Env:     withStdioProcessEnv(os.Environ(), map[string]string{stdioProcessRoleEnv: "server", stdioDescendantPID: pidFile}),
	}, func() error {
		retired.Add(1)
		var processErr error
		if pid != 0 && !waitForStdioProcessExit(pid, 2*time.Second) {
			processErr = errors.New("execution content retired before process teardown")
		}
		return errors.Join(processErr, os.Remove(marker))
	})
	if err != nil {
		t.Fatal(err)
	}
	c := &Connections{lifetime: t.Context(), client: newClient()}
	t.Cleanup(func() { _ = c.Shutdown(context.WithoutCancel(t.Context())) })
	if err := c.Configure(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	pid = waitForStdioDescendantPID(t, pidFile)
	if _, err := os.Stat(marker); err != nil || retired.Load() != 0 {
		t.Fatalf("successful connection lost execution content: %v", err)
	}
	if err := c.Detach(config.ID()); err != nil {
		t.Fatal(err)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if retired.Load() != 1 {
		t.Fatalf("retirement count: %d", retired.Load())
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("joined shutdown retained execution content: %v", err)
	}
}
