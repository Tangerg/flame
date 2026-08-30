package agentexec

import (
	"context"
	"testing"

	agent "github.com/Tangerg/scope/agent"
)

func TestInteractionSessionSubtreeCancellationIsScopedAndCoversLateDescendants(t *testing.T) {
	rootID := mustInteractionProcessID(t, "root")
	targetID := mustInteractionProcessID(t, "target")
	descendantID := mustInteractionProcessID(t, "descendant")
	siblingID := mustInteractionProcessID(t, "sibling")

	targetCtx, cancelTarget := context.WithCancelCause(t.Context())
	descendantCtx, cancelDescendant := context.WithCancelCause(t.Context())
	siblingCtx, cancelSibling := context.WithCancelCause(t.Context())
	rootCtx, cancelRoot := context.WithCancelCause(t.Context())
	t.Cleanup(func() {
		cancelTarget(nil)
		cancelDescendant(nil)
		cancelSibling(nil)
		cancelRoot(nil)
	})

	session := &interactionSession{
		state: interactionState{
			delegateChildren: map[agent.ProcessID]*managedDelegateCall{
				targetID:     {identity: delegateCallIdentity{parentID: rootID}},
				descendantID: {identity: delegateCallIdentity{parentID: targetID}},
				siblingID:    {identity: delegateCallIdentity{parentID: rootID}},
			},
			activeDispatches: map[interactionDispatchIdentity]activeInteractionDispatch{
				{processID: rootID, effectID: mustInteractionEffectID(t, "root")}:             {processID: rootID, cancel: cancelRoot},
				{processID: targetID, effectID: mustInteractionEffectID(t, "target")}:         {processID: targetID, cancel: cancelTarget},
				{processID: descendantID, effectID: mustInteractionEffectID(t, "descendant")}: {processID: descendantID, cancel: cancelDescendant},
				{processID: siblingID, effectID: mustInteractionEffectID(t, "sibling")}:       {processID: siblingID, cancel: cancelSibling},
			},
			canceledSubtreeRoots: make(map[agent.ProcessID]struct{}),
		},
	}

	session.cancelSubtreeDispatches(targetID)
	assertInteractionDispatchCanceled(t, targetCtx, "target")
	assertInteractionDispatchCanceled(t, descendantCtx, "descendant")
	assertInteractionDispatchRunning(t, rootCtx, "root")
	assertInteractionDispatchRunning(t, siblingCtx, "sibling")

	lateDescendantID := mustInteractionProcessID(t, "late-descendant")
	session.state.mu.Lock()
	session.state.delegateChildren[lateDescendantID] = &managedDelegateCall{
		identity: delegateCallIdentity{parentID: descendantID},
	}
	canceled := session.inCanceledSubtreeLocked(lateDescendantID)
	session.state.mu.Unlock()
	if !canceled {
		t.Fatal("late descendant did not inherit its ancestor's cancellation scope")
	}
}

func mustInteractionProcessID(t *testing.T, value string) agent.ProcessID {
	t.Helper()
	id, err := agent.ParseProcessID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustInteractionEffectID(t *testing.T, value string) agent.EffectID {
	t.Helper()
	id, err := agent.ParseEffectID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertInteractionDispatchCanceled(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	select {
	case <-ctx.Done():
		if cause := context.Cause(ctx); cause != errInteractionRunCanceled {
			t.Fatalf("%s cancellation cause = %v", name, cause)
		}
	default:
		t.Fatalf("%s dispatch remains active", name)
	}
}

func assertInteractionDispatchRunning(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatalf("%s dispatch was canceled: %v", name, context.Cause(ctx))
	default:
	}
}
