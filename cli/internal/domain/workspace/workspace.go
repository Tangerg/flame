package workspace

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
)

// ErrVersionControlUnavailable means the workspace has no version-control
// projection. It is distinct from an empty change set and from an adapter
// failure.
var ErrVersionControlUnavailable = errors.New("version control unavailable")

type Workspace struct {
	Path         string
	ProjectRoot  string
	Availability protocol.WorkspaceAvailability
}

func (w Workspace) IsAvailable() bool { return w.Availability == protocol.WorkspaceAvailable }

type Summary struct {
	Workspace  Workspace
	Name       string
	Sessions   int
	LastActive *time.Time
}

func (s Summary) Clone() Summary {
	if s.LastActive != nil {
		s.LastActive = new(*s.LastActive)
	}
	return s
}

type ResolveRequest struct {
	Path string
}

func (r ResolveRequest) Validate() error {
	if r.Path != "" && !filepath.IsAbs(r.Path) {
		return errors.New("workspace resolve path is not absolute")
	}
	return nil
}
