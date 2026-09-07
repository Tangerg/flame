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

type Summary struct {
	Workspace  protocol.WorkspaceInfo
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
	if err := protocol.ValidateWireTree(s.Workspace); err != nil {
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
