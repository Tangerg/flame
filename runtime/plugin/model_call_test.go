package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
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
			host.AdmitSession(session)
			recorded := make(chan capability.Invocation, 1)
			callback, err := NewSelectedModelCallback(func(ctx context.Context, command Command[ModelCallRequest]) (*chat.Response, error) {
				actual, _ := InvocationFromContext(ctx)
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
				ctx, err = WithInvocation(ctx, capability.Invocation{ID: "original-owner", Origin: capability.Human, Parent: "original-parent", Target: capability.Target{Scope: "call", Reference: "original-owner"}})
				if err != nil {
					t.Fatal(err)
				}
			}
			ref, release, err := host.OpenRPC(ctx, session, CallbackLimits{MaxRequestBytes: 64 << 10, MaxResponseBytes: 64 << 10})
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
			want := capability.Invocation{ID: "fresh-model-call", Origin: capability.Human, Parent: "original-owner", Target: capability.Target{Scope: "call", Reference: "fresh-model-call"}}
			if err != nil || response == nil || !reflect.DeepEqual(actual, want) {
				t.Fatalf("original owner changed: %+v %+v %v", actual, response, err)
			}
			r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ref.Endpoint+"v2/rpc", nil)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", ref.Credential)
			r.Header.Set("Flame-Call", ref.Call)
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
