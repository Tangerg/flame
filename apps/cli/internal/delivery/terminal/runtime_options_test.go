package terminal

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/core/input"
)

type delayedModelCatalog struct {
	Runtime
	calls                                atomic.Uint64
	started, canceled, release, returned chan struct{}
}

func (r *delayedModelCatalog) ListModels(ctx context.Context) ([]protocol.Model, error) {
	label := "Initial model"
	switch r.calls.Add(1) {
	case 2:
		close(r.started)
		select {
		case <-ctx.Done():
			close(r.canceled)
			<-r.release
		case <-r.release:
		}
		close(r.returned)
		label = "Retired model"
	case 3:
		label = "Reopened model"
	case 4:
		label = "Current model"
	}
	return []protocol.Model{{ID: "model", Provider: "mock", DisplayName: label}}, nil
}

func TestModelPickerDismissalRetiresItsCatalogRead(t *testing.T) {
	for _, reopen := range []bool{false, true} {
		t.Run(map[bool]string{false: "dismiss", true: "reopen"}[reopen], func(t *testing.T) {
			runtime := &delayedModelCatalog{Runtime: runtimefixture.New(), started: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{}), returned: make(chan struct{})}
			source := runtimeResourceChangeSource(protocol.TopicModelsChanged)
			host, stop := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtime, Changes: source})
			release := sync.OnceFunc(func() { close(runtime.release) })
			t.Cleanup(release)
			host.Shows(t, "Ask flame")
			assertSingleRuntimeTopic(t, source.subscription, protocol.TopicModelsChanged)
			host.Type("/model")
			host.Press(input.Enter)
			host.Shows(t, "Initial model")
			source.events <- changefeed.Event{Type: protocol.RuntimeModelsChanged, Sequence: 1}
			awaitValue(t, source.applied, "model invalidation")
			awaitValue(t, runtime.started, "model refresh")
			host.Press(input.Esc)
			host.Hides(t, "Models")
			awaitValue(t, runtime.canceled, "dismissed model read cancellation")
			if reopen {
				host.Type("/model")
				host.Press(input.Enter)
				host.Shows(t, "Reopened model")
			}
			release()
			awaitValue(t, runtime.returned, "retired model result")
			if reopen {
				source.events <- changefeed.Event{Type: protocol.RuntimeModelsChanged, Sequence: 2}
				awaitValue(t, source.applied, "current model invalidation")
				host.Shows(t, "Current model")
				host.Hides(t, "Retired model")
				host.Press(input.Esc)
				host.Hides(t, "Models")
			}
			host.Type("draft after dismissal")
			host.Shows(t, "draft after dismissal")
			host.Hides(t, "Models")
			stop()
		})
	}
}

// The picker describes a mode from the gates the Runtime publishes, so a policy
// the CLI has never seen reads exactly as published.
func TestApprovalPolicyDetailFollowsThePublishedGates(t *testing.T) {
	policy := protocol.ApprovalModePolicy{
		Mode: protocol.ApprovalModeBalanced, Write: protocol.ApprovalGatePass,
		Exec: protocol.ApprovalGateDeny, Network: protocol.ApprovalGatePrompt,
	}
	want := "write allowed · exec refused · network asks first"
	if got := approvalPolicyDetail(policy); got != want {
		t.Fatalf("detail = %q, want %q", got, want)
	}
}
