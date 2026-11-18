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

	run := func(command string) *Shell {
		id, launchErr := shells.Launch(t.Context(), "s1", workspace, command, testTimeout(t, 10*time.Second), false)
		if launchErr != nil {
			t.Fatalf("launch: %v", launchErr)
		}
		sh, ok := shells.Get("s1", id)
		if !ok {
			t.Fatal("launched shell vanished")
		}
		<-sh.Done()
		return sh
	}

	sh := run("printf inside > inside.txt")
	if code, _, _, _ := sh.Outcome(); code != 0 {
		out, _, readErr := sh.read()
		t.Fatalf("inside write exited %d: %q (%v)", code, out.Text, readErr)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "inside.txt"))
	if err != nil || string(content) != "inside" {
		t.Fatalf("inside write = (%q, %v)", content, err)
	}

	sh = run("printf outside > " + strconv.Quote(outside))
	if code, _, _, _ := sh.Outcome(); code == 0 {
		t.Fatal("write outside the workspace unexpectedly succeeded")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside write created a file: %v", err)
	}
}
