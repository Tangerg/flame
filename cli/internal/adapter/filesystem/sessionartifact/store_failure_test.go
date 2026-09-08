//go:build darwin || linux

package sessionartifact

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/agent/session"
)

func TestPublishCleansStagingFileAfterWriteFailure(t *testing.T) {
	const workspaceEnv = "FLAME_TEST_EXPORT_WRITE_FAILURE_WORKSPACE"
	if workspace := os.Getenv(workspaceEnv); workspace != "" {
		// Keep the process-wide file limit and signal policy inside this child.
		signal.Ignore(syscall.SIGXFSZ)
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 1, Max: 1}); err != nil {
			t.Fatal(err)
		}
		document, err := session.NewDocument(protocol.ExportFormatMarkdown, []byte("# Session"))
		if err != nil {
			t.Fatal(err)
		}
		path, err := (Store{}).Publish(workspace, "Session", "session.md", document)
		if !errors.Is(err, syscall.EFBIG) || path != "" {
			t.Fatalf("Publish = %q, %v; want no path and file size limit error", path, err)
		}
		return
	}

	workspace := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.CommandContext(t.Context(), executable, "-test.run=^"+t.Name()+"$")
	child.Env = append(os.Environ(), workspaceEnv+"="+workspace)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("export with file size limit: %v\n%s", err, output)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed export left files in workspace: %v", entries)
	}
}
