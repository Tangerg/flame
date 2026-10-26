package testsupport

import (
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// MustRestoreSession fills irrelevant lifecycle defaults, restores snapshot, and
// panics when the requested fixture is invalid.
func MustRestoreSession(snapshot session.Snapshot) session.Session {
	if snapshot.Workspace.Path() == "" {
		snapshot.Workspace = MustWorkspace("/fixture")
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Unix(1, 0).UTC()
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = snapshot.CreatedAt
	}
	if snapshot.Revision == 0 {
		snapshot.Revision = 1
	}
	if !snapshot.Selection.Configured() {
		snapshot.Selection = fixtureSelection()
	}
	value, err := session.Restore(snapshot)
	if err != nil {
		panic(err)
	}
	return value
}

// MustWorkspace constructs a valid exact workspace value for test fixtures.
func MustWorkspace(path string) session.Workspace {
	workspace, err := session.NewWorkspace(path)
	if err != nil {
		panic(err)
	}
	return workspace
}

func fixtureSelection() modelchoice.Selection {
	selection, err := modelchoice.New("test-provider", "test-model")
	if err != nil {
		panic(err)
	}
	return selection
}
