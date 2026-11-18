package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
)

func TestResumeInputsReachTheNextRootModelAfterConsecutiveQuestions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	const task = "ask three questions in sequence"
	const questions = 3
	inputs := []string{"extra context from first answer", "extra context from second answer"}
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		var child, delegateFinished bool
		var answered int
		var received []string
		for _, message := range request.Messages {
			if message.Role == chat.RoleUser {
				child = child || strings.Contains(message.Text(), task)
				if slices.Contains(inputs, message.Text()) {
					received = append(received, message.Text())
				}
			}
			for _, part := range message.Parts {
				if part.ToolResult == nil {
					continue
				}
				switch part.ToolResult.Name {
				case "ask_user":
					answered++
				case "delegate_task":
					delegateFinished = true
				}
			}
		}
		var call chat.ToolCall
		switch {
		case child && answered == questions:
			return completedTextResponse("All questions were answered."), nil
		case child:
			call = chat.ToolCall{ID: fmt.Sprintf("question_%d", answered), Name: "ask_user",
				Arguments: fmt.Sprintf(`{"questions":[{"question":"Answer question %d"}]}`, answered+1)}
		case delegateFinished:
			if !slices.Equal(received, inputs) {
				return nil, fmt.Errorf("root model received Resume inputs %v", received)
			}
			return completedTextResponse("Both inputs arrived in order."), nil
		default:
			call = chat.ToolCall{ID: "delegate_questions", Name: "delegate_task",
				Arguments: fmt.Sprintf(`{"summary":"Three questions","instructions":%q}`, task)}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls},
			&chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	})}
	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{
		ProtocolVersion: protocol.Version,
		ClientCapabilities: &protocol.ClientCapabilities{
			Features:       map[string]protocol.FeaturePreference{protocol.FeatureChildRuns: {Enabled: true}},
			InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
		},
	})
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "Consecutive questions",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Delegate the questions."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "first question")

	for index := range questions {
		if index == questions-1 {
			if err := host.Close(); err != nil {
				t.Fatal(err)
			}
			host, api = openProtocolRuntime(t, model)
		}
		pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
		if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
			t.Fatalf("question %d: %+v, %v", index+1, pending, err)
		}
		request := protocol.ResumeRunRequest{
			RunID: started.RunID,
			Responses: []protocol.InterruptResponse{{
				ItemID:   pending.Data[0].Interrupts[0].ItemID,
				Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"yes"}}},
			}},
		}
		if index < len(inputs) {
			request.Input = []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: inputs[index]}}
		}
		_, resumed, err := api.ResumeRun(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		waitForRunEvents(t, collectRunEvents(resumed), "answered question")
	}
	waitForProtocolRunTerminal(t, ctx, api, started.RunID)
	finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
		var outcome []byte
		if finished != nil {
			outcome, _ = json.Marshal(finished.Outcome)
		}
		t.Fatalf("Run did not deliver both inputs: outcome=%s, %v", outcome, err)
	}

	exported, err := api.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: session.ID})
	if err != nil || exported.Artifact == nil {
		t.Fatalf("expected Session artifact: %v", err)
	}
	var applied []string
	for _, encoded := range exported.Artifact.Messages {
		var message chat.Message
		if err := json.Unmarshal(encoded, &message); err != nil {
			t.Fatal(err)
		}
		if message.Role == chat.RoleUser && slices.Contains(inputs, message.Text()) {
			applied = append(applied, message.Text())
		}
	}
	if !slices.Equal(applied, inputs) {
		t.Fatalf("durable conversation retained Resume inputs %v", applied)
	}
}
