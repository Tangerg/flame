//go:build darwin

package exec

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
)

// TestLaunchSandboxConfinesWrites proves the opt-in jail is actually wired onto
// the live shell: a sandboxed Launch confines a command's writes to its cwd.
func TestLaunchSandboxConfinesWrites(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")

	confiner, err := sandbox.NewConfiner(t.TempDir(), nil)
	shells, err := NewShells(confiner, err, true)
	if err != nil {
		t.Fatalf("new shells: %v", err)
	}
	t.Cleanup(func() { _ = shells.KillAll() })

	run := func(command string) Output {
		id, launchErr := shells.Launch(t.Context(), "s1", workspace, command, testTimeout(t, 10*time.Second), false)
		if launchErr != nil {
			t.Fatalf("launch: %v", launchErr)
		}
		sh, ok := shells.Get("s1", id)
		if !ok {
			t.Fatal("launched shell vanished")
		}
		<-sh.Done()
		output, readErr := shells.Read("s1", id)
		if readErr != nil || output.Exit == nil {
			t.Fatalf("read finished shell: %+v (%v)", output, readErr)
		}
		return output
	}

	if output := run("printf inside > inside.txt"); output.Exit.Code != 0 {
		t.Fatalf("inside write exited %d: %q", output.Exit.Code, output.Text)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "inside.txt"))
	if err != nil || string(content) != "inside" {
		t.Fatalf("inside write = (%q, %v)", content, err)
	}

	if output := run("printf outside > " + strconv.Quote(outside)); output.Exit.Code == 0 {
		t.Fatal("write outside the workspace unexpectedly succeeded")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside write created a file: %v", err)
	}
}
