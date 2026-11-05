package pluginservices

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type workflowDefinitionRegistry struct {
	toolSourceRegistry
	read func(context.Context) error
}

func (r workflowDefinitionRegistry) Definition(ctx context.Context, _ mcpserver.ID) (mcpserver.Server, bool, error) {
	return mcpserver.Server{}, false, r.read(ctx)
}

func TestWorkflowSessionPreservesActivationReadOutcomes(t *testing.T) {
	fixture := newStreamFixture(t, "query")
	source := extension.ContributionSource{
		InstallationID: fixture.request.Installation.String(),
		Release:        fixture.request.Digest.String(),
		Contribution:   fixture.request.Contribution.String(),
	}
	storageFailure := errors.New("workflow installation read failed")
	withdrawal := errors.New("workflow request withdrawn")
	for _, test := range []struct {
		name        string
		failure     error
		cancel      bool
		unavailable bool
	}{
		{name: "storage", failure: storageFailure},
		{name: "cancellation", cancel: true},
		{name: "joined", failure: storageFailure, cancel: true},
		{name: "missing", unavailable: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			read := false
			registry := workflowDefinitionRegistry{read: func(ctx context.Context) error {
				read = true
				if test.cancel {
					cancel(withdrawal)
				}
				return errors.Join(test.failure, errors.Join(ctx.Err(), context.Cause(ctx)))
			}}
			pool, err := mcp.Open(t.Context(), t.Context(), nil, nil, registry, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := pool.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
			})
			calls := *fixture.calls
			calls.pool = pool
			_, borrow, err := workflowSession(ctx, &calls, fixture.request.Contract, source)
			if !read || borrow != nil || err == nil {
				t.Fatalf("activation read = %t, borrow = %v, error = %v", read, borrow, err)
			}
			if errors.Is(err, plugin.ErrUnavailable) != test.unavailable {
				t.Fatalf("availability classification = %v, want unavailable %t", err, test.unavailable)
			}
			if test.failure != nil && !errors.Is(err, test.failure) {
				t.Fatalf("activation lost storage failure: %v", err)
			}
			if test.cancel && (!errors.Is(err, context.Canceled) || !errors.Is(err, withdrawal)) {
				t.Fatalf("activation lost caller cancellation: %v", err)
			}
		})
	}
}
