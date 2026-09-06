package runtimebinding

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type sessionCatalogStub struct {
	pages  map[string]*protocol.Page[protocol.Session]
	list   func(protocol.ListSessionsRequest) (*protocol.Page[protocol.Session], error)
	create func(protocol.CreateSessionRequest) (*protocol.Session, error)
	update func(protocol.UpdateSessionRequest) (*protocol.Session, error)
	fork   func(protocol.ForkSessionRequest) (*protocol.Session, error)
	delete func(protocol.DeleteSessionRequest, flameruntime.CommandOptions) error
}

func testProtocolWorkspace(path, projectRoot string, availability protocol.WorkspaceAvailability) protocol.WorkspaceInfo {
	return protocol.WorkspaceInfo{
		Ref: protocol.WorkspaceRef{Path: path}, ProjectRoot: projectRoot, Availability: availability,
	}
}

const (
	testSessionProvider = "mock"
	testSessionModel    = "balanced"
)

var testSessionTime = time.Unix(1, 0).UTC()

func (s sessionCatalogStub) ListSessions(_ context.Context, query protocol.ListSessionsRequest, _ flameruntime.CallOptions) (*protocol.Page[protocol.Session], error) {
	if s.list != nil {
		return s.list(query)
	}
	return s.pages[query.Cursor], nil
}

