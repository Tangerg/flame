package runtimebinding

import (
	"context"
	"encoding/base64"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type sessionBinding interface {
	RollbackSession(context.Context, protocol.RollbackSessionRequest, flameruntime.CommandOptions) (*protocol.RollbackSessionResponse, error)
	ExportSession(context.Context, protocol.ExportSessionRequest, flameruntime.CallOptions) (*protocol.ExportSessionResponse, error)
	ImportSession(context.Context, protocol.ImportSessionRequest, flameruntime.CommandOptions) (*protocol.ImportSessionResponse, error)
}

var _ session.TransferService = (*Connection)(nil)

func (r *Connection) RollbackSession(ctx context.Context, input conversation.RollbackSession) (conversation.RollbackResult, error) {
	if err := input.Validate(); err != nil {
		return conversation.RollbackResult{}, err
	}
	if input.RestoresFiles() {
		if err := r.requireFeature(protocol.FeatureCheckpoints); err != nil {
			return conversation.RollbackResult{}, err
		}
	}
	options, err := r.commandOptionsFor(input.CommandID)
	if err != nil {
		return conversation.RollbackResult{}, err
	}
	response, err := r.sessions.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID: input.SessionID, ToRunID: input.ToRunID, RestoreType: input.Scope,
	}, options)
	if err != nil {
		return conversation.RollbackResult{}, classifyError(err)
	}
	if response == nil || response.Session == nil {
		return conversation.RollbackResult{}, runtimeContractViolation("rollback session returned an incomplete result")
	}
	result := conversation.RollbackResult{
		Session: projectSession(*response.Session),
		Dropped: make([]conversation.DroppedRun, 0, len(response.DroppedRuns)),
	}
	if result.Session.ID != input.SessionID {
		return conversation.RollbackResult{}, runtimeContractViolation("rollback session returned session %q for %q", result.Session.ID, input.SessionID)
	}
	for _, dropped := range response.DroppedRuns {
		if dropped.Run.SessionID != input.SessionID {
			return conversation.RollbackResult{}, runtimeContractViolation(
				"rollback session %q returned dropped run %q from %q",
				input.SessionID, dropped.Run.ID, dropped.Run.SessionID,
			)
		}
		projected, err := projectDroppedRun(dropped)
		if err != nil {
			return conversation.RollbackResult{}, runtimeContractViolation("rollback session returned an invalid dropped run: %v", err)
		}
		result.Dropped = append(result.Dropped, projected)
	}
	return result, nil
}

func projectDroppedRun(value protocol.DroppedRun) (conversation.DroppedRun, error) {
	projected := conversation.DroppedRun{RunID: value.Run.ID, Input: make([]conversation.InputContent, 0, len(value.UserInput))}
	for index, content := range value.UserInput {
		switch content.Type {
		case protocol.ContentBlockText:
			projected.Input = append(projected.Input, conversation.InputContent{Kind: content.Type, Text: content.Text})
		case protocol.ContentBlockImage:
			data, err := base64.StdEncoding.DecodeString(content.Data)
			if err != nil {
				return conversation.DroppedRun{}, fmt.Errorf("rollback dropped run %s image %d: %w", value.Run.ID, index+1, err)
			}
			projected.Input = append(projected.Input, conversation.InputContent{Kind: content.Type, MimeType: content.Mime, Data: data})
		default:
			return conversation.DroppedRun{}, fmt.Errorf("rollback dropped run %s content %d has unsupported type %q", value.Run.ID, index+1, content.Type)
		}
	}
	return projected, nil
}

func (r *Connection) ExportSession(ctx context.Context, request session.ExportRequest) (session.Document, error) {
	if err := request.Validate(); err != nil {
		return session.Document{}, err
	}
	if err := r.requireFeature(protocol.FeatureSessionExport); err != nil {
		return session.Document{}, err
	}
	response, err := r.sessions.ExportSession(ctx, protocol.ExportSessionRequest{
		SessionID: request.SessionID, Format: request.Format,
	}, r.callOptions())
	if err != nil {
		return session.Document{}, classifyError(err)
	}
	if response == nil {
		return session.Document{}, runtimeContractViolation("export session returned nil")
	}
	if request.Format != response.Format {
		return session.Document{}, runtimeContractViolation("export session returned format %q, want %q", response.Format, request.Format)
	}
	var body []byte
	switch request.Format {
	case protocol.ExportFormatMarkdown:
		if response.Artifact != nil || response.Markdown == "" {
			return session.Document{}, runtimeContractViolation("export session returned a malformed Markdown result")
		}
		body = []byte(response.Markdown)
	case protocol.ExportFormatJSON:
		if response.Artifact == nil || response.Markdown != "" {
			return session.Document{}, runtimeContractViolation("export session returned a malformed JSON result")
		}
		if response.Artifact.Session.ID != request.SessionID {
			return session.Document{}, runtimeContractViolation(
				"export session returned artifact for %q, want %q",
				response.Artifact.Session.ID, request.SessionID,
			)
		}
		body, err = json.Marshal(response.Artifact, jsontext.WithIndent("  "), json.Deterministic(true))
		if err != nil {
			return session.Document{}, runtimeContractViolation("export session artifact cannot be encoded: %v", err)
		}
	}
	document, err := session.NewDocument(request.Format, body)
	if err != nil {
		return session.Document{}, runtimeContractViolation("export session returned an invalid document: %v", err)
	}
	return document, nil
}

func (r *Connection) ImportSession(ctx context.Context, request session.ImportRequest) (conversation.Session, error) {
	if err := request.Validate(); err != nil {
		return conversation.Session{}, err
	}
	if err := r.requireFeature(protocol.FeatureSessionExport); err != nil {
		return conversation.Session{}, err
	}
	// An artifact is user-supplied text: duplicate members, unknown members and
	// a trailing second document all have to fail before it becomes a Session.
	var artifact protocol.SessionArtifact
	if err := json.Unmarshal(
		request.Artifact.Bytes(), &artifact, json.RejectUnknownMembers(true),
	); err != nil {
		return conversation.Session{}, fmt.Errorf("import session: decode artifact: %w", err)
	}
	if err := protocol.ValidateWireTree(artifact); err != nil {
		return conversation.Session{}, fmt.Errorf("import session: %w", err)
	}
	options := r.commandOptions()
	response, err := r.sessions.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: artifact}, options)
	if err != nil {
		return conversation.Session{}, classifyError(err)
	}
	if response == nil || response.Session == nil {
		return conversation.Session{}, runtimeContractViolation("import session returned an incomplete result")
	}
	return projectSessionResult("import session", artifact.Session.ID, response.Session, nil)
}
