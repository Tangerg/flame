package pluginservices

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/qimu"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestCapturedContributionActivationClassifiesMissingLiveSource(t *testing.T) {
	contract, err := pluginsdk.ToolProposalContract()
	if err != nil {
		t.Fatal(err)
	}
	composition, _ := installedCallCompositionFixture(t, contract, []string{"proposal"}, func(backend *qimumcp.Backend) {
		method, err := pluginsdk.ToolProposalMethod()
		if err != nil {
			t.Fatal(err)
		}
		if err := method.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command qimumcp.Command[chat.ToolCall]) (chat.ToolCall, error) {
			return command.Input, nil
		}); err != nil {
			t.Fatal(err)
		}
	}, http.NotFoundHandler())
	bindings, err := composition.ResolveToolProposals(t.Context())
	if err != nil || len(bindings) != 1 {
		t.Fatalf("capture installed contribution: %+v, %v", bindings, err)
	}
	captured := captureTestContributionBindings(t, composition, bindings)
	if err := composition.calls.pool.Detach(bindings[0].ServerID); err != nil {
		t.Fatal(err)
	}
	if composition.Pending(bindings) {
		t.Fatal("retired source became a pending connection")
	}
	if _, err := composition.Activations(t.Context(), bindings); !errors.Is(err, plugin.ErrUnavailable) || !errors.Is(err, mcp.ErrUnavailable) {
		t.Fatalf("missing source lost its definite availability category: %v", err)
	}
	activations, err := composition.Activations(t.Context(), captured)
	if err != nil || len(activations) != 1 || !activations[0].Equal(*captured[0].Activation) {
		t.Fatalf("original captured graph depended on the current catalog: %+v, %v", activations, err)
	}
	if err := composition.calls.framework.WaitRetired(t.Context(), qimu.InstallationID(captured[0].InstallationID.String())); err != nil {
		t.Fatal(err)
	}
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "original", Origin: extension.Model, Parent: "run_owner", Target: portable.Target{Scope: "call", Reference: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	proposal := chat.ToolCall{ID: "original", Name: "write_record", Arguments: `{}`}
	result, err := composition.PrepareToolProposal(ctx, captured, proposal)
	if err != nil || result != proposal {
		t.Fatalf("original contribution could not execute independently of catalog replacement: %+v, %v", result, err)
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
			composition, store := installedCallCompositionFixture(t, contract, []string{"first", "second"}, func(backend *qimumcp.Backend) {
				method, err := pluginsdk.ToolProposalMethod()
				if err != nil {
					t.Fatal(err)
				}
				if err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[chat.ToolCall]) (chat.ToolCall, error) {
					caller, err := qimumcp.InvocationFromContext(ctx)
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
			activations, err := composition.Activations(t.Context(), bindings)
			if err != nil || len(activations) != 1 {
				t.Fatalf("two contributions from one setup produced competing activation owners: %+v, %v", activations, err)
			}
			bindings, err = plugins.BindActivations(bindings, activations)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "original", Origin: extension.Model, Parent: "run_owner", Target: portable.Target{Scope: "call", Reference: "original"}})
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
			store.disable(t)
			before := calls
			if _, err := composition.PrepareToolProposal(ctx, bindings, original); err == nil || calls != before {
				t.Fatalf("withdrawn contribution was entered: calls=%d, %v", calls, err)
			}
		})
	}
}