func TestSessionCatalogPublishesTheCLIPageDefaultAsPositiveWireIntent(t *testing.T) {
	t.Parallel()
	runtime := &Connection{sessionCatalog: sessionCatalogStub{list: func(query protocol.ListSessionsRequest) (*protocol.Page[protocol.Session], error) {
		if query.Limit == nil || *query.Limit != agent.DefaultPageRows {
			t.Fatalf("default session page limit = %v, want %d", query.Limit, agent.DefaultPageRows)
		}
		return protocol.NewPage([]protocol.Session{}), nil
	}}}

	if _, err := runtime.ListSessions(t.Context(), agent.SessionQuery{PageSize: agent.DefaultPageSize()}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCatalogRejectsOversizedCursorsAtTheAdapterBoundary(t *testing.T) {
	t.Parallel()
	oversized := strings.Repeat("x", maximumPaginationCursorBytes+1)
	called := false
	runtime := &Connection{sessionCatalog: sessionCatalogStub{list: func(protocol.ListSessionsRequest) (*protocol.Page[protocol.Session], error) {
		called = true
		return protocol.NewPage([]protocol.Session{}), nil
	}}}
	if _, err := runtime.ListSessions(t.Context(), agent.SessionQuery{
		PageSize: agent.DefaultPageSize(), Cursor: oversized,
	}); err == nil || !strings.Contains(err.Error(), "transport limit") {
		t.Fatalf("oversized request cursor error = %v", err)
	}
	if called {
		t.Fatal("oversized request cursor reached the Runtime binding")
	}

	_, err := projectSessionPage(
		protocol.NewPageWithCursor([]protocol.Session{}, oversized), agent.SessionQuery{}, agent.DefaultPageRows,
	)
	if err == nil || !strings.Contains(err.Error(), "continuation cursor larger") {
		t.Fatalf("oversized response cursor error = %v", err)
	}
	requireRuntimeContractViolation(t, err)
}

func TestSessionCatalogRejectsPagesOutsideRuntimeOrder(t *testing.T) {
	t.Parallel()
	session := func(id string, updated time.Time, favorite bool) protocol.Session {
		return protocol.Session{
			ID: id, Status: protocol.SessionStatusIdle,
			Provider: testSessionProvider, Model: testSessionModel,
			Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable),
			CreatedAt: testSessionTime, UpdatedAt: updated, Favorite: favorite, Revision: 1,
		}
	}
	updated := testSessionTime.Add(time.Hour)
	for _, test := range []struct {
		name     string
		sessions []protocol.Session
	}{
		{
			name: "favorite follows ordinary",
			sessions: []protocol.Session{
				session("ses_ordinary", updated, false), session("ses_favorite", updated, true),
			},
		},
		{
			name: "update time ascends",
			sessions: []protocol.Session{
				session("ses_old", updated, false), session("ses_new", updated.Add(time.Second), false),
			},
		},
		{
			name: "equal-time identity ascends",
			sessions: []protocol.Session{
				session("ses_a", updated, false), session("ses_b", updated, false),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := projectSessionPage(protocol.NewPage(test.sessions), agent.SessionQuery{}, agent.DefaultPageRows)
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestSessionCatalogRejectsPagesOutsideWorkspaceFilter(t *testing.T) {
	t.Parallel()
	runtime := &Connection{sessionCatalog: sessionCatalogStub{list: func(protocol.ListSessionsRequest) (*protocol.Page[protocol.Session], error) {
		return protocol.NewPage([]protocol.Session{{
			ID: "ses_other", Status: protocol.SessionStatusIdle,
			Provider: testSessionProvider, Model: testSessionModel,
			Workspace: testProtocolWorkspace("/other", "/other", protocol.WorkspaceAvailable),
			CreatedAt: testSessionTime, UpdatedAt: testSessionTime, Revision: 1,
		}}), nil
	}}, meta: requestMeta("test")}

	_, err := runtime.ListSessions(t.Context(), agent.SessionQuery{
		Workspace: "/workspace", PageSize: agent.DefaultPageSize(),
	})
	requireRuntimeContractViolation(t, err)
}

func TestSessionCatalogRejectsPagesOutsideSearchFilter(t *testing.T) {
	t.Parallel()
	runtime := &Connection{sessionCatalog: sessionCatalogStub{list: func(protocol.ListSessionsRequest) (*protocol.Page[protocol.Session], error) {
		return protocol.NewPage([]protocol.Session{{
			ID: "ses_other", Title: "unrelated", Status: protocol.SessionStatusIdle,
			Provider: testSessionProvider, Model: testSessionModel,
			Workspace: testProtocolWorkspace("/other", "/other", protocol.WorkspaceAvailable),
			CreatedAt: testSessionTime, UpdatedAt: testSessionTime, Revision: 1,
		}}), nil
	}}, meta: requestMeta("test")}

	_, err := runtime.ListSessions(t.Context(), agent.SessionQuery{
		Search: "release", PageSize: agent.DefaultPageSize(),
	})
	requireRuntimeContractViolation(t, err)
}

func (s sessionCatalogStub) CreateSession(_ context.Context, request protocol.CreateSessionRequest, _ flameruntime.CommandOptions) (*protocol.Session, error) {
	if s.create != nil {
		return s.create(request)
	}
	return nil, errors.New("unexpected CreateSession")
}

func (s sessionCatalogStub) UpdateSession(_ context.Context, request protocol.UpdateSessionRequest, _ flameruntime.CommandOptions) (*protocol.Session, error) {
	if s.update != nil {
		return s.update(request)
	}
	return nil, errors.New("unexpected UpdateSession")
}

func (s sessionCatalogStub) ForkSession(_ context.Context, request protocol.ForkSessionRequest, _ flameruntime.CommandOptions) (*protocol.Session, error) {
	if s.fork != nil {
		return s.fork(request)
	}
	return nil, errors.New("unexpected ForkSession")
}

func TestCreateAndForkSessionRejectAcknowledgementDrift(t *testing.T) {
	t.Parallel()
	base := protocol.Session{
		ID: "ses_new", Title: "Requested", Status: protocol.SessionStatusIdle,
		Provider: testSessionProvider, Model: testSessionModel,
		Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable),
		CreatedAt: testSessionTime, UpdatedAt: testSessionTime, Revision: 1,
	}
	tests := []struct {
		name    string
		invoke  func(*Connection) error
		binding sessionCatalogStub
	}{
		{
			name: "create title",
			binding: sessionCatalogStub{create: func(protocol.CreateSessionRequest) (*protocol.Session, error) {
				result := base
				result.Title = "Ignored"
				return &result, nil
			}},
			invoke: func(runtime *Connection) error {
				_, err := runtime.CreateSession(t.Context(), agent.CreateSession{Title: base.Title, Workspace: "/workspace"})
				return err
			},
		},
		{
			name: "create workspace",
			binding: sessionCatalogStub{create: func(protocol.CreateSessionRequest) (*protocol.Session, error) {
				result := base
				result.Workspace = testProtocolWorkspace("/other", "/other", protocol.WorkspaceAvailable)
				return &result, nil
			}},
			invoke: func(runtime *Connection) error {
				_, err := runtime.CreateSession(t.Context(), agent.CreateSession{Title: base.Title, Workspace: "/workspace"})
				return err
			},
		},
		{
			name: "fork title",
			binding: sessionCatalogStub{fork: func(protocol.ForkSessionRequest) (*protocol.Session, error) {
				result := base
				result.Title = "Ignored"
				return &result, nil
			}},
			invoke: func(runtime *Connection) error {
				_, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: "ses_source", Title: base.Title})
				return err
			},
		},
		{
			name: "fork source identity",
			binding: sessionCatalogStub{fork: func(request protocol.ForkSessionRequest) (*protocol.Session, error) {
				result := base
				result.ID = request.SessionID
				return &result, nil
			}},
			invoke: func(runtime *Connection) error {
				_, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: "ses_source", Title: base.Title})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := &Connection{
				sessionCatalog: test.binding,
				workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
					Ref:          protocol.WorkspaceRef{Path: "/workspace"},
					ProjectRoot:  "/workspace",
					Availability: protocol.WorkspaceAvailable,
				}},
				meta: requestMeta("test"),
			}
			requireRuntimeContractViolation(t, test.invoke(runtime))
		})
	}
}

