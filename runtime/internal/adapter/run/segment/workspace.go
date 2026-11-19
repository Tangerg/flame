package segment

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

// FileChangePublisher nudges live workspace subscribers after a tool-owned file
// mutation. It is deliberately path-only so persistence does not acquire event
// presentation responsibilities.
type FileChangePublisher func(workspaceapp.FileChangeNotice)

// WorkspaceNotifier adapts the live publisher to the Run application's narrow
// notification port. It owns no durable state and never joins a transaction.
type WorkspaceNotifier struct {
	publish FileChangePublisher
}

var _ runs.WorkspaceChangeNotifier = WorkspaceNotifier{}

func NewWorkspaceNotifier(publish FileChangePublisher) (WorkspaceNotifier, error) {
	if publish == nil {
		return WorkspaceNotifier{}, errors.New("segment: file change publisher is required")
	}
	return WorkspaceNotifier{publish: publish}, nil
}

func (w WorkspaceNotifier) Nudge(cwd string, paths []string) {
	if len(paths) > 0 {
		w.publish(workspaceapp.FileChangeNotice{CWD: cwd, Paths: slices.Clone(paths)})
	}
}
