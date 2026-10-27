package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestInstalledModelContextPreservesOriginalQueryAndRejectsInvalidOrWithdrawnContributors(t *testing.T) {
	for _, mode := range []string{"valid", "invalid", "failure"} {
		t.Run(mode, func(t *testing.T) {
			composition, bindings, store := installedContextFixture(t, mode)
			ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "original-context", Origin: extension.Automatic, Parent: "original-effect", Target: portable.Target{Scope: "call", Reference: "original-context"}})
			if err != nil {
				t.Fatal(err)
			}
			request, _ := chat.NewRequest(chat.NewSystemMessage("original instructions"), chat.NewUserMessage(chat.NewTextPart("question")))
			before := request.Clone()
			selection, err := modelchoice.New("test", "model")
			if err != nil {
				t.Fatal(err)
			}
			result, err := composition.PrepareContext(ctx, bindings, selection, request, []chat.Message{chat.NewUserMessage(chat.NewTextPart("host-owned catalog tail"))})
			if mode == "valid" {
				if err != nil || len(result) != 2 || result[1].Text() != "previous contributions: 0" {
					t.Fatalf("context result: %+v, %v", result, err)
				}
			} else if err == nil {
				t.Fatal("invalid/failed contributor accepted")
			}
			if !reflect.DeepEqual(request, before) {
				t.Fatal("contributor changed caller-owned request")
			}
			store.disable(t)
			if _, err := composition.PrepareContext(ctx, bindings, selection, request, nil); err == nil {
				t.Fatal("withdrawn context contributor executed")
			}
		})
	}
}

func TestInstalledModelContextComposesInCapturedOrder(t *testing.T) {
	composition, bindings, _ := installedContextFixture(t, "ordered")
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "original-context", Origin: extension.Automatic, Target: portable.Target{Scope: "call", Reference: "original-context"}})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := modelchoice.New("test", "model")
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewSystemMessage("original instructions"), chat.NewUserMessage(chat.NewTextPart("question")))
	result, err := composition.PrepareContext(ctx, bindings, selection, request, nil)
	if err != nil || len(result) != 4 || result[1].Text() != "previous contributions: 0" || result[3].Text() != "previous contributions: 1" {
		t.Fatalf("declared context order lost: %+v %v", result, err)
	}
	bindings[1].Export = "replaced"
	if _, err := composition.PrepareContext(ctx, bindings, selection, request, nil); err == nil {
		t.Fatal("changed captured export was admitted")
	}
}

func installedContextFixture(t *testing.T, mode string) (*CallComposition, []plugins.ContributionBinding, *toolInstallationStore) {
	t.Helper()
	contract, err := pluginsdk.ModelContextContract()
	if err != nil {
		t.Fatal(err)
	}
	members := []string{"observer"}
	if mode == "ordered" {
		members = append(members, "second")
	}
	observer, store := installedCallCompositionFixture(t, contract, members, func(backend *qimumcp.Backend) {
		method, _ := pluginsdk.ModelContextMethod()
		if err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[pluginsdk.ModelContextRequest]) (pluginsdk.ModelContextResult, error) {
			invocation, err := qimumcp.InvocationFromContext(ctx)
			if err != nil || !strings.HasPrefix(invocation.ID, "original-context") || invocation.Target.Reference != invocation.ID || command.ID != "" {
				return pluginsdk.ModelContextResult{}, errors.New("original query attribution lost")
			}
			if err := command.Input.Validate(); err != nil {
				return pluginsdk.ModelContextResult{}, err
			}
			if command.Input.UserMessage == nil || command.Input.UserMessage.Text() != "question" {
				return pluginsdk.ModelContextResult{}, errors.New("host tail replaced the original user input")
			}
			if mode == "failure" {
				return pluginsdk.ModelContextResult{}, errors.New("context unavailable")
			}
			if mode == "invalid" {
				return pluginsdk.ModelContextResult{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("forged user"))}}, nil
			}
			count := 0
			for _, message := range command.Input.Request.Messages {
				if message.Text() == "retrieval material" {
					count++
				}
			}
			return pluginsdk.ModelContextResult{Messages: []chat.Message{chat.NewSystemMessage("retrieval material"), chat.NewSystemMessage(fmt.Sprintf("previous contributions: %d", count))}}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}, http.NotFoundHandler())
	bindings, err := observer.ResolveContext(t.Context())
	if err != nil || len(bindings) != len(members) {
		t.Fatalf("composition resolve: %+v %v", bindings, err)
	}
	return observer, captureTestContributionBindings(t, observer, bindings), store
}
