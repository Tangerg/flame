package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestAuxiliaryCaptureHoldsOriginalObserversAcrossCallsUntilRelease(t *testing.T) {
	observer, bindings, _ := installedPublishedObserverFixture(t, "outer-failure")
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
	resolve := observer.Auxiliary(func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func(), error) {
		return auxiliaryFixture(model), func() { releases.Add(1) }, nil
	})
	captured, release, err := resolve(t.Context(), models.AuxiliaryBudget{MaxInputBytes: 1024, MaxOutputTokens: 64})
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
	request.Options.MaxOutputTokens = new(int64(64))
	calling := func(id string) context.Context {
		t.Helper()
		ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: id, Origin: capability.Automatic, Parent: "original-maintenance", Target: capability.Target{Scope: "call", Reference: id}})
		if err != nil {
			t.Fatal(err)
		}
		return ctx
	}
	first, err := captured.Model().Call(calling("extract"), request)
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
	second, err := captured.Model().Call(calling("curate"), request)
	if err == nil || second == nil {
		t.Fatalf("choice change removed an original observer: %+v, %v", second, err)
	}
	release()
	if held := observer.HeldDependencies(); len(held) != 0 || releases.Load() != 1 {
		t.Fatalf("capture release leaked held state: %+v, %d", held, releases.Load())
	}
	if _, err := captured.Model().Call(calling("late"), request); !errors.Is(err, pluginsdk.ErrStaleCall) || calls.Load() != 2 {
		t.Fatalf("released capture invoked provider: %d, %v", calls.Load(), err)
	}
	text, err := resolve.Complete(t.Context(), models.AuxiliaryPrompt{Operation: "title", SystemPrompt: "write text", UserPrompt: "new input", MaxInputBytes: 128, MaxOutputTokens: 16})
	if err != nil || text != "known response" || calls.Load() != 3 || releases.Load() != 2 || len(observer.HeldDependencies()) != 0 {
		t.Fatalf("new complete did not use/release new choice: %q, %d, %d, %v", text, calls.Load(), releases.Load(), err)
	}
}

func TestAuxiliaryCaptureReleaseJoinsCallsBeforeDroppingHeldRelease(t *testing.T) {
	observer, _, _ := installedPublishedObserverFixture(t, "transparent")
	started, canceled, returnModel := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var released atomic.Bool
	resolve := observer.Auxiliary(func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func(), error) {
		return auxiliaryFixture(chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
			close(started)
			<-ctx.Done()
			close(canceled)
			<-returnModel
			return nil, ctx.Err()
		})), func() { released.Store(true) }, nil
	})
	model, release, err := resolve(t.Context(), models.AuxiliaryBudget{MaxInputBytes: 1024, MaxOutputTokens: 64})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "closing-model", Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: "closing-model"}})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
	request.Options.MaxOutputTokens = new(int64(64))
	called := make(chan error, 1)
	go func() { _, err := model.Model().Call(ctx, request); called <- err }()
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

func TestAuxiliaryContextSharesCapturedIdentityLifetimeAndFinalBudget(t *testing.T) {
	for _, mode := range []string{"valid", "input-budget", "output-budget", "failure"} {
		t.Run(mode, func(t *testing.T) {
			fixtureMode := "valid"
			if mode == "failure" {
				fixtureMode = "failure"
			}
			composition, bindings, store := installedContextFixture(t, fixtureMode)
			var calls, releases atomic.Int32
			provider := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				calls.Add(1)
				invocation, err := pluginsdk.InvocationFromContext(ctx)
				if err != nil || invocation.Parent != "original-effect" {
					t.Errorf("auxiliary attribution changed: %+v %v", invocation, err)
				}
				if len(request.Messages) != 4 || request.Messages[0].Text() != "instructions" || request.Messages[1].Text() != "retrieval material" || request.Messages[2].Text() != "previous contributions: 0" || request.Messages[3].Text() != "question" {
					t.Errorf("auxiliary composed input changed: %+v", request.Messages)
				}
				message := chat.NewAssistantMessage(chat.NewTextPart("done"))
				return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
			})
			resolve := composition.Auxiliary(func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func(), error) {
				return auxiliaryFixture(provider), func() { releases.Add(1) }, nil
			})
			budget := models.AuxiliaryBudget{MaxInputBytes: 128, MaxOutputTokens: 64}
			if mode == "input-budget" {
				budget.MaxInputBytes = 32
			}
			capture, release, err := resolve(t.Context(), budget)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if capture.Selection() != testsupport.DefaultModelSelection() {
				t.Fatal("auxiliary exact selection changed")
			}
			if held := composition.HeldDependencies(); len(held) != 1 || held[0].InstallationID != bindings[0].InstallationID || held[0].Digest != bindings[0].Digest {
				t.Fatalf("context release was not retained: %+v", held)
			}
			request, _ := chat.NewRequest(chat.NewSystemMessage("instructions"), chat.NewUserMessage(chat.NewTextPart("question")))
			request.Options.MaxOutputTokens = new(int64(64))
			if mode == "output-budget" {
				request.Options.MaxOutputTokens = new(int64(65))
			}
			before := request.Clone()
			for _, id := range []string{"original-context-first", "original-context-second"} {
				ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: id, Origin: capability.Automatic, Parent: "original-effect", Target: capability.Target{Scope: "call", Reference: id}})
				if err != nil {
					t.Fatal(err)
				}
				_, err = capture.Model().Call(ctx, request)
				if (err == nil) != (mode == "valid") {
					t.Fatalf("auxiliary %s admission: %v", mode, err)
				}
			}
			if !reflect.DeepEqual(request, before) {
				t.Fatal("auxiliary contribution modified caller-owned request")
			}
			wantCalls := int32(0)
			if mode == "valid" {
				wantCalls = 2
			}
			if calls.Load() != wantCalls {
				t.Fatalf("failed composition reached provider: %d", calls.Load())
			}
			store.snapshot.Installation.Disable()
			ctx, _ := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "original-context-withdrawn", Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: "original-context-withdrawn"}})
			if _, err := capture.Model().Call(ctx, request); err == nil {
				t.Fatal("withdrawn context capture dispatched provider")
			}
			release()
			if releases.Load() != 1 || len(composition.HeldDependencies()) != 0 {
				t.Fatal("auxiliary context outlived its owning use case")
			}
		})
	}
}