func TestUpdateSessionProjectsEveryWritableField(t *testing.T) {
	workspace, title, favorite := "/workspace/new", "Renamed", true
	model := agent.ModelRef{Provider: "deepseek", Model: "deep"}
	stub := sessionCatalogStub{update: func(request protocol.UpdateSessionRequest) (*protocol.Session, error) {
		if request.SessionID != "ses_1" || request.ExpectedRevision != 7 || request.Title == nil || *request.Title != title ||
			request.Workspace == nil || request.Workspace.Path != workspace || request.Provider == nil || *request.Provider != model.Provider ||
			request.Model == nil || *request.Model != model.Model ||
			request.Favorite == nil || *request.Favorite != favorite {
			t.Fatalf("update request = %+v", request)
		}
		return &protocol.Session{
			ID: request.SessionID, Title: title, Status: protocol.SessionStatusIdle, Provider: model.Provider, Model: model.Model,
			Workspace: testProtocolWorkspace(request.Workspace.Path, "/workspace", protocol.WorkspaceAvailable),
			CreatedAt: testSessionTime, UpdatedAt: testSessionTime,
			Favorite: favorite, Revision: 8,
		}, nil
	}}
	runtime := &Connection{
		sessionCatalog: stub,
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref:          protocol.WorkspaceRef{Path: workspace},
			ProjectRoot:  "/workspace",
			Availability: protocol.WorkspaceAvailable,
		}},
		meta: requestMeta("test"),
		profile: profileWithFeatures(t, map[string]protocol.FeatureCapability{
			protocol.FeatureRelocate: {Enabled: true},
		}),
	}
	updated, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: "ses_1", Title: &title, Workspace: &workspace, Model: &model,
		Favorite: &favorite, ExpectedRevision: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Workspace.Path != workspace || updated.Workspace.ProjectRoot != "/workspace" || !updated.Workspace.IsAvailable() ||
		updated.Provider != model.Provider || updated.Model != model.Model || !updated.Favorite || updated.Revision != 8 {
		t.Fatalf("updated session = %+v", updated)
	}
}

func TestUpdateSessionRejectsWorkspaceWithoutRelocateCapability(t *testing.T) {
	t.Parallel()
	called := false
	runtime := &Connection{sessionCatalog: sessionCatalogStub{update: func(protocol.UpdateSessionRequest) (*protocol.Session, error) {
		called = true
		return nil, nil
	}}}
	workspace := "/workspace/new"
	if _, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: "ses_1", Workspace: &workspace, ExpectedRevision: 7,
	}); err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("UpdateSession error = %v, want ErrIncompatibleRuntime", err)
	}
	if called {
		t.Fatal("workspace update reached the binding without relocate capability")
	}
}

