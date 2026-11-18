package mcp

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func testCoordinator(t testing.TB, cfg Config) *Coordinator {
	t.Helper()
	ports := &fakePorts{}
	if cfg.External == nil {
		cfg.External = &testExternalWorkflow{attempts: newAuthorizationAttemptStore()}
	}
	if cfg.Registry == nil {
		cfg.Registry = &testRegistry{servers: make(map[mcpserver.ID]mcpserver.Server)}
	}
	if cfg.Store == nil {
		store, isStore := cfg.Registry.(store)
		if !isStore {
			store = &testRegistry{servers: make(map[mcpserver.ID]mcpserver.Server)}
		}
		cfg.Store = store
	}
	if cfg.Connections == nil {
		cfg.Connections = ports
	}
	if cfg.ToolCatalog == nil {
		cfg.ToolCatalog = ports
	}
	if cfg.Exposure == nil {
		cfg.Exposure = NewExposureState(nil, nil)
	}
	coordinator, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { requireCoordinatorShutdown(t, coordinator) })
	return coordinator
}

type testExternalWorkflow struct{ attempts *authorizationAttemptStore }

func (*testExternalWorkflow) Affected(context.Context, []mcpserver.ID) (bool, error) {
	return false, nil
}
func (*testExternalWorkflow) List(context.Context) ([]mcpserver.Server, error) { return nil, nil }
func (*testExternalWorkflow) AdmitProbe(context.Context) (func(), error)       { return func() {}, nil }
func (w *testExternalWorkflow) StartAuthorization(_ context.Context, s mcpserver.Server, _ AuthorizationAttemptID) (AuthorizationAttempt, func(context.Context, AuthorizationAttemptStatus) error, error) {
	a := w.attempts.create(s.ID())
	return a, func(_ context.Context, status AuthorizationAttemptStatus) error {
		w.attempts.settle(a.ID, status)
		return nil
	}, nil
}
func (w *testExternalWorkflow) ReadAuthorization(_ context.Context, id AuthorizationAttemptID) (AuthorizationAttempt, error) {
	a, ok := w.attempts.get(id)
	if !ok {
		return a, ErrAuthorizationAttemptNotFound
	}
	return a, nil
}

func (*testExternalWorkflow) Invalidate() {}
