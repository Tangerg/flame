package delivery

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func workspaceRefPath(ref *protocol.WorkspaceRef) string {
	if ref == nil {
		return ""
	}
	return ref.Path
}

func (s *Handler) sessionWorkspacePath(ref *protocol.WorkspaceRef) string {
	if ref == nil {
		return s.serverInfo.DefaultWorkspace.Path
	}
	return ref.Path
}

func workspaceRefFromPath(path string) *protocol.WorkspaceRef {
	if path == "" {
		return nil
	}
	return &protocol.WorkspaceRef{Path: path}
}

func wireWorkspaceError(err error) error {
	if wired, ok := workspaceFailure(err); ok {
		return wired
	}
	return err
}

func workspaceFailure(err error) (error, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, workspaceapp.ErrPromptSourceTooLarge):
		return protocol.NewFailure(protocol.ErrPromptSourceTooLarge, "AGENTS.md instructions exceed the supported document or complete guidance budget; shorten the source documents before retrying", err), true
	case errors.Is(err, workspaceapp.ErrCWDUnavailable):
		return protocol.NewFailure(protocol.ErrWorkspaceUnavailable, err.Error(), err), true
	case errors.Is(err, workspaceapp.ErrPathOutsideRoot):
		return protocol.ErrPathOutsideRoot, true
	case errors.Is(err, workspaceapp.ErrUnsupportedFile):
		return protocol.NewFailure(protocol.ErrUnsupportedMime, err.Error(), err), true
	case errors.Is(err, tool.ErrInvalidArguments),
		errors.Is(err, workspaceapp.ErrPathRequired),
		errors.Is(err, workspaceapp.ErrWatchLimit),
		errors.Is(err, workspaceapp.ErrInvalidFileRange),
		errors.Is(err, workspaceapp.ErrFileReadTooLarge),
		errors.Is(err, workspaceapp.ErrInvalidFileListPath),
		errors.Is(err, workspaceapp.ErrInvalidFileGlob),
		errors.Is(err, workspaceapp.ErrInvalidFileNameQuery),
		errors.Is(err, workspaceapp.ErrGrepQueryMissing),
		errors.Is(err, workspaceapp.ErrInvalidGrepQuery),
		errors.Is(err, workspaceapp.ErrInvalidGrepLimit),
		errors.Is(err, workspaceapp.ErrGrepResultTooLarge),
		errors.Is(err, workspaceapp.ErrInvalidFileReadLimit),
		errors.Is(err, workspaceapp.ErrFileListTooLarge),
		errors.Is(err, workspaceapp.ErrPageLimit),
		errors.Is(err, workspaceapp.ErrPageCursor),
		errors.Is(err, workspaceapp.ErrVCSResultTooLarge),
		errors.Is(err, workspaceapp.ErrVCSBaseUnknown):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err), true
	case errors.Is(err, workspaceapp.ErrVCSUnavailable):
		return protocol.ErrVcsUnavailable, true
	default:
		return nil, false
	}
}
