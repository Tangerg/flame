package delivery

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ExportSession serializes a session to a portable artifact.
// format=json (default) produces a round-trippable SessionArtifact —
// session identity + chat history + canonical items + runs + portable offloaded tool
// bodies — that ImportSession restores. format=md produces a human-readable
// transcript (not re-importable).
// Portable JSON uses the same encoded document budget on import and export.
func (s *Handler) ExportSession(ctx context.Context, request protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
	format := request.Format
	if format == "" {
		format = protocol.ExportFormatJSON
	}

	if format != protocol.ExportFormatJSON && format != protocol.ExportFormatMarkdown {
		return nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unsupported export format %q", format))
	}
	results := sessions.ReferencedToolResults
	if format == protocol.ExportFormatMarkdown {
		if s.pluginCalls == nil {
			return nil, protocol.NewFailure(protocol.ErrPluginUnavailable, "no session Markdown renderer is available")
		}
		results = sessions.ExpandedToolResults
	}
	result, err := s.sessions.ExportSession(ctx, request.SessionID, results)
	if err != nil {
		if errors.Is(err, rundomain.ErrSessionBusy) {
			return nil, protocol.NewFailure(protocol.ErrSessionBusy, fmt.Sprintf("session %q has a run in flight or open interrupt", request.SessionID), err)
		}
		return nil, wireSessionErr(err)
	}
	artifact, err := artifactFromPortable(result)
	if err != nil {
		return nil, fmt.Errorf("sessions.export: encode artifact: %w", err)
	}

	if format == protocol.ExportFormatMarkdown {
		// Ordinary text is already expanded in its Item. Model-only bodies may
		// contain evidence absent from the structured result and must survive.
		artifact.ToolResults = slices.DeleteFunc(artifact.ToolResults, func(body protocol.ArtifactToolResult) bool {
			return !body.ModelOnly
		})
		document, err := json.Marshal(artifact)
		if err != nil {
			return nil, err
		}
		if len(document) > protocol.MaximumSessionArtifactBytes {
			return nil, protocol.ErrExportTooLarge
		}
		rendered, err := s.pluginCalls.RenderSessionMarkdown(ctx, artifact.Session.ID, document)
		if err != nil {
			return nil, wirePluginError(err)
		}
		return &protocol.ExportSessionResponse{Format: format, Markdown: rendered}, nil
	}
	if err := validatePortableSize(artifact); err != nil {
		return nil, err
	}
	return &protocol.ExportSessionResponse{
		Format: format, Artifact: &artifact,
	}, nil
}

// ImportSession recreates a Session from a SessionArtifact under its original
// identity. Re-importing the same artifact is idempotent; importing over an
// existing Session restores it atomically.
func (s *Handler) ImportSession(ctx context.Context, request protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
	artifact := request.Artifact
	if artifact.Version != protocol.SessionArtifactVersion {
		return nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unsupported artifact version %d (want %d)", artifact.Version, protocol.SessionArtifactVersion))
	}
	if err := validatePortableSize(artifact); err != nil {
		return nil, err
	}
	portable, err := portableArtifactFromWire(artifact)
	if err != nil {
		return nil, err
	}
	sessionID := artifact.Session.ID

	// Hand the strictly decoded portable archive to the session use case.
	// It commits the whole thing as ONE transaction — upsert the
	// session row, replace existing history (drop old items/runs/tool bodies + clear the
	// chat log + stale open interrupts), re-seed the messages, re-persist
	// runs+items+tool bodies — so a mid-sequence failure after the destructive
	// delete/truncate can't leave the session row live but its history
	// half-destroyed (an import-over losing the prior history with nothing to
	// replace it).
	view, err := s.sessions.RestorePortableSession(ctx, portable)
	if err != nil {
		if errors.Is(err, rundomain.ErrSessionBusy) {
			return nil, protocol.NewFailure(protocol.ErrSessionBusy, fmt.Sprintf("session %q has a run in flight or open interrupt", sessionID), err)
		}
		if errors.Is(err, sessions.ErrInvalidPortableSnapshot) {
			return nil, protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
		}
		if errors.Is(err, transcript.ErrIdentityConflict) || errors.Is(err, transcript.ErrToolResultIdentityConflict) {
			return nil, protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
		}
		return nil, wireSessionErr(err)
	}

	presented := presentSession(view)
	return &protocol.ImportSessionResponse{Session: &presented}, nil
}

func validatePortableSize(artifact protocol.SessionArtifact) error {
	err := json.MarshalWrite(&encodedSizeLimit{remaining: protocol.MaximumSessionArtifactBytes}, artifact)
	if errors.Is(err, sessions.ErrExportTooLarge) {
		return protocol.NewFailure(protocol.ErrExportTooLarge, fmt.Sprintf("session archive exceeds %d bytes", protocol.MaximumSessionArtifactBytes))
	}
	return err
}