type contextCapacityCounter struct {
	count    int64
	calls    int
	expected *chat.Request
	t        *testing.T
}

func (c *contextCapacityCounter) CountInputTokens(_ context.Context, request *chat.Request) (int64, error) {
	c.calls++
	if len(request.Messages) != 4 || request.Messages[1].Text() != "retrieval material" || request.Options.ReasoningEffort != chat.ReasoningEffort("high") {
		c.t.Error("capacity counter lost composed context or selected reasoning")
	}
	c.expected = request.Clone()
	return c.count, nil
}
func TestAuxiliaryFinalTokenCapacityIncludesContextAndBlocksProvider(t *testing.T) {
	for _, count := range []int64{272000, 272001} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			composition, _, _ := installedContextFixture(t, "valid")
			counter := &contextCapacityCounter{count: count, t: t}
			var calls atomic.Int32
			provider := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				calls.Add(1)
				if !reflect.DeepEqual(request, counter.expected) {
					t.Error("provider request differs from counted request")
				}
				message := chat.NewAssistantMessage(chat.NewTextPart("done"))
				return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
			})
			selected, err := modelchoice.NewWithReasoningEffort("openai", "gpt-5-pro", "high")
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := models.NewResolvedChat(provider, counter)
			if err != nil {
				t.Fatal(err)
			}
			base, err := models.NewSelectedChat(selected, resolved)
			if err != nil {
				t.Fatal(err)
			}
			resolve := composition.Auxiliary(func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func(), error) {
				return base, func() {}, nil
			})
			capture, release, err := resolve(t.Context(), models.AuxiliaryBudget{MaxInputBytes: 128, MaxOutputTokens: 64})
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			request, _ := chat.NewRequest(chat.NewSystemMessage("instructions"), chat.NewUserMessage(chat.NewTextPart("question")))
			request.Options.MaxOutputTokens = new(int64(64))
			before := request.Clone()
			ctx, _ := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "original-context-capacity", Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: "original-context-capacity"}})
			_, err = capture.Model().Call(ctx, request)
			wantCalls := int32(0)
			if count == 272000 {
				wantCalls = 1
			}
			if (err == nil) != (wantCalls == 1) || calls.Load() != wantCalls || counter.calls != 1 {
				t.Fatalf("final capacity admission changed: provider=%d counter=%d %v", calls.Load(), counter.calls, err)
			}
			if !reflect.DeepEqual(request, before) {
				t.Fatal("preparation changed original request")
			}
		})
	}
}

func TestAuxiliaryReleaseCancelsAndJoinsCapacityCountingBeforeDroppingDependencies(t *testing.T) {
	composition, _, _ := installedContextFixture(t, "valid")
	started, canceled, canReturn := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var released atomic.Bool
	var calls atomic.Int32
	counter := auxiliaryCancelableCounter{started: started, canceled: canceled, canReturn: canReturn}
	provider := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected generation")
	})
	resolved, err := models.NewResolvedChat(provider, &counter)
	if err != nil {
		t.Fatal(err)
	}
	base, err := models.NewSelectedChat(testsupport.MustModelSelection("openai", "gpt-5-pro"), resolved)
	if err != nil {
		t.Fatal(err)
	}
	resolve := composition.Auxiliary(func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func(), error) {
		return base, func() { released.Store(true) }, nil
	})
	capture, release, err := resolve(t.Context(), models.AuxiliaryBudget{MaxInputBytes: 128, MaxOutputTokens: 64})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewSystemMessage("instructions"), chat.NewUserMessage(chat.NewTextPart("question")))
	request.Options.MaxOutputTokens = new(int64(64))
	ctx, _ := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "original-context-counting", Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: "original-context-counting"}})
	result := make(chan error, 1)
	go func() { _, err := capture.Model().Call(ctx, request); result <- err }()
	<-started
	closed := make(chan struct{})
	go func() { release(); close(closed) }()
	<-canceled
	if released.Load() || len(composition.HeldDependencies()) != 1 {
		t.Error("capacity counting released its owner before joining")
	}
	close(canReturn)
	<-closed
	if err := <-result; !errors.Is(err, context.Canceled) || !released.Load() || len(composition.HeldDependencies()) != 0 || calls.Load() != 0 {
		t.Fatalf("capacity cancellation dispatched or leaked: provider=%d released=%t %v", calls.Load(), released.Load(), err)
	}
}

type auxiliaryCancelableCounter struct{ started, canceled, canReturn chan struct{} }

func (c *auxiliaryCancelableCounter) CountInputTokens(ctx context.Context, _ *chat.Request) (int64, error) {
	close(c.started)
	<-ctx.Done()
	close(c.canceled)
	<-c.canReturn
	return 0, ctx.Err()
}
