package pluginservices

import (
	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	coreinvocation "github.com/Tangerg/scope/core/invocation"

	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	sdk "github.com/Tangerg/go-sdk/mcp"

	"github.com/Tangerg/scope/core/chat"
)

func TestModelProviderRejectsOversizedCompleteFrameBeforeDispatch(t *testing.T) {
	contract, err := modelprovider.Contract()
	if err != nil {
		t.Fatal(err)
	}
	method, err := modelprovider.CallMethod()
	if err != nil {
		t.Fatal(err)
	}
	installation, err := identity.ParseInstallation("7ea9e90b-9f0a-4a55-af5f-f30319e61abc")
	if err != nil {
		t.Fatal(err)
	}
	use := &providerUse{owner: &ModelProviders{contract: contract}, binding: plugins.ContributionBinding{InstallationID: installation, Export: "model-provider"}, handle: modelprovider.Handle{ID: modelprovider.HandleID(strings.Repeat("a", 64))}, requestMethods: []string{method.Name()}}
	ctx, err := pluginmcp.WithInvocation(t.Context(), framemcp.Invocation{ID: "original-call", Origin: extension.Human, Target: portable.Target{Scope: extension.ScopeCall, Reference: "original-call"}})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("small input")))
	if fits, err := use.futureRequestFits(ctx, request); err != nil || !fits {
		t.Fatalf("ordinary complete request: fits=%t %v", fits, err)
	}
	request.Messages[0] = chat.NewUserMessage(chat.NewTextPart(strings.Repeat("x", sdk.DefaultMaxLineLength)))
	if fits, err := use.futureRequestFits(ctx, request); err != nil || fits {
		t.Fatalf("oversized complete request: fits=%t %v", fits, err)
	}
	response, err := (&providerChat{use: use}).Call(ctx, request)
	failure, found := errors.AsType[*coreinvocation.Failure](err)
	if response != nil || !found || failure.Kind != coreinvocation.FailureNotExecuted {
		t.Fatalf("local frame refusal lost execution evidence: %+v %v", response, err)
	}
}
