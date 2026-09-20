package terminal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
)

func TestResolveWorkspaceUsesTheCurrentRootForRelativePathsAndRejectsFiles(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveWorkspace(root, "nested")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(nested)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != expected {
		t.Fatalf("workspace = %s, want %s", resolved, expected)
	}
	file := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveWorkspace(root, file); err == nil {
		t.Fatal("regular file was accepted as a workspace")
	}
}

func TestWorkspaceChoicesUseTheFrozenCurrentWorkspace(t *testing.T) {
	now := time.Now()
	known := []workspace.Summary{
		{Workspace: workspace.Workspace{Path: "/work/first"}, Sessions: 2, LastActive: &now},
		{Workspace: workspace.Workspace{Path: "/work/current"}, Sessions: 1},
	}
	recent := []workbench.Workspace{{Path: "/work/recent", LastOpened: now.Add(time.Hour)}}

	choices := mergeWorkspaceChoices(known, recent, "/work/current")

	if len(choices) != 3 || !choices[0].current || choices[0].workspace.Path != "/work/current" {
		t.Fatalf("workspace choices = %+v, want frozen current workspace first", choices)
	}
	for _, choice := range choices[1:] {
		if choice.current {
			t.Fatalf("workspace %q was also marked current", choice.workspace.Path)
		}
	}
}

type workspaceRecordingRuntime struct {
	*runtimefixture.Runtime
	created chan string
}

func (w *workspaceRecordingRuntime) CreateSession(ctx context.Context, input agent.CreateSession) (agent.Session, error) {
	w.created <- input.Workspace
	return w.Runtime.CreateSession(ctx, input)
}

func TestRecentWorkspacePickerCreatesAndSwitchesToTheSelectedRoot(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "current")
	recent := filepath.Join(root, "recent-project")
	state := filepath.Join(root, "state")
	for _, directory := range []string{current, recent, state} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := openSessionWorkbench(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RememberWorkspace(recent); err != nil {
		t.Fatal(err)
	}
	backend := &workspaceRecordingRuntime{Runtime: runtimefixture.New(), created: make(chan string, 4)}
	backend.Instant = true
	host, stop := runUIWithState(t, backend, current, "", state)
	host.Shows(t, "Ask flame")
	if opened := <-backend.created; !samePath(opened, current) {
		t.Fatalf("opening workspace = %s, want %s", opened, current)
	}

	host.Type("/workspace")
	host.Press(input.Enter)
	host.Shows(t, "Workspaces")
	host.Type("recent-project")
	host.Shows(t, "recent-project")
	host.Press(input.Enter)
	if selected := <-backend.created; !samePath(selected, recent) {
		t.Fatalf("selected workspace = %s, want %s", selected, recent)
	}
	host.Shows(t, "session ·")
	stop()
}
