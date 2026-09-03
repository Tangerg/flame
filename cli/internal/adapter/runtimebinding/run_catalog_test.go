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

type runCatalogBindingStub struct {
	get  func(context.Context, protocol.GetRunRequest, flameruntime.CallOptions) (*protocol.RunRef, error)
	list func(context.Context, protocol.ListRunsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error)
}

func (r runCatalogBindingStub) GetRun(ctx context.Context, request protocol.GetRunRequest, options flameruntime.CallOptions) (*protocol.RunRef, error) {
	return r.get(ctx, request, options)
}

func (r runCatalogBindingStub) ListRuns(ctx context.Context, request protocol.ListRunsRequest, options flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error) {
	return r.list(ctx, request, options)
}

func TestRunCatalogMapsQueriesAndProjectsPages(t *testing.T) {
	t.Parallel()
	wantRun := protocol.RunRef{
		RunSummary: protocol.RunSummary{
			ID: "run_1", SessionID: "ses_1", Provider: "deepseek", Model: "deepseek-chat",
			CreatedAt: time.Unix(1, 0).UTC(), FinishedAt: time.Unix(2, 0).UTC(),
			Status: protocol.RunStatusFinished, Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted},
		},
		ProtocolProfile: protocol.RunProtocolProfile{RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{}},
	}
	stub := runCatalogBindingStub{
		get: func(_ context.Context, request protocol.GetRunRequest, options flameruntime.CallOptions) (*protocol.RunRef, error) {
			if request.RunID != wantRun.ID || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
				t.Fatalf("get = (%+v, %+v)", request, options)
			}
			return &wantRun, nil
		},
		list: func(_ context.Context, request protocol.ListRunsRequest, options flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error) {
			if request.SessionID != "ses_1" || len(request.Statuses) != 1 || request.Statuses[0] != protocol.RunStatusFinished ||
				!request.IncludeDescendants || request.Cursor != "opaque" || request.Limit == nil || *request.Limit != agent.MaximumPageRows ||
				options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
				t.Fatalf("list = (%+v, %+v)", request, options)
			}
			return protocol.NewPageWithCursor([]protocol.RunRef{wantRun}, "next"), nil
		},
	}
	runtime := &Connection{
		runCatalog: stub, meta: requestMeta("test"),
		profile: Profile{Features: map[string]Feature{
			protocol.FeatureSubagents: {
				Enabled: true, ClientOptIn: true, ClientRequested: true,
			},
		}},
	}
	got, err := runtime.GetRun(t.Context(), "run_1")
	if err != nil || got.ID != "run_1" || got.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("GetRun = %+v, %v", got, err)
	}
	page, err := runtime.ListRuns(t.Context(), agent.RunQuery{
		SessionID: "ses_1", Statuses: []protocol.RunStatus{protocol.RunStatusFinished},
		IncludeDescendants: true, Cursor: "opaque", PageSize: agent.MaximumPageSize(),
	})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "run_1" || page.NextCursor != "next" {
		t.Fatalf("ListRuns = %+v, %v", page, err)
	}
}

func TestRunCatalogPublishesTheCLIPageDefaultAsPositiveWireIntent(t *testing.T) {
	t.Parallel()
	runtime := &Connection{runCatalog: runCatalogBindingStub{list: func(
		_ context.Context,
		request protocol.ListRunsRequest,
		_ flameruntime.CallOptions,
	) (*protocol.Page[protocol.RunRef], error) {
		if request.Limit == nil || *request.Limit != agent.DefaultPageRows {
			t.Fatalf("default run page limit = %v, want %d", request.Limit, agent.DefaultPageRows)
		}
		return protocol.NewPage([]protocol.RunRef{}), nil
	}}}

	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{PageSize: agent.DefaultPageSize()}); err != nil {
		t.Fatal(err)
	}
}

func TestRunCatalogRejectsOversizedCursorsAtTheAdapterBoundary(t *testing.T) {
	t.Parallel()
	oversized := strings.Repeat("x", maximumPaginationCursorBytes+1)
	called := false
	runtime := &Connection{runCatalog: runCatalogBindingStub{list: func(
		context.Context,
		protocol.ListRunsRequest,
		flameruntime.CallOptions,
	) (*protocol.Page[protocol.RunRef], error) {
		called = true
		return protocol.NewPage([]protocol.RunRef{}), nil
	}}}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{
		PageSize: agent.DefaultPageSize(), Cursor: oversized,
	}); err == nil || !strings.Contains(err.Error(), "transport limit") {
		t.Fatalf("oversized request cursor error = %v", err)
	}
	if called {
		t.Fatal("oversized request cursor reached the Runtime binding")
	}

	_, err := projectRunPage(protocol.NewPageWithCursor([]protocol.RunRef{}, oversized), agent.RunQuery{}, agent.DefaultPageRows)
	if err == nil || !strings.Contains(err.Error(), "continuation cursor larger") {
		t.Fatalf("oversized response cursor error = %v", err)
	}
	requireRuntimeContractViolation(t, err)
}

func TestRunCatalogRejectsDescendantQueryWithoutNegotiatedSubagents(t *testing.T) {
	t.Parallel()
	called := false
	runtime := &Connection{runCatalog: runCatalogBindingStub{
		list: func(context.Context, protocol.ListRunsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error) {
			called = true
			return protocol.NewPage([]protocol.RunRef{}), nil
		},
	}}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{
		IncludeDescendants: true, PageSize: agent.DefaultPageSize(),
	}); err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("ListRuns error = %v, want ErrIncompatibleRuntime", err)
	}
	if called {
		t.Fatal("descendant query reached the binding without negotiated subagents")
	}
}

