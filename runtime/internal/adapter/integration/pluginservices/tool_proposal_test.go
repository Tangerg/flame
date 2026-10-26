package pluginservices

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestCapturedContributionActivationClassifiesMissingLiveSource(t *testing.T) {
	contract, err := pluginsdk.ToolProposalContract()
	if err != nil {
		t.Fatal(err)
	}
	composition, _ := installedCallCompositionFixture(t, contract, []string{"proposal"}, func(backend *pluginsdk.Backend) {
		method, err := pluginsdk.ToolProposalMethod()
		if err != nil {
			t.Fatal(err)
		}
		if err := method.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command pluginsdk.Command[chat.ToolCall]) (chat.ToolCall, error) {
			return command.Input, nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	bindings, err := composition.ResolveToolProposals(t.Context())
	if err != nil || len(bindings) != 1 {
		t.Fatalf("capture installed contribution: %+v, %v", bindings, err)
	}
	if err := composition.calls.pool.Detach(bindings[0].ServerID); err != nil {
		t.Fatal(err)
	}
	if composition.Pending(bindings) {
		t.Fatal("retired source became a pending connection")
	}
	if _, err := composition.Activations(t.Context(), bindings); !errors.Is(err, plugin.ErrUnavailable) || !errors.Is(err, mcp.ErrUnavailable) {
		t.Fatalf("missing source lost its definite availability category: %v", err)
	}
}

func TestInstalledToolProposalsPreserveOrderCallerAndWithdrawal(t *testing.T) {
	for _, mode := range []string{"ordered", "identity", "name", "failure"} {
		t.Run(mode, func(t *testing.T) {
			contract, err := pluginsdk.ToolProposalContract()
			if err != nil {
				t.Fatal(err)
			}
			var calls int
			composition, store := installedCallCompositionFixture(t, contract, []string{"first", "second"}, func(backend *pluginsdk.Backend) {
				method, err := pluginsdk.ToolProposalMethod()
				if err != nil {
					t.Fatal(err)
				}
				if err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[chat.ToolCall]) (chat.ToolCall, error) {
					caller, err := pluginsdk.InvocationFromContext(ctx)
					if err != nil || caller.ID != "original" || caller.Parent != "run_owner" || caller.Origin != extension.Model || caller.Target.Reference != "original" || command.ID != "" {
						return chat.ToolCall{}, errors.New("original Tool caller was replaced")
					}
					calls++
					proposal := command.Input
					switch mode {
					case "identity":
						proposal.ID = "changed"
					case "name":
						proposal.Name = "changed"
					case "failure":
						return chat.ToolCall{}, errors.New("transformation failed")
					default:
						if (calls == 1 && proposal.Arguments != `{"value":"original"}`) || (calls == 2 && proposal.Arguments != `{"value":"first"}`) {
							return chat.ToolCall{}, errors.New("previous transformation was replaced")
						}
						proposal.Arguments = `{"value":"first"}`
						if calls == 2 {
							proposal.Arguments = `{"value":"second"}`
						}
					}
					return proposal, nil
				}); err != nil {
					t.Fatal(err)
				}
			}, http.NotFoundHandler())
			bindings, err := composition.ResolveToolProposals(t.Context())
			if err != nil || len(bindings) != 2 {
				t.Fatalf("installed composition: %+v, %v", bindings, err)
			}
			ctx, err := pluginsdk.WithInvocation(t.Context(), extension.Invocation{ID: "original", Origin: extension.Model, Parent: "run_owner", Target: extension.Target{Scope: "call", Reference: "original"}})
			if err != nil {
				t.Fatal(err)
			}
			original := chat.ToolCall{ID: "original", Name: "write_record", Arguments: `{"value":"original"}`}
			result, err := composition.PrepareToolProposal(ctx, bindings, original)
			if mode == "ordered" {
				if err != nil || result.Arguments != `{"value":"second"}` || calls != 2 {
					t.Fatalf("captured transformations: %+v calls=%d, %v", result, calls, err)
				}
			} else if err == nil || calls != 1 {
				t.Fatalf("invalid contributor proceeded: %+v calls=%d, %v", result, calls, err)
			}
			store.snapshot.Installation.Disable()
			before := calls
			if _, err := composition.PrepareToolProposal(ctx, bindings, original); err == nil || calls != before {
				t.Fatalf("withdrawn contribution was entered: calls=%d, %v", calls, err)
			}
		})
	}
}
