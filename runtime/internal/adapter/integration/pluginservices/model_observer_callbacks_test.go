package pluginservices

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

type continuationOwnerModel struct {
	check func(context.Context)
	calls atomic.Int32
}

func (m *continuationOwnerModel) Call(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
	m.check(ctx)
	m.calls.Add(1)
	message := chat.NewAssistantMessage(chat.NewTextPart("original endpoint"))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
}

func (m *continuationOwnerModel) Stream(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		m.check(ctx)
		m.calls.Add(1)
		yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("original endpoint")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 9007199254740993, OutputTokens: 2}}}, nil)
	}
}

func TestObserverCohostsRPCAndNextWithDistinctCapturedAuthority(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "call", true: "stream"}[streaming], func(t *testing.T) {
			contract, err := pluginsdk.ModelObserverContract()
			if err != nil {
				t.Fatal(err)
			}
			original := capability.Invocation{ID: "original-model", Origin: capability.Model, Parent: "original-effect", Target: capability.Target{Scope: "call", Reference: "original-model"}}
			refs := make(chan pluginsdk.CallbackRef, 1)
			beforeNext := func(ctx context.Context) (pluginsdk.CallbackRef, error) {
				ref, err := pluginsdk.CallbackFromContext(ctx)
				if err != nil {
					return ref, err
				}
				select {
				case refs <- ref:
				default:
					return ref, errors.New("observer was dispatched recursively")
				}
				if status, err := phaseHostRequest(ctx, ref); err != nil || status != http.StatusNoContent {
					return ref, errors.Join(err, errors.New("observer host RPC failed"))
				}
				_, err = pluginsdk.CallSelectedModel(ctx, ref, "independent-from-observer", selectedCallInput("configured-model"))
				failure, known := errors.AsType[*pluginsdk.ModelFailure](err)
				if !known || failure.Code != pluginsdk.ModelFailed || failure.Message != plugin.ErrInvalid.Error()+": recursive model contributor observer" {
					return ref, errors.Join(err, errors.New("independent exchange did not retain its explicit observer-cycle refusal"))
				}
				return ref, nil
			}
			composition, store := installedModelCompositionFixture(t, contract, []string{"observer"}, func(backend *pluginsdk.Backend) {
				call, _ := pluginsdk.ModelMethod()
				if err := call.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, input pluginsdk.Command[chat.Request]) (pluginsdk.ModelResult, error) {
					ref, err := beforeNext(ctx)
					if err != nil {
						return pluginsdk.ModelResult{}, err
					}
					response, err := pluginsdk.CallModel(ctx, ref, input.ID, input.Input.Clone())
					return pluginsdk.ModelResult{Response: response}, err
				}); err != nil {
					t.Fatal(err)
				}
				stream, _ := pluginsdk.ModelStreamMethod()
				if err := stream.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, input pluginsdk.Command[chat.Request]) iter.Seq2[pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult], error] {
					return func(yield func(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult], error) bool) {
						ref, err := beforeNext(ctx)
						if err != nil {
							yield(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult]{}, err)
							return
						}
						pluginsdk.StreamModel(ctx, ref, input.ID, input.Input.Clone())(yield)
					}
				}); err != nil {
					t.Fatal(err)
				}
			})
			caller, err := operation.NewActor(store.snapshot.Installation.ID().String(), testsupport.Digest("2").String(), testsupport.Digest("3").String(), "caller")
			if err != nil {
				t.Fatal(err)
			}
			model := &continuationOwnerModel{check: func(ctx context.Context) {
				actor, found := operation.ActorFrom(ctx)
				invocation, err := pluginsdk.InvocationFromContext(ctx)
				if !found || actor != caller || err != nil || invocation != original {
					t.Error("next adopted the observer's authority or changed original invocation")
				}
			}}
			var hostCalls atomic.Int32
			host := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actor, found := operation.ActorFrom(r.Context())
				invocation, err := pluginsdk.InvocationFromContext(r.Context())
				if !found || actor.Server().String() != "observer" || actor.Release() != store.snapshot.Selected.Digest() || err != nil || invocation != original {
					t.Error("RPC did not retain the observer activation and original invocation")
					http.Error(w, "wrong receiver", http.StatusForbidden)
					return
				}
				hostCalls.Add(1)
				w.WriteHeader(http.StatusNoContent)
			})
			callbacks, err := composition.ModelCallbacks(&routedCallResolver{model: model})
			if err != nil {
				t.Fatal(err)
			}
			if err := composition.callbacks.PublishRPC(host, callbacks...); err != nil {
				t.Fatal(err)
			}
			bindings, err := composition.Resolve(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			bound, streamer := composition.Bind(model, model, bindings)
			ctx, err := pluginsdk.WithInvocation(operation.WithActor(t.Context(), caller), original)
			if err != nil {
				t.Fatal(err)
			}
			request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
			var response *chat.Response
			if streaming {
				var accumulated chat.ResponseAccumulator
				for delta, failure := range streamer.Stream(ctx, request) {
					if failure != nil {
						t.Fatal(failure)
					}
					if err := accumulated.Add(delta); err != nil {
						t.Fatal(err)
					}
				}
				response, err = accumulated.Response()
				if err != nil || response.Metadata.Usage.InputTokens != 9007199254740993 {
					t.Fatalf("stream changed canonical usage: %+v %v", response, err)
				}
			} else {
				response, err = bound.Call(ctx, request)
			}
			if err != nil || response == nil || response.Text() != "original endpoint" || model.calls.Load() != 1 || hostCalls.Load() != 1 {
				t.Fatalf("cohosted result: %+v err=%v provider=%d host=%d", response, err, model.calls.Load(), hostCalls.Load())
			}
			if status, err := phaseHostRequest(t.Context(), <-refs); err != nil || status != http.StatusGone {
				t.Fatalf("observer retained RPC after completion: %d %v", status, err)
			}
			if len(composition.HeldDependencies()) != 0 {
				t.Fatal("refused independent exchange retained a dependency")
			}
		})
	}
}
