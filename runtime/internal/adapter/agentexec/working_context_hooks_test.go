package agentexec

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
)

func TestWorkingContextLifecycleHooksPreserveResolutionFailure(t *testing.T) {
	resolveErr := errors.New("hook configuration unavailable")
	composer := NewWorkingContextComposer(WorkingContextConfig{
		Hooks: failingLifecycleHookResolver{err: resolveErr},
	})
	allowed, err := composer.BeforeCompaction(t.Context(), "session:one", "/workspace")
	if allowed || !errors.Is(err, resolveErr) {
		t.Fatalf("BeforeCompaction = %t, %v; want denied with resolution error", allowed, err)
	}
	if err := composer.NotifyWaiting(t.Context(), "session:one", "/workspace"); !errors.Is(err, resolveErr) {
		t.Fatalf("NotifyWaiting error = %v, want resolution error", err)
	}
	if err := composer.NotifyStopped(t.Context(), "session:one", "/workspace", "completed"); !errors.Is(err, resolveErr) {
		t.Fatalf("NotifyStopped error = %v, want resolution error", err)
	}
}

func TestInteractionExecutorReportsNotificationFailureWithoutChangingCompletion(t *testing.T) {
	var diagnostics bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	resolveErr := errors.New("terminal hook configuration unavailable")
	composer := NewWorkingContextComposer(WorkingContextConfig{
		Hooks: failingLifecycleHookResolver{err: resolveErr},
	})
	model := &observationScriptModel{
		responses: []*chat.Response{interactionUsageTextResponse("done", 2, 1)},
	}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{LifecycleHooks: composer})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted || ended[0].Failure() != nil {
		t.Fatalf("notification failure changed completion: %#v", ended)
	}
	if output := diagnostics.String(); !strings.Contains(output, resolveErr.Error()) ||
		!strings.Contains(output, "session.id=session_1") {
		t.Fatalf("notification failure was not reported: %s", output)
	}
}

type failingLifecycleHookResolver struct{ err error }

func (r failingLifecycleHookResolver) For(context.Context, string) (*apphooks.Bound, error) {
	return nil, r.err
}
