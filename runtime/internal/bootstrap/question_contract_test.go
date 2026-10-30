package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
)

func TestQuestionContractRejectsTheSameFieldAndPersistsExplicitSkips(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil {
					if len(part.ToolResult.Output.Content) != 1 || !strings.Contains(part.ToolResult.Output.Content[0].Text, "custom, one") {
						return nil, errors.New("question result changed the literal custom value")
					}
					message := chat.NewAssistantMessage(chat.NewTextPart("completed"))
					return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, &chat.ResponseMetadata{Model: "claude-test"})
				}
			}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
			ID: "contract_question", Name: "ask_user", Arguments: `{"questions":[{"question":"Context"},{"question":"Platform","options":[{"label":"linux"},{"label":"darwin"}]},{"question":"Checks","options":[{"label":"unit"},{"label":"integration"}],"multi_select":true}]}`,
		}))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, &chat.ResponseMetadata{Model: "claude-test"})
	})
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	host, api := buildProtocolRuntime(t, protocolRuntimeConfig(t, stores, model), home)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	options := acceptedCommandOptions()
	options.RequestMeta.ClientCapabilities.InterruptTypes = []protocol.InterruptType{protocol.InterruptQuestion}
	discovery, err := api.Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	options.IdempotencyNamespace = discovery.Capabilities.Limits.Idempotency.Namespace
	ctx := delivery.WithRequestMeta(t.Context(), options.RequestMeta)
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "ask for the contract"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range events {
		if err != nil {
			t.Fatal(err)
		}
	}
	pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	interrupt := pending.Data[0].Interrupts[0]
	definition := interrupt.Payload.Question
	fields := make([]questioncontract.Field, len(definition.Fields))
	for i, field := range definition.Fields {
		labels := make([]string, len(field.Options))
		for j, option := range field.Options {
			labels[j] = option.Label
		}
		fields[i] = questioncontract.Field{Kind: questioncontract.Kind(field.Type), Options: labels, Multiple: field.Multiple, AllowCustom: field.AllowCustom}
	}
	contract, err := questioncontract.New(fields)
	if err != nil {
		t.Fatal(err)
	}
	requestFor := func(answers [][]string) protocol.ResumeRunRequest {
		return protocol.ResumeRunRequest{RunID: started.RunID, Responses: []protocol.InterruptResponse{{ItemID: interrupt.ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: answers}}}}
	}
	for i, answers := range [][][]string{
		nil, {{}}, {{"a", "b"}, {}, {}}, {{" "}, {}, {}},
		{{}, {"linux", "darwin"}, {}}, {{}, {" linux"}, {}},
		{{}, {}, {"one", "two"}}, {{}, {}, {"unit", "unit"}}, {{}, {}, {"custom "}},
	} {
		local := contract.Validate(answers)
		if local == nil {
			t.Fatalf("invalid fixture passed the public contract: %q", answers)
		}
		options.IdempotencyKey = fmt.Sprintf("invalid-question-%d", i)
		result := host.Endpoint().Invoke(ctx, delivery.RunsResume, requestFor(answers), options)
		if result.Failure == nil {
			t.Fatalf("invalid answers reached execution: %q", answers)
		}
		constraint, ok := errors.AsType[*protocol.ConstraintError](result.Failure)
		if !ok || len(constraint.Fields) != 1 {
			t.Fatalf("failure has no single field location: %v", result.Failure)
		}
		want := "responses[0].response.answers"
		if field, ok := errors.AsType[*questioncontract.FieldError](local); ok {
			want += fmt.Sprintf("[%d]", field.Index)
		}
		if constraint.Fields[0].Field != want {
			t.Fatalf("field = %q, public contract wants %q", constraint.Fields[0].Field, want)
		}
	}
	accepted := [][]string{{}, {}, {"unit", "custom, one"}}
	options.IdempotencyKey = "accept-question-skips"
	result := host.Endpoint().Invoke(ctx, delivery.RunsResume, requestFor(accepted), options)
	if result.Failure != nil {
		t.Fatal(result.Failure)
	}
	completed := false
	assertQuestion := func(question *protocol.Question) {
		t.Helper()
		if question == nil || question.Answers == nil || len(question.Answers) != 3 || len(question.Answers[0]) != 0 || len(question.Answers[1]) != 0 || !slices.Equal(question.Answers[2], accepted[2]) {
			t.Fatalf("accepted question = %+v", question)
		}
	}
	for value, err := range result.Events {
		if err != nil {
			t.Fatal(err)
		}
		event, ok := value.(protocol.RunEvent)
		if !ok {
			t.Fatalf("resume event type = %T", value)
		}
		if event.Event.Type == protocol.StreamItemCompleted && event.Event.Item != nil && event.Event.Item.Question != nil && event.Event.Item.Question.Answers != nil {
			assertQuestion(event.Event.Item.Question)
			completed = true
		}
	}
	if !completed {
		t.Fatal("accepted skip did not appear in the completion stream")
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	second, cold := buildProtocolRuntime(t, protocolRuntimeConfig(t, reopened, model), home)
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	})
	assertItems := func(api *delivery.Handler, scope protocol.ItemListScope) {
		t.Helper()
		items, err := api.ListItems(ctx, protocol.ListItemsRequest{Scope: scope})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range items.Data {
			if item.Question != nil {
				assertQuestion(item.Question)
				found = true
			}
		}
		if !found {
			t.Fatal("accepted question is absent from history")
		}
	}
	assertItems(cold, protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: started.RunID})
	exported, err := cold.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: session.ID})
	if err != nil || exported.Artifact == nil {
		t.Fatalf("export = %+v, %v", exported, err)
	}
	imported, err := cold.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: *exported.Artifact})
	if err != nil {
		t.Fatal(err)
	}
	assertItems(cold, protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: imported.Session.ID})
}
