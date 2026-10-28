package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"iter"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestModelCallBudgetIncludesCompleteRequestAndExactOutputBoundary(t *testing.T) {
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("input")))
	request.Options.MaxOutputTokens = new(int64(64))
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	budget := ModelCallBudget{MaxRequestBytes: int64(len(encoded)), MaxOutputTokens: 64}
	if err := budget.Admit(request); err != nil {
		t.Fatal(err)
	}
	budget.MaxRequestBytes--
	if err := budget.Admit(request); err == nil {
		t.Fatal("over-budget framing admitted")
	}
	budget.MaxRequestBytes++
	request.Options.MaxOutputTokens = new(int64(65))
	if err := budget.Admit(request); err == nil {
		t.Fatal("over-budget output admitted")
	}
	request.Options.MaxOutputTokens = nil
	if err := budget.Admit(request); err == nil {
		t.Fatal("missing output reservation admitted")
	}
}

func TestSelectedModelSharesPublishedRPCAndRequiresOriginalInvocation(t *testing.T) {
	for _, owning := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-owner", true: "original-owner"}[owning], func(t *testing.T) {
			host := newTestCallbacks(t)
			session := new(sdk.ClientSession)
			if err := host.AdmitSession(session); err != nil {
				t.Fatal(err)
			}
			recorded := make(chan portable.Invocation, 1)
			callback, err := NewSelectedModelCallback(func(ctx context.Context, command qimumcp.Command[ModelCallRequest]) (*chat.Response, error) {
				actual, _ := qimumcp.InvocationFromContext(ctx)
				recorded <- actual
				if command.Input.Selection.Model != "configured" {
					return nil, errors.New("route changed")
				}
				message := chat.NewAssistantMessage(chat.NewTextPart("known"))
				return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
			})
			if err != nil {
				t.Fatal(err)
			}
			var rpcCalls atomic.Int32
			if err := host.PublishRPC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { rpcCalls.Add(1); w.WriteHeader(http.StatusNoContent) }), callback); err != nil {
				t.Fatal(err)
			}
			ctx := t.Context()
			if owning {
				ctx, err = qimumcp.WithInvocation(ctx, portable.Invocation{ID: "original-owner", Origin: extension.Human, Parent: "original-parent", Target: portable.Target{Scope: "call", Reference: "original-owner"}})
				if err != nil {
					t.Fatal(err)
				}
			}
			ref, release, err := host.OpenRPC(ctx, session, qimumcp.CallbackLimits{MaxRequestBytes: 64 << 10, MaxResponseBytes: 64 << 10})
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("input")))
			request.Options.MaxOutputTokens = new(int64(64))
			input := ModelCallRequest{Selection: ModelSelection{Provider: "anthropic", Model: "configured"}, Budget: ModelCallBudget{MaxRequestBytes: 4096, MaxOutputTokens: 64}, Request: *request}
			response, err := CallSelectedModel(t.Context(), ref, "fresh-model-call", input)
			if !owning {
				if err == nil || response != nil || len(recorded) != 0 {
					t.Fatalf("unowned model dispatched: %+v %v", response, err)
				}
				return
			}
			actual := <-recorded
			want := portable.Invocation{ID: "fresh-model-call", Origin: extension.Human, Parent: "original-owner", Target: portable.Target{Scope: "call", Reference: "fresh-model-call"}}
			if err != nil || response == nil || !reflect.DeepEqual(actual, want) {
				t.Fatalf("original owner changed: %+v %+v %v", actual, response, err)
			}
			r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ref.Endpoint+"v2/rpc", nil)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", ref.Credential)
			r.Header.Set(qimumcp.CallHeader, ref.Call)
			returned, err := http.DefaultClient.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			returned.Body.Close()
			if returned.StatusCode != http.StatusNoContent || rpcCalls.Load() != 1 {
				t.Fatal("model callbacks displaced canonical RPC")
			}
		})
	}
}

func TestSelectedBackgroundCallbacksProjectOnlyRetainedOwnerForCallAndStream(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	if err := host.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	original := portable.Invocation{ID: "retained-operation", Origin: extension.Automatic, Parent: "original-trigger", Target: portable.Target{Scope: "call", Reference: "retained-operation"}}
	records := make(chan portable.Invocation, 2)
	unary, err := NewSelectedModelCallback(func(ctx context.Context, _ qimumcp.Command[ModelCallRequest]) (*chat.Response, error) {
		inv, _ := qimumcp.InvocationFromContext(ctx)
		records <- inv
		message := chat.NewAssistantMessage(chat.NewTextPart("known"))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := NewSelectedModelStreamCallback(func(ctx context.Context, _ qimumcp.Command[ModelCallRequest]) iter.Seq2[*chat.ResponseDelta, error] {
		return func(yield func(*chat.ResponseDelta, error) bool) {
			inv, _ := qimumcp.InvocationFromContext(ctx)
			records <- inv
			yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("known")}, FinishReason: chat.FinishReasonStop}, nil)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.PublishRPC(http.NotFoundHandler(), unary, stream); err != nil {
		t.Fatal(err)
	}
	var projections atomic.Int32
	ref, release, err := host.OpenRPCWithInvocation(t.Context(), session, qimumcp.CallbackLimits{MaxRequestBytes: 64 << 10, MaxResponseBytes: 64 << 10}, func(ctx context.Context, wire portable.Invocation) (context.Context, error) {
		if wire != original {
			return nil, errors.New("unretained invocation")
		}
		projections.Add(1)
		return qimumcp.WithInvocation(ctx, wire)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, err := qimumcp.WithInvocation(t.Context(), original)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("input")))
	request.Options.MaxOutputTokens = new(int64(64))
	input := ModelCallRequest{Selection: ModelSelection{Provider: "anthropic", Model: "configured"}, Budget: ModelCallBudget{MaxRequestBytes: 4096, MaxOutputTokens: 64}, Request: *request}
	response, err := CallSelectedModel(ctx, ref, "background-unary", input)
	if err != nil || response == nil {
		t.Fatalf("retained background unary: %+v %v", response, err)
	}
	for _, err := range StreamSelectedModel(ctx, ref, "background-stream", input) {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"background-unary", "background-stream"} {
		got := <-records
		if got.ID != id || got.Parent != original.ID || got.Origin != original.Origin {
			t.Fatalf("background owner changed: %+v", got)
		}
	}
	forgedOwner := original
	forgedOwner.Origin = extension.Human
	forged, err := qimumcp.WithInvocation(t.Context(), forgedOwner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CallSelectedModel(forged, ref, "forged-call", input); err == nil {
		t.Fatal("forged background owner admitted")
	}
	refused := false
	for _, err := range StreamSelectedModel(forged, ref, "forged-stream", input) {
		if err == nil {
			t.Fatal("forged background stream admitted")
		}
		refused = true
	}
	if !refused {
		t.Fatal("forged background stream ended without refusal")
	}
	if projections.Load() != 2 || len(records) != 0 {
		t.Fatalf("unretained work reached model: %d", projections.Load())
	}
}
