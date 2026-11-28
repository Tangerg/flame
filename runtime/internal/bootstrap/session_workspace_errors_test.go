package bootstrap

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestSessionEntryPointsReportUnavailableWorkspace(t *testing.T) {
	r := poMCPOpen(t, poMCPConfig(t))
	ctx := protocolLifecycleContext(t.Context())
	workspace := protocol.WorkspaceRef{Path: "relative-workspace"}
	opening := &protocol.RunSessionOpening{
		SessionID: "ses_relative_workspace", RunID: "run_relative_workspace", Workspace: &workspace,
	}
	for _, request := range []struct {
		method delivery.Name
		input  any
	}{
		{delivery.SessionsCreate, protocol.CreateSessionRequest{Workspace: &workspace}},
		{delivery.RunsStart, protocol.StartRunRequest{NewSession: opening, Input: []protocol.ContentBlock{
			{Type: protocol.ContentBlockText, Text: "Open in the requested workspace"},
		}}},
	} {
		result := r.endpoint.Invoke(ctx, request.method, request.input, protocol.CallMetadata{IdempotencyKey: request.method.String()})
		if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrWorkspaceUnavailable) {
			t.Errorf("%s with a relative workspace = %+v, %v; want workspace unavailable", request.method, result.Value, result.Failure)
		}
	}
	if value, err := r.api.GetSession(ctx, opening.SessionID); !errors.Is(err, protocol.ErrSessionNotFound) {
		t.Fatalf("refused opening left a Session: %+v, %v", value, err)
	}
	result := r.endpoint.Invoke(ctx, delivery.WorkspacesResolve, protocol.ResolveWorkspaceRequest{Ref: &workspace}, protocol.CallMetadata{})
	if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrWorkspaceUnavailable) {
		t.Errorf("workspaces.resolve with a relative workspace = %+v, %v; want workspace unavailable", result.Value, result.Failure)
	}
	missing := protocol.WorkspaceRef{Path: filepath.Join(t.TempDir(), "missing")}
	resolved := r.must(delivery.WorkspacesResolve, protocol.ResolveWorkspaceRequest{Ref: &missing}, "").(*protocol.WorkspaceInfo)
	if resolved.Availability != protocol.WorkspaceMissing {
		t.Errorf("workspaces.resolve with a missing absolute workspace = %+v; want missing", resolved)
	}
}
