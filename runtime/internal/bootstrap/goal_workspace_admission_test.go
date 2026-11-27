package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/scope/core/chatclient"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestWaitingGoalFollowsCommittedWorkspaceEdit(t *testing.T) {
	observer := &admissionWaitObserver{waiting: make(chan struct{}, 1)}
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(observer))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tracer)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		if err := tracer.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})

	model := newLifecycleModel()
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.modelProviders = testChatResolver(client)
	runtime := poMCPOpen(t, cfg)
	poMCPActivate(runtime, filepath.Join(firstPartyPackages, "goals"))
	ctx, stop := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
	defer stop()
	session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "Move a waiting Goal"})
	if err != nil {
		t.Fatal(err)
	}
	release, acquired, err := runtime.host.application.admissions.AcquireWorkingTreeMutation(session.Workspace.Ref.Path)
	if err != nil || !acquired {
		t.Fatalf("hold the original working tree: acquired=%t, %v", acquired, err)
	}
	defer release()
	started, err := runtime.api.StartGoal(ctx, protocol.StartGoalRequest{
		SessionID: session.ID, Objective: "Run in the Session's current workspace.",
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-observer.waiting:
	case <-ctx.Done():
		t.Fatalf("Goal did not reach the blocked working tree: %v", ctx.Err())
	}

	workspace := t.TempDir()
	updated, err := runtime.api.UpdateSession(ctx, protocol.UpdateSessionRequest{
		SessionID: session.ID, ExpectedRevision: session.Revision,
		Workspace: &protocol.WorkspaceRef{Path: workspace},
	})
	if err != nil || updated.Workspace.Ref.Path == session.Workspace.Ref.Path {
		t.Fatalf("move Session away from its blocked tree: %+v, %v", updated, err)
	}
	select {
	case <-model.firstCallStarted:
	case <-ctx.Done():
		t.Fatalf("Goal stayed blocked after committing the new workspace: %v", ctx.Err())
	}
	if err := runtime.api.StopGoal(ctx, started.GoalTarget); err != nil {
		t.Fatal(err)
	}
}

type admissionWaitObserver struct{ waiting chan struct{} }

func (o *admissionWaitObserver) OnStart(_ context.Context, span sdktrace.ReadWriteSpan) {
	if span.Name() == "wait for session admission" {
		select {
		case o.waiting <- struct{}{}:
		default:
		}
	}
}

func (*admissionWaitObserver) OnEnd(sdktrace.ReadOnlySpan)      {}
func (*admissionWaitObserver) Shutdown(context.Context) error   { return nil }
func (*admissionWaitObserver) ForceFlush(context.Context) error { return nil }
