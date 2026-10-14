package pluginservices

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

func TestAuxiliaryCaptureHoldsOriginalObserversAcrossCallsUntilRelease(t *testing.T) {
	observer, bindings, _ := installedObserverFixture(t, "outer-failure")
	var calls, releases atomic.Int32
	model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		invocation, err := pluginsdk.InvocationFromContext(ctx)
		if err != nil || invocation.Target.Scope != "call" {
			t.Errorf("provider lost concrete model call: %+v, %v", invocation, err)
		}
		calls.Add(1)
		message := chat.NewAssistantMessage(chat.NewTextPart("known response"))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
	})
	resolve := observer.Auxiliary(func(context.Context) (chat.Model, func(), error) {
		return model, func() { releases.Add(1) }, nil
	})
	captured, release, err := resolve(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	if held := observer.HeldDependencies(); len(held) != 1 || held[0].InstallationID != bindings[0].InstallationID || held[0].Digest != bindings[0].Digest {
		t.Fatalf("captured release not held: %+v", held)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
	if err != nil {
		t.Fatal(err)
	}
	calling := func(id string) context.Context {
		t.Helper()
		ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: id, Origin: capability.Automatic, Parent: "original-maintenance", Target: capability.Target{Scope: "call", Reference: id}})
		if err != nil {
			t.Fatal(err)
		}
		return ctx
	}
	first, err := captured.Call(calling("extract"), request)
	if err == nil || first == nil || first.Text() != "known response" {
		t.Fatalf("original observer outcome: %+v, %v", first, err)
	}
	contract, _ := pluginsdk.ModelObserverContract()
	empty, err := plugin.NewOrderedComposition(plugin.CompositionRecord{Contract: contract.Identity(), Members: []plugin.CompositionMember{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := observer.calls.compositions.Save(t.Context(), empty); err != nil {
		t.Fatal(err)
	}
	second, err := captured.Call(calling("curate"), request)
	if err == nil || second == nil {
		t.Fatalf("choice change removed an original observer: %+v, %v", second, err)
	}
	release()
	if held := observer.HeldDependencies(); len(held) != 0 || releases.Load() != 1 {
		t.Fatalf("capture release leaked held state: %+v, %d", held, releases.Load())
	}
	if _, err := captured.Call(calling("late"), request); !errors.Is(err, pluginsdk.ErrStaleCall) || calls.Load() != 2 {
		t.Fatalf("released capture invoked provider: %d, %v", calls.Load(), err)
	}
	text, err := resolve.Complete(t.Context(), models.AuxiliaryPrompt{Operation: "title", SystemPrompt: "write text", UserPrompt: "new input", MaxInputBytes: 128, MaxOutputTokens: 16})
	if err != nil || text != "known response" || calls.Load() != 3 || releases.Load() != 2 || len(observer.HeldDependencies()) != 0 {
		t.Fatalf("new complete did not use/release new choice: %q, %d, %d, %v", text, calls.Load(), releases.Load(), err)
	}
}

func TestAuxiliaryCaptureReleaseJoinsCallsBeforeDroppingHeldRelease(t *testing.T) {
	observer, _, _ := installedObserverFixture(t, "transparent")
	started, canceled, returnModel := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var released atomic.Bool
	resolve := observer.Auxiliary(func(context.Context) (chat.Model, func(), error) {
		return chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
			close(started)
			<-ctx.Done()
			close(canceled)
			<-returnModel
			return nil, ctx.Err()
		}), func() { released.Store(true) }, nil
	})
	model, release, err := resolve(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "closing-model", Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: "closing-model"}})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
	called := make(chan error, 1)
	go func() { _, err := model.Call(ctx, request); called <- err }()
	<-started
	closed := make(chan struct{})
	go func() { release(); close(closed) }()
	<-canceled
	if len(observer.HeldDependencies()) != 1 || released.Load() {
		t.Error("release dropped retained source before the provider joined")
	}
	close(returnModel)
	<-closed
	if err := <-called; !errors.Is(err, context.Canceled) || !released.Load() || len(observer.HeldDependencies()) != 0 {
		t.Fatalf("release did not join canceled model: %t, %v", released.Load(), err)
	}
}