func TestUpdateSessionRejectsAcknowledgementsThatDidNotApplyTheMutation(t *testing.T) {
	t.Parallel()
	workspace, title, favorite := "/workspace/new", "Renamed", true
	model := agent.ModelRef{Provider: "deepseek", Model: "deep"}
	request := agent.UpdateSession{
		SessionID: "ses_1", Title: &title, Workspace: &workspace, Model: &model,
		Favorite: &favorite, ExpectedRevision: 7,
	}
	valid := protocol.Session{
		ID: request.SessionID, Title: title, Status: protocol.SessionStatusIdle, Provider: model.Provider, Model: model.Model,
		Workspace: testProtocolWorkspace(workspace, "/workspace", protocol.WorkspaceAvailable),
		CreatedAt: testSessionTime, UpdatedAt: testSessionTime,
		Favorite: favorite, Revision: 8,
	}
	tests := []struct {
		name   string
		mutate func(*protocol.Session)
	}{
		{name: "stale revision", mutate: func(session *protocol.Session) { session.Revision = 7 }},
		{name: "title", mutate: func(session *protocol.Session) { session.Title = "Old" }},
		{name: "workspace", mutate: func(session *protocol.Session) { session.Workspace.Ref.Path = "/workspace/old" }},
		{name: "model", mutate: func(session *protocol.Session) { session.Model = "shallow" }},
		{name: "favorite", mutate: func(session *protocol.Session) { session.Favorite = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := valid
			test.mutate(&result)
			runtime := &Connection{
				sessionCatalog: sessionCatalogStub{update: func(protocol.UpdateSessionRequest) (*protocol.Session, error) {
					return &result, nil
				}},
				workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
					Ref:          protocol.WorkspaceRef{Path: workspace},
					ProjectRoot:  workspace,
					Availability: protocol.WorkspaceAvailable,
				}},
				meta: requestMeta("test"),
				profile: profileWithFeatures(t, map[string]protocol.FeatureCapability{
					protocol.FeatureRelocate: {Enabled: true},
				}),
			}
			_, err := runtime.UpdateSession(t.Context(), request)
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestSessionMutationsUseResolvedWorkspaceIdentity(t *testing.T) {
	t.Parallel()
	const requested = "/workspace/alias"
	const canonical = "/workspace/canonical"
	resolved := &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
		Ref:          protocol.WorkspaceRef{Path: canonical},
		ProjectRoot:  canonical,
		Availability: protocol.WorkspaceAvailable,
	}}
	result := protocol.Session{
		ID: "ses_1", Title: "Requested", Status: protocol.SessionStatusIdle,
		Provider: testSessionProvider, Model: testSessionModel,
		Workspace: testProtocolWorkspace(canonical, canonical, protocol.WorkspaceAvailable),
		CreatedAt: testSessionTime, UpdatedAt: testSessionTime, Revision: 1,
	}
	catalog := sessionCatalogStub{
		create: func(request protocol.CreateSessionRequest) (*protocol.Session, error) {
			if request.Workspace == nil || request.Workspace.Path != canonical {
				t.Fatalf("create workspace = %+v, want %q", request.Workspace, canonical)
			}
			created := result
			return &created, nil
		},
		update: func(request protocol.UpdateSessionRequest) (*protocol.Session, error) {
			if request.Workspace == nil || request.Workspace.Path != canonical {
				t.Fatalf("update workspace = %+v, want %q", request.Workspace, canonical)
			}
			updated := result
			updated.Revision = 2
			return &updated, nil
		},
	}
	runtime := &Connection{
		sessionCatalog: catalog, workspaces: resolved, meta: requestMeta("test"),
		profile: profileWithFeatures(t, map[string]protocol.FeatureCapability{
			protocol.FeatureRelocate: {Enabled: true},
		}),
	}
	if _, err := runtime.CreateSession(t.Context(), agent.CreateSession{
		Title: result.Title, Workspace: requested,
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	requestedWorkspace := requested
	if _, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: result.ID, Workspace: &requestedWorkspace, ExpectedRevision: 1,
	}); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
}

func TestProjectSessionPreservesResolvedWorkspaceIdentity(t *testing.T) {
	t.Parallel()

	projected, err := projectSession(protocol.Session{
		ID: "ses_1", Status: protocol.SessionStatusIdle,
		Provider: testSessionProvider, Model: testSessionModel, ReasoningEffort: "high",
		Workspace: testProtocolWorkspace("/repo/work", "/repo", protocol.WorkspaceMissing),
		CreatedAt: testSessionTime, UpdatedAt: testSessionTime, Revision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projected.ReasoningEffort != "high" || projected.Workspace.Path != "/repo/work" || projected.Workspace.ProjectRoot != "/repo" ||
		projected.Workspace.IsAvailable() {
		t.Fatalf("workspace = %+v", projected.Workspace)
	}
}

func TestProjectSessionRejectsIncompleteWorkspaceIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workspace protocol.WorkspaceInfo
	}{
		{name: "project root", workspace: testProtocolWorkspace("/workspace", "", protocol.WorkspaceAvailable)},
		{name: "availability", workspace: testProtocolWorkspace("/workspace", "/workspace", "")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := projectSession(protocol.Session{
				ID: "ses_1", Status: protocol.SessionStatusIdle,
				Provider: testSessionProvider, Model: testSessionModel, Workspace: test.workspace,
				CreatedAt: testSessionTime, UpdatedAt: testSessionTime, Revision: 1,
			})
			if err == nil {
				t.Fatalf("projectSession accepted %+v", test.workspace)
			}
		})
	}
}

