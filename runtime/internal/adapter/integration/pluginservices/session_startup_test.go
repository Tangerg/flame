package pluginservices

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func TestContributionSessionJoinsStartupWithoutOwningItsCancellation(t *testing.T) {
	for _, failure := range []error{nil, errors.New("backend launch refused")} {
		name := "ready"
		if failure != nil {
			name = "failed"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newStreamFixture(t, "query")
			contract := fixture.request.Contract
			selection, err := plugin.NewProviderSelection(plugin.SelectionRecord{
				Role: contract.Descriptor().Reference, Installation: fixture.request.Installation, Contribution: fixture.request.Contribution,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := fixture.host.ProviderSelections.Save(t.Context(), selection); err != nil {
				t.Fatal(err)
			}
			managed := fixture.managed
			managed.close(t)
			entered, allowed := make(chan struct{}), make(chan struct{})
			allow := sync.OnceFunc(func() { close(allowed) })
			t.Cleanup(allow)
			announce := sync.OnceFunc(func() { close(entered) })
			managed.packages.beforeLaunch = func(ctx context.Context) error {
				announce()
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-allowed:
					return failure
				}
			}
			managed.open(t, false)
			<-entered
			cause := errors.New("consumer no longer needs the backend")
			ctx, cancel := context.WithTimeoutCause(t.Context(), time.Second, cause)
			defer cancel()
			_, borrowed, err := contributionSession(ctx, managed.Host, fixture.store, managed.Stores.ProviderSelections, managed.Pool, contract)
			if borrowed != nil {
				defer borrowed.Close()
			}
			if borrowed != nil || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, cause) || errors.Is(err, plugin.ErrUnavailable) {
				t.Fatalf("pending startup was treated as unavailable: borrow=%v error=%v statuses=%+v", borrowed, err, managed.Pool.Statuses())
			}
			statuses := managed.Pool.Statuses()
			if len(statuses) != 1 || statuses[0].State != mcpserver.ConnectionConnecting {
				t.Fatalf("consumer cancellation retired the startup owner: %+v", statuses)
			}
			allow()
			binding, borrowed, err := contributionSession(t.Context(), managed.Host, fixture.store, managed.Stores.ProviderSelections, managed.Pool, contract)
			if borrowed != nil {
				defer borrowed.Close()
			}
			if failure != nil {
				if borrowed != nil || !errors.Is(err, plugin.ErrUnavailable) {
					t.Fatalf("failed startup supplied a session: %v, %v", borrowed, err)
				}
			} else {
				if err != nil {
					t.Fatalf("the original startup was not joined: %v", err)
				}
				activation, err := borrowed.Activation()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := plugins.OriginalActivation([]plugin.Activation{activation}, binding.InstallationID, binding.Digest, binding.ServerName); err != nil {
					t.Fatal(err)
				}
				if _, err := pluginmcp.ResolveCapability(borrowed.Context(), borrowed.Session(), binding.Export, contract); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
