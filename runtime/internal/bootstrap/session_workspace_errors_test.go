package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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

func TestSessionWorkspaceBindingRemainsStableAcrossDirectoryAliases(t *testing.T) {
	r := poMCPOpen(t, poMCPConfig(t))
	firstDirectory := t.TempDir()
	secondDirectory := t.TempDir()
	create := func(path, key string) *protocol.Session {
		t.Helper()
		return r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{
			Workspace: &protocol.WorkspaceRef{Path: path},
		}, key).(*protocol.Session)
	}
	first := create(firstDirectory, "first-workspace-binding")
	second := create(secondDirectory, "second-workspace-binding")
	if err := os.Rename(firstDirectory, filepath.Join(t.TempDir(), "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secondDirectory, firstDirectory); err != nil {
		t.Fatal(err)
	}

	read := r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: first.ID}, "").(*protocol.Session)
	if read.Workspace.Ref != first.Workspace.Ref || read.Workspace.RepositoryRoot != second.Workspace.RepositoryRoot {
		t.Errorf("stored workspace now reads as %+v; want ref %+v and live root %q", read.Workspace, first.Workspace.Ref, second.Workspace.RepositoryRoot)
	}
	filtered := r.must(delivery.SessionsList, protocol.ListSessionsRequest{Workspace: &read.Workspace.Ref}, "").(*protocol.Page[protocol.Session])
	if len(filtered.Data) != 1 || filtered.Data[0].ID != first.ID || filtered.Data[0].Workspace.Ref != first.Workspace.Ref {
		t.Errorf("filtering by the returned workspace = %+v; want only %s", filtered.Data, first.ID)
	}
	catalog, err := r.api.ListWorkspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Data) != 2 {
		t.Errorf("workspace catalog joined two stored paths: %+v", catalog.Data)
	}
	for _, ref := range []protocol.WorkspaceRef{first.Workspace.Ref, second.Workspace.Ref} {
		if !slices.ContainsFunc(catalog.Data, func(summary protocol.WorkspaceSummary) bool {
			return summary.Workspace.Ref == ref && summary.SessionCount == 1
		}) {
			t.Errorf("workspace catalog = %+v; want %q with one session", catalog.Data, ref.Path)
		}
	}
	resolved := r.must(delivery.WorkspacesResolve, protocol.ResolveWorkspaceRequest{Ref: &first.Workspace.Ref}, "").(*protocol.WorkspaceInfo)
	if resolved.Ref != second.Workspace.Ref {
		t.Errorf("resolving the same path as a new selection = %+v; want %+v", resolved.Ref, second.Workspace.Ref)
	}
}