func TestProjectSessionRejectsMissingLifecycleTimes(t *testing.T) {
	t.Parallel()

	valid := protocol.Session{
		ID: "ses_1", Status: protocol.SessionStatusIdle,
		Provider: testSessionProvider, Model: testSessionModel,
		Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable),
		CreatedAt: testSessionTime, UpdatedAt: testSessionTime, Revision: 1,
	}
	for _, test := range []struct {
		name  string
		field string
		clear func(*protocol.Session)
	}{
		{name: "creation", field: "createdAt", clear: func(value *protocol.Session) { value.CreatedAt = time.Time{} }},
		{name: "update", field: "updatedAt", clear: func(value *protocol.Session) { value.UpdatedAt = time.Time{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := valid
			test.clear(&value)
			_, err := projectSession(value)
			if err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("projectSession error = %v, want %q", err, test.field)
			}
		})
	}
}

func (s sessionCatalogStub) DeleteSession(_ context.Context, request protocol.DeleteSessionRequest, options flameruntime.CommandOptions) error {
	if s.delete != nil {
		return s.delete(request, options)
	}
	return errors.New("unexpected DeleteSession")
}

func TestDeleteSessionUsesTheDurableMutationIdentity(t *testing.T) {
	t.Parallel()
	commandID := agent.CommandID("cli_11111111111111111111111111111111")
	const namespace = compatibleReplayNamespace
	called := false
	runtime := &Connection{sessionCatalog: sessionCatalogStub{delete: func(request protocol.DeleteSessionRequest, options flameruntime.CommandOptions) error {
		called = true
		if request.SessionID != "ses_1" || options.IdempotencyKey != string(commandID) ||
			options.IdempotencyNamespace != namespace {
			t.Fatalf("delete request = %+v, options = %+v", request, options)
		}
		return nil
	}}, meta: requestMeta("test"), profile: profileWithReplayNamespace(t, namespace)}
	if err := runtime.DeleteSession(t.Context(), agent.DeleteSession{CommandID: commandID, SessionID: "ses_1"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("delete session did not reach the runtime binding")
	}
}

func TestSessionCatalogProjectsFiltersWithoutClientSideCursorScanning(t *testing.T) {
	t.Parallel()
	calls := 0
	runtime := &Connection{sessionCatalog: sessionCatalogStub{list: func(request protocol.ListSessionsRequest) (*protocol.Page[protocol.Session], error) {
		calls++
		if request.Search != "Needle" || request.Workspace == nil || request.Workspace.Path != "/workspace" ||
			request.Cursor != "current" || request.Limit == nil || *request.Limit != agent.DefaultPageRows {
			t.Fatalf("filtered sessions request = %+v", request)
		}
		return protocol.NewPageWithCursor([]protocol.Session{}, "next"), nil
	}}, meta: requestMeta("test")}

	page, err := runtime.ListSessions(t.Context(), agent.SessionQuery{
		PageSize: agent.DefaultPageSize(), Search: "  Needle  ", Workspace: "/workspace", Cursor: "current",
	})
	if err != nil || page.NextCursor != "next" || calls != 1 {
		t.Fatalf("ListSessions = (%+v, %v), calls=%d", page, err, calls)
	}
}

func TestSessionCatalogRejectsAStalledCursorAndMutationIdentity(t *testing.T) {
	t.Parallel()
	runtime := &Connection{sessionCatalog: sessionCatalogStub{
		pages: map[string]*protocol.Page[protocol.Session]{
			"stalled": protocol.NewPageWithCursor([]protocol.Session{}, "stalled"),
		},
		update: func(protocol.UpdateSessionRequest) (*protocol.Session, error) {
			return &protocol.Session{
				ID: "ses_other", Status: protocol.SessionStatusIdle,
				Provider: testSessionProvider, Model: testSessionModel,
				Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable),
				CreatedAt: testSessionTime, UpdatedAt: testSessionTime, Revision: 2,
			}, nil
		},
	}, meta: requestMeta("test")}

	_, err := runtime.ListSessions(t.Context(), agent.SessionQuery{Cursor: "stalled", PageSize: agent.DefaultPageSize()})
	requireRuntimeContractViolation(t, err)
	title := "Renamed"
	_, err = runtime.UpdateSession(t.Context(), agent.UpdateSession{SessionID: "ses_1", Title: &title, ExpectedRevision: 1})
	requireRuntimeContractViolation(t, err)
}

func TestSessionCatalogRejectsInvalidLocalFiltersBeforeCallingRuntime(t *testing.T) {
	t.Parallel()

	runtime := &Connection{sessionCatalog: sessionCatalogStub{}, meta: requestMeta("test")}
	for _, query := range []agent.SessionQuery{
		{PageSize: agent.DefaultPageSize(), Workspace: "relative/workspace"},
		{PageSize: agent.DefaultPageSize(), Search: strings.Repeat("x", 1025)},
	} {
		if _, err := runtime.ListSessions(t.Context(), query); err == nil {
			t.Fatalf("ListSessions accepted %+v", query)
		}
	}
}
