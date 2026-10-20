package pluginservices

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/runtime/capability"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestAuxiliaryInstalledInputTransformationRetainsOriginalOrderLifetimeAndFinalBudget(t *testing.T) {
	for _, mode := range []string{"ordered", "failure", "route", "output", "input-budget", "token-budget"} {
		t.Run(mode, func(t *testing.T) {
			contract, err := pluginsdk.ModelInputContract()
			if err != nil {
				t.Fatal(err)
			}
			composition, store := installedCallCompositionFixture(t, contract, []string{"first", "second"}, func(backend *pluginsdk.Backend) {
				method, err := pluginsdk.ModelInputMethod()
				if err != nil {
					t.Fatal(err)
				}
				if err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[pluginsdk.ModelInputRequest]) (chat.Request, error) {
					invocation, err := pluginsdk.InvocationFromContext(ctx)
					if err != nil || invocation.ID != invocation.Target.Reference || invocation.Parent != "original-effect" || command.ID != "" {
						return chat.Request{}, errors.New("input transformation lost original query attribution")
					}
					if mode == "failure" {
						return chat.Request{}, errors.New("transform unavailable")
					}
					request := command.Input.Request.Clone()
					if mode == "route" {
						request.Options.Model = "replaced"
					}
					if mode == "output" {
						request.Options.MaxOutputTokens = new(int64(65))
					}
					text := request.Messages[0].Text() + " transformed"
					if mode == "input-budget" {
						text = strings.Repeat("x", 129)
					}
					request.Messages[0] = chat.NewUserMessage(chat.NewTextPart(text))
					return *request, nil
				}); err != nil {
					t.Fatal(err)
				}
			}, http.NotFoundHandler())
			var calls atomic.Int32
			provider := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				calls.Add(1)
				if request.Messages[0].Text() != "question transformed transformed" {
					t.Errorf("captured transformation order changed: %+v", request.Messages)
				}
				message := chat.NewAssistantMessage(chat.NewTextPart("done"))
				return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
			})
			counter := &transformedInputCounter{t: t}
			base := auxiliaryFixture(provider)
			if mode == "token-budget" {
				resolved, err := models.NewResolvedChat(provider, counter)
				if err != nil {
					t.Fatal(err)
				}
				base, err = models.NewSelectedChat(testsupport.MustModelSelection("openai", "gpt-5-pro"), resolved)
				if err != nil {
					t.Fatal(err)
				}
			}
			resolver := composition.Auxiliary(func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func(), error) {
				return base, func() {}, nil
			})
			capture, release, err := resolver(t.Context(), models.AuxiliaryBudget{MaxInputBytes: 128, MaxOutputTokens: 64})
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if held := composition.HeldDependencies(); len(held) != 1 || held[0].InstallationID != store.snapshot.Installation.ID() {
				t.Fatalf("transform release not held once: %+v", held)
			}
			original, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
			original.Options.MaxOutputTokens = new(int64(64))
			before := original.Clone()
			for _, id := range []string{"input-first", "input-second"} {
				ctx, _ := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: id, Origin: capability.Automatic, Parent: "original-effect", Target: capability.Target{Scope: "call", Reference: id}})
				_, err := capture.Model().Call(ctx, original)
				if (err == nil) != (mode == "ordered") {
					t.Fatalf("transformed %s input admission: %v", mode, err)
				}
			}
			want := int32(0)
			if mode == "ordered" {
				want = 2
			}
			if calls.Load() != want {
				t.Fatalf("failed transformation reached provider: %d", calls.Load())
			}
			if !reflect.DeepEqual(original, before) {
				t.Fatal("transformed input entered caller-owned conversation")
			}
			store.snapshot.Installation.Disable()
			ctx, _ := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "withdrawn-input", Origin: capability.Automatic, Parent: "original-effect", Target: capability.Target{Scope: "call", Reference: "withdrawn-input"}})
			if _, err := capture.Model().Call(ctx, original); err == nil {
				t.Fatal("withdrawn transformer was invoked")
			}
			release()
			if len(composition.HeldDependencies()) != 0 {
				t.Fatal("transform dependencies outlived capture")
			}
			if mode == "token-budget" && counter.calls != 2 {
				t.Fatalf("transformed request did not reach capacity admission: %d", counter.calls)
			}
			if capture.Selection() != base.Selection() {
				t.Fatal("transform changed selected identity")
			}
		})
	}
}

type transformedInputCounter struct {
	t     *testing.T
	calls int
}

func (c *transformedInputCounter) CountInputTokens(_ context.Context, request *chat.Request) (int64, error) {
	c.calls++
	if request.Messages[0].Text() != "question transformed transformed" {
		c.t.Error("token admission counted the original input")
	}
	return 272001, nil
}
