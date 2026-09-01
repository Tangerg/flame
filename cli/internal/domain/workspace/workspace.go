package workspace

import (
	"errors"
	"path/filepath"
	"strings"
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

func (w Workspace) Validate() error {
	switch {
	case strings.TrimSpace(w.Path) == "":
		return errors.New("workspace path is empty")
	case !filepath.IsAbs(w.Path):
		return errors.New("workspace path is not absolute")
	case strings.TrimSpace(w.ProjectRoot) == "":
		return errors.New("workspace project root is empty")
	case !filepath.IsAbs(w.ProjectRoot):
		return errors.New("workspace project root is not absolute")
	default:
		return (protocol.WorkspaceInfo{Availability: w.Availability}).ValidateWire()
	}
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

func (s Summary) Validate() error {
	if err := s.Workspace.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("workspace summary name is empty")
	}
	if s.Sessions < 0 {
		return errors.New("workspace session count is negative")
	}
	return nil
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
