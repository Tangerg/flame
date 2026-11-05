package execution

import (
	"context"
	"encoding/json/jsontext"
	"reflect"
	"strings"
	"testing"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestOriginalUserProjectionPreservesTextMediaAndSkipsHostContext(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("original image"))
	if err != nil {
		t.Fatal(err)
	}
	user := chat.NewUserMessage(chat.NewTextPart("question"), chat.NewMediaPart(image))
	if err := user.Parts[0].Metadata.Set("caller/value", jsontext.Value(`9007199254740993`)); err != nil {
		t.Fatal(err)
	}
	messages := originalUserWorkingContext(t, user)
	plan := chat.NewUserMessage(chat.NewTextPart("host-owned plan"))
	if err := (contextSources{contextSourceSessionPlan.source("session_1")}).attach(&plan.Metadata, "plan"); err != nil {
		t.Fatal(err)
	}
	output, err := modeladapter.WithToolResultReference(toolcontract.NewTextOutput("preview"), transcript.ToolResultRef{ID: "BLOB234"}, testToolResultReaderName, modeladapter.ToolResultRange{LengthBytes: 32000})
	if err != nil {
		t.Fatal(err)
	}
	reminder, found, err := modeladapter.RetainedToolResultReferences([]chat.Message{chat.NewToolMessage(chat.ToolResult{ID: "read", Name: "read", Output: output})})
	if err != nil || !found {
		t.Fatalf("reference fixture: %t %v", found, err)
	}
	messages = append(messages, plan, reminder)
	before := cloneChatMessages(messages)
	projected, err := originalUserMessage(messages)
	if err != nil || projected == nil || !reflect.DeepEqual(*projected, user) || !reflect.DeepEqual(messages, before) {
		t.Fatalf("original caller projection changed evidence: %+v %v", projected, err)
	}
	projected.Parts[0].Text = "changed copy"
	projected.Parts[1].Media.Source.Bytes[0] = 'X'
	if !reflect.DeepEqual(messages, before) {
		t.Fatal("editing a contributor projection changed retained history")
	}
	if projected, err := originalUserMessage([]chat.Message{plan, reminder}); err != nil || projected != nil {
		t.Fatalf("host-only context invented caller input: %+v %v", projected, err)
	}
}

func TestOriginalUserProjectionRefusesMalformedProvenance(t *testing.T) {
	for _, wholeMessage := range []bool{false, true} {
		message := chat.NewUserMessage(chat.NewTextPart("question"))
		values := &message.Parts[0].Metadata
		if wholeMessage {
			values = &message.Metadata
		}
		if err := values.Set(contextProvenanceMetadataKey, contextSources{{Kind: contextSourceRecalledMemory, Purpose: contextPurposeInstruction}}); err != nil {
			t.Fatal(err)
		}
		if _, err := originalUserMessage([]chat.Message{message}); err == nil {
			t.Fatal("malformed host provenance was reinterpreted as caller input")
		}
	}
}

func TestInteractionContextReceivesOriginalInputWithoutDiscardingHostParts(t *testing.T) {
	user := chat.NewUserMessage(chat.NewTextPart("question"))
	start := interactionTestStart()
	start.WorkingContext = originalUserWorkingContext(t, user)
	owner := &contextCaptureComposition{t: t}
	providerCalls := 0
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		providerCalls++
		var requestText strings.Builder
		for _, message := range request.Messages {
			requestText.WriteString(message.Text())
		}
		if !strings.Contains(requestText.String(), "hook context") || !strings.Contains(requestText.String(), "recalled fact") {
			t.Error("query projection removed context from the provider request")
		}
		return interactionTextResponse("done"), nil
	}), InteractionExecutorConfig{CallComposition: owner})
	runInteractionHarness(t.Context(), t, executor, start, nil)
	if providerCalls != 1 || len(owner.userInputs) != 1 || owner.userInputs[0] == nil || !reflect.DeepEqual(*owner.userInputs[0], user) {
		t.Fatalf("contributor query contains host-authored text: calls=%d inputs=%+v", providerCalls, owner.userInputs)
	}
}

func originalUserWorkingContext(t *testing.T, user chat.Message) []chat.Message {
	t.Helper()
	hooks := []hookdata.Hook{{Event: hookdata.UserPromptSubmit, Inject: "hook context"}}
	composer := newTestWorkingContextComposer(t, WorkingContextConfig{
		Hooks: provenanceHookResolver{bound: apphooks.NewBound(hooks, func(context.Context, hookprovider.Evaluation) (hookprovider.Advice, error) {
			return hookprovider.Advice{Decision: hookdata.CommandDecision{Verdict: hookdata.CommandAllow, InjectContext: "hook context"}}, nil
		}, nil, nil)},
		AgentMemorySearch: &fakeAgentMemorySearcher{items: []MemoryPromptItem{testMemoryPromptItem(MemoryPromptItem{ID: testAgentMemoryItemID(t, '2'), Content: "recalled fact"})}},
	})
	cwd := t.TempDir()
	messages, err := composer.ComposeWorkingContext(t.Context(), runs.WorkingContextInput{
		SessionID: "session_1", CWD: cwd, WorkspaceCWD: cwd, PromptText: user.Text(), Seed: []chat.Message{user},
	})
	if err != nil {
		t.Fatal(err)
	}
	return messages
}
