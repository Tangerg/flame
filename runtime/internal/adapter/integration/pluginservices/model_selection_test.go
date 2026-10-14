package pluginservices

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"reflect"
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

func TestInstalledModelSelectionPreservesProposalAndOriginalCallerWithoutFallback(t *testing.T) {
	for _, mode := range []string{"selected", "invalid", "failure", "withdrawn"} {
		t.Run(mode, func(t *testing.T) {
			contract, err := pluginsdk.ModelSelectionContract()
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			composition, store := installedModelCompositionFixture(t, contract, []string{"observer"}, func(backend *pluginsdk.Backend) {
				method, err := pluginsdk.ModelSelectionMethod()
				if err != nil {
					t.Fatal(err)
				}
				if err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[pluginsdk.ModelSelectionRequest]) (pluginsdk.ModelSelection, error) {
					calls.Add(1)
					invocation, err := pluginsdk.InvocationFromContext(ctx)
					if err != nil || invocation.Origin != capability.Human || invocation.Parent != "original-selection-parent" || invocation.ID != invocation.Target.Reference || command.ID != "" {
						return pluginsdk.ModelSelection{}, errors.New("original selection query attribution changed")
					}
					if command.Input.Proposed.Provider != "provider" || command.Input.Proposed.Model != "proposed-model" || command.Input.Proposed.ReasoningEffort != "high" {
						return pluginsdk.ModelSelection{}, errors.New("proposal identity changed")
					}
					if mode == "failure" {
						return pluginsdk.ModelSelection{}, errors.New("route refused")
					}
					if mode == "invalid" {
						return pluginsdk.ModelSelection{Provider: "provider"}, nil
					}
					return pluginsdk.ModelSelection{Provider: "provider", Model: "selected-model", ReasoningEffort: "low"}, nil
				}); err != nil {
					t.Fatal(err)
				}
			})
			if mode == "withdrawn" {
				store.snapshot.Installation.Disable()
			}
			proposed, err := modelref.NewWithReasoningEffort("provider", "proposed-model", "high")
			if err != nil {
				t.Fatal(err)
			}
			ctx := operation.Adopt(t.Context(), "original-selection-parent", capability.Human, "original-start")
			request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
			before := request.Clone()
			actual, err := composition.SelectModel(ctx, proposed, request)
			if mode == "selected" {
				if err != nil || actual.Provider() != "provider" || actual.Model() != "selected-model" || actual.ReasoningEffort() != "low" || calls.Load() != 1 {
					t.Fatalf("selected route: %+v %v calls=%d", actual, err, calls.Load())
				}
			} else if err == nil || actual.Configured() {
				t.Fatalf("unavailable selection silently fell back: %+v %v", actual, err)
			}
			if mode == "withdrawn" && (!errors.Is(err, plugin.ErrUnavailable) || calls.Load() != 0) {
				t.Fatalf("withdrawn selector was queried: %v %d", err, calls.Load())
			}
			if !reflect.DeepEqual(request, before) {
				t.Fatal("selector changed caller-owned request")
			}
		})
	}
}
func TestAbsentModelSelectionDoesNotDiscoverAnImplicitProvider(t *testing.T) {
	composition, _, _ := installedContextFixture(t, "valid")
	proposed := testsupport.DefaultModelSelection()
	actual, err := composition.SelectModel(t.Context(), proposed, nil)
	if err != nil || actual != proposed {
		t.Fatalf("unselected route changed: %+v %v", actual, err)
	}
}