func TestRunCatalogRejectsIncompleteBindingResults(t *testing.T) {
	t.Parallel()
	stub := runCatalogBindingStub{
		get: func(context.Context, protocol.GetRunRequest, flameruntime.CallOptions) (*protocol.RunRef, error) {
			return nil, nil
		},
		list: func(context.Context, protocol.ListRunsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error) {
			return nil, nil
		},
	}
	runtime := &Connection{runCatalog: stub, meta: requestMeta("test")}
	if _, err := runtime.GetRun(t.Context(), "run_1"); err == nil {
		t.Fatal("GetRun accepted nil response")
	} else {
		requireRuntimeContractViolation(t, err)
	}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{PageSize: agent.DefaultPageSize()}); err == nil {
		t.Fatal("ListRuns accepted nil response")
	} else {
		requireRuntimeContractViolation(t, err)
	}
	if _, err := runtime.GetRun(t.Context(), " "); err == nil {
		t.Fatal("GetRun accepted empty id")
	}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{
		PageSize: agent.DefaultPageSize(), Statuses: []protocol.RunStatus{"paused"},
	}); err == nil {
		t.Fatal("ListRuns accepted invalid status")
	}

	failing := runCatalogBindingStub{
		get: func(context.Context, protocol.GetRunRequest, flameruntime.CallOptions) (*protocol.RunRef, error) {
			return nil, protocol.ErrRunNotFound
		},
		list: func(context.Context, protocol.ListRunsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error) {
			return nil, protocol.ErrSessionNotFound
		},
	}
	runtime.runCatalog = failing
	if _, err := runtime.GetRun(t.Context(), "missing"); !errors.Is(err, agent.ErrRunNotFound) {
		t.Fatalf("GetRun error = %v", err)
	}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{
		SessionID: "missing", PageSize: agent.DefaultPageSize(),
	}); !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("ListRuns error = %v", err)
	}
}

func TestRunCatalogRejectsResponsesOutsideTheRequestedScope(t *testing.T) {
	t.Parallel()
	base := protocol.RunRef{
		RunSummary: protocol.RunSummary{ID: "run_1", SessionID: "ses_other", Status: protocol.RunStatusFinished,
			Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted}},
		ProtocolProfile: protocol.RunProtocolProfile{RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{}},
	}
	wrongIdentity := base
	wrongIdentity.ID = "run_other"
	runtime := &Connection{runCatalog: runCatalogBindingStub{get: func(context.Context, protocol.GetRunRequest, flameruntime.CallOptions) (*protocol.RunRef, error) {
		return &wrongIdentity, nil
	}}, meta: requestMeta("test")}
	_, err := runtime.GetRun(t.Context(), "run_1")
	requireRuntimeContractViolation(t, err)

	for _, test := range []struct {
		name  string
		query agent.RunQuery
		value protocol.RunRef
	}{
		{name: "session", query: agent.RunQuery{SessionID: "ses_1", PageSize: agent.DefaultPageSize()}, value: base},
		{name: "status", query: agent.RunQuery{PageSize: agent.DefaultPageSize(), Statuses: []protocol.RunStatus{protocol.RunStatusRunning}}, value: base},
		{name: "descendant", query: agent.RunQuery{
			PageSize: agent.DefaultPageSize(),
		}, value: func() protocol.RunRef {
			value := base
			value.SessionID = "ses_1"
			value.SpawnedByItemID, value.ParentRunID, value.RootRunID = "item_1", "run_root", "run_root"
			return value
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := runCatalogBindingStub{list: func(context.Context, protocol.ListRunsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error) {
				return protocol.NewPage([]protocol.RunRef{test.value}), nil
			}}
			runtime := &Connection{runCatalog: stub, meta: requestMeta("test")}
			_, err := runtime.ListRuns(t.Context(), test.query)
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestRunCatalogRejectsPagesOutsideRuntimeOrder(t *testing.T) {
	t.Parallel()
	finished := func(id string, created time.Time) protocol.RunRef {
		return protocol.RunRef{
			RunSummary: protocol.RunSummary{
				ID: id, SessionID: "ses_1", Provider: "deepseek", Model: "deepseek-chat",
				CreatedAt: created, FinishedAt: created.Add(time.Second),
				Status: protocol.RunStatusFinished, Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted},
			},
			ProtocolProfile: protocol.RunProtocolProfile{
				RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{},
			},
		}
	}
	created := time.Unix(10, 0).UTC()
	for _, test := range []struct {
		name string
		runs []protocol.RunRef
	}{
		{
			name: "creation time ascends",
			runs: []protocol.RunRef{finished("run_old", created), finished("run_new", created.Add(time.Second))},
		},
		{
			name: "equal-time identity ascends",
			runs: []protocol.RunRef{finished("run_a", created), finished("run_b", created)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := projectRunPage(protocol.NewPage(test.runs), agent.RunQuery{}, agent.DefaultPageRows)
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestRunCatalogOmitsAnEmptyStatusFilter(t *testing.T) {
	t.Parallel()
	stub := runCatalogBindingStub{
		get: func(context.Context, protocol.GetRunRequest, flameruntime.CallOptions) (*protocol.RunRef, error) {
			return nil, errors.New("unexpected get")
		},
		list: func(_ context.Context, request protocol.ListRunsRequest, _ flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error) {
			if request.Statuses != nil {
				t.Fatalf("statuses = %#v, want absent", request.Statuses)
			}
			if err := request.ValidateWire(); err != nil {
				t.Fatalf("wire query = %v", err)
			}
			return protocol.NewPage([]protocol.RunRef{}), nil
		},
	}
	runtime := &Connection{runCatalog: stub, meta: requestMeta("test")}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{
		PageSize: agent.DefaultPageSize(), Statuses: []protocol.RunStatus{},
	}); err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
}
