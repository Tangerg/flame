package runtimebinding

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
)

func configureIntegrationRuntime(t *testing.T) {
	t.Helper()
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_MODEL", "claude-opus-5")
	t.Setenv("FLAME_APIKEY", "test-key")
}

func openIntegrationRuntime(t *testing.T, workspace string) *Connection {
	t.Helper()
	productRoot := runtimefixture.TempDirectory(t)
	owner := NewOwner(Config{
		ProductRoot: productRoot, DefaultWorkspacePath: workspace,
		UserHomePath: runtimefixture.TempDirectory(t), ConfigDirectories: []string{runtimefixture.TempDirectory(t)}, ClientVersion: "test",
	})
	runtime, err := owner.Connection(t.Context(), settings.Runtime{Mode: settings.RuntimeEmbedded})
	if err != nil {
		t.Fatalf("open Runtime: %v", err)
	}
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Errorf("close Runtime: %v", err)
			const shutdownDiagnosticBytes = 1 << 20
			stack := make([]byte, shutdownDiagnosticBytes)
			t.Logf("runtime close goroutines:\n%s", stack[:goruntime.Stack(stack, true)])
		}
		if err := filepath.WalkDir(productRoot, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		}); err != nil {
			t.Errorf("restore fixture directory permissions: %v", err)
		}
	})
	return runtime
}

func testStartRequest(sessionID string, message authoring.Message) authoring.StartRun {
	return authoring.StartRun{
		SessionID: sessionID, Message: message,
		Options: authoring.RunOptions{},
	}
}
