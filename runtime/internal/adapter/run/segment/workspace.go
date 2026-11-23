package segment

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

type FileChangePublisher func(workspaceapp.FileChangeNotice)

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
