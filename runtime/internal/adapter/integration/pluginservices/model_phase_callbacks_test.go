package pluginservices

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/runtime/capability"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func phaseHostRequest(ctx context.Context, ref pluginsdk.CallbackRef) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, ref.Endpoint+"v2/rpc", strings.NewReader(`{}`))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", ref.Credential)
	request.Header.Set("Flame-Call", ref.Call)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

func TestModelPreparationPhasesCaptureReentrantHostAuthorityAndReleaseIt(t *testing.T) {
	for _, phase := range []string{"selection", "context", "input"} {
		for _, fail := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "/success", true: "/failure"}[fail], func(t *testing.T) {
				var contract *capability.Contract
				var err error
				switch phase {
				case "selection":
					contract, err = pluginsdk.ModelSelectionContract()
				case "context":
					contract, err = pluginsdk.ModelContextContract()
				case "input":
					contract, err = pluginsdk.ModelInputContract()
				}
				if err != nil {
					t.Fatal(err)
				}
				var store *toolInstallationStore
				var composition *CallComposition
				var bindings []plugins.ContributionBinding
				var hostCalls atomic.Int32
				var backendCalls atomic.Int32
				selection := testsupport.DefaultModelSelection()
				request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
				original := capability.Invocation{ID: "preparing-model", Origin: capability.Model, Parent: "original-tool", Target: capability.Target{Scope: "call", Reference: "preparing-model"}}
				callers := make(chan capability.Invocation, 1)
				refs := make(chan pluginsdk.CallbackRef, 1)
				host := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					actor, found := operation.ActorFrom(r.Context())
					invocation, err := pluginsdk.InvocationFromContext(r.Context())
					var expected capability.Invocation
					select {
					case expected = <-callers:
					default:
						http.Error(w, "no active phase caller", http.StatusForbidden)
						return
					}
					if !found || actor.InstallationID() != store.snapshot.Installation.ID() || actor.Release() != store.snapshot.Selected.Digest() || actor.Server().String() != "observer" || err != nil || invocation != expected {
						t.Error("host callback lost its original invocation or captured backend authority")
						http.Error(w, "wrong owner", http.StatusForbidden)
						return
					}
					hostCalls.Add(1)
					var cycle error
					switch phase {
					case "selection":
						_, cycle = composition.SelectModel(r.Context(), modelapp.SelectionInput{Context: modelchoice.SelectionContext{Use: modelchoice.Interaction}, Proposed: selection, Request: request})
					case "context":
						_, cycle = composition.PrepareContext(r.Context(), bindings, selection, request, nil)
					case "input":
						_, cycle = composition.PrepareInput(r.Context(), bindings, selection, request)
					}
					if !errors.Is(cycle, plugin.ErrInvalid) {
						t.Errorf("nested phase did not refuse its active contributor: %v", cycle)
						http.Error(w, "recursive phase admitted", http.StatusForbidden)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				})
				callback := func(ctx context.Context) error {
					if backendCalls.Add(1) != 1 {
						return errors.New("active contributor was called recursively")
					}
					invocation, err := pluginsdk.InvocationFromContext(ctx)
					if err != nil {
						return err
					}
					if phase == "selection" {
						if invocation.Parent != original.ID || invocation.Origin != original.Origin {
							return errors.New("selector changed original parent or origin")
						}
					} else if invocation != original {
						return errors.New("preparation changed original invocation")
					}
					ref, err := pluginsdk.CallbackFromContext(ctx)
					if err != nil {
						return err
					}
					refs <- ref
					callers <- invocation
					status, err := phaseHostRequest(ctx, ref)
					if err != nil || status != http.StatusNoContent {
						return errors.Join(err, errors.New("nested host request failed"))
					}
					if fail {
						return errors.New("preparation failed after host observation")
					}
					return nil
				}
				installed, installation := installedCallCompositionFixture(t, contract, []string{"phase"}, func(backend *pluginsdk.Backend) {
					switch phase {
					case "selection":
						method, err := pluginsdk.ModelSelectionMethod()
						if err != nil {
							t.Fatal(err)
						}
						err = method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, input pluginsdk.Command[pluginsdk.ModelSelectionRequest]) (pluginsdk.ModelSelection, error) {
							return input.Input.Proposed, callback(ctx)
						})
						if err != nil {
							t.Fatal(err)
						}
					case "context":
						method, err := pluginsdk.ModelContextMethod()
						if err != nil {
							t.Fatal(err)
						}
						err = method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, _ pluginsdk.Command[pluginsdk.ModelContextRequest]) (pluginsdk.ModelContextResult, error) {
							return pluginsdk.ModelContextResult{Messages: []chat.Message{chat.NewSystemMessage("retrieved")}}, callback(ctx)
						})
						if err != nil {
							t.Fatal(err)
						}
					case "input":
						method, err := pluginsdk.ModelInputMethod()
						if err != nil {
							t.Fatal(err)
						}
						err = method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, input pluginsdk.Command[pluginsdk.ModelInputRequest]) (chat.Request, error) {
							return input.Input.Request, callback(ctx)
						})
						if err != nil {
							t.Fatal(err)
						}
					}
				}, host)
				composition = installed
				store = installation
				ctx, err := pluginsdk.WithInvocation(t.Context(), original)
				if err != nil {
					t.Fatal(err)
				}
				actor, err := operation.NewActor(store.snapshot.Installation.ID().String(), store.snapshot.Selected.Digest().String(), testsupport.Digest("3").String(), "caller")
				if err != nil {
					t.Fatal(err)
				}
				ctx = operation.WithActor(ctx, actor)
				direct := installedCall{calls: composition.calls, request: plugins.CallRequest{Installation: store.snapshot.Installation.ID(), Digest: store.snapshot.Selected.Digest(), Contribution: "phase", Contract: contract}}
				if _, err := direct.resolve(ctx); !errors.Is(err, plugin.ErrInvalid) {
					t.Fatalf("managed caller bypassed its declared requirements: %v", err)
				}
				switch phase {
				case "selection":
					_, err = composition.SelectModel(ctx, modelapp.SelectionInput{Context: modelchoice.SelectionContext{Use: modelchoice.Interaction}, Proposed: selection, Request: request})
				case "context":
					var resolveErr error
					bindings, resolveErr = composition.ResolveContext(ctx)
					if resolveErr != nil {
						t.Fatal(resolveErr)
					}
					_, err = composition.PrepareContext(ctx, bindings, selection, request, nil)
				case "input":
					var resolveErr error
					bindings, resolveErr = composition.ResolveInput(ctx)
					if resolveErr != nil {
						t.Fatal(resolveErr)
					}
					_, err = composition.PrepareInput(ctx, bindings, selection, request)
				}
				if (err != nil) != fail || hostCalls.Load() != 1 || backendCalls.Load() != 1 {
					t.Fatalf("phase result: calls=%d failure=%t err=%v", hostCalls.Load(), fail, err)
				}
				if status, err := phaseHostRequest(t.Context(), <-refs); err != nil || status != http.StatusGone {
					t.Fatalf("completed phase retained host authority: status=%d err=%v", status, err)
				}
			})
		}
	}
}
