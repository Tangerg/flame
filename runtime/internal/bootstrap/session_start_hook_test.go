package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestSessionStartHookFollowsTheFirstAdmittedRun(t *testing.T) {
	const initialization = "Initialize the workspace context once"
	const refusal = "fixture refuses the first prompt"
	const firstPrompt = "The first admitted prompt"
	const nextPrompt = "The later prompt after restart"
	const initializationRule = `{"event":"SessionStart","inject":"` + initialization + `"}`
	const refusalRule = `{"event":"UserPromptSubmit","command":"printf '%s' '{\"decision\":\"deny\",\"reason\":\"` + refusal + `\"}'"}`
	inputs := make(chan string, 2)
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		for index := len(request.Messages) - 1; index >= 0; index-- {
			message := request.Messages[index]
			prompt := message.Text()
			if message.Role == chat.RoleUser && (strings.Contains(prompt, firstPrompt) || strings.Contains(prompt, nextPrompt)) {
				select {
				case inputs <- prompt:
					return completedTextResponse("Workspace inspected"), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		}
		return nil, fmt.Errorf("model request did not contain a prompt")
	})}
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.modelProviders = testChatResolver(client)
	writeHookSource(t, cfg.userHome, `{"hooks":[`+initializationRule+`,`+refusalRule+`]}`)
	runtime := poMCPOpen(t, cfg)
	activateHookProvider(t, runtime.api)
	session := runtime.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Session startup hooks"}, "create-hook-session").(*protocol.Session)
	start := func(key, prompt string) protocol.OperationResult {
		return runtime.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{
			SessionID: session.ID,
			Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: prompt}},
		}, protocol.CallMetadata{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}})
	}
	if rejected := start("refused-opening", "The original prompt"); rejected.Failure == nil || !strings.Contains(fmt.Sprint(rejected.Failure.Unwrap()), refusal) {
		t.Fatalf("first opening did not reach the prompt hook: %v", rejected.Failure)
	}
	page := runtime.must(delivery.RunsList, protocol.ListRunsRequest{SessionID: session.ID}, "").(*protocol.Page[protocol.RunRef])
	if len(page.Data) != 0 {
		t.Fatalf("a rejected prompt opened a Run: %+v", page.Data)
	}

	writeHookSource(t, cfg.userHome, `{"hooks":[`+initializationRule+`]}`)
	first := admittedModelInput(t, start("first-admitted-opening", firstPrompt), inputs)
	if !strings.Contains(first, initialization) || !strings.Contains(first, firstPrompt) {
		t.Fatalf("first admitted model input = %q, want the startup hook and prompt", first)
	}
	runtime = reopenPluginRuntime(t, runtime, &cfg)
	if later := admittedModelInput(t, start("opening-after-restart", nextPrompt), inputs); later != nextPrompt {
		t.Fatalf("later model input = %q, want %q", later, nextPrompt)
	}
}

func admittedModelInput(t *testing.T, result protocol.OperationResult, inputs <-chan string) string {
	t.Helper()
	if result.Failure != nil {
		t.Fatal(result.Failure)
	}
	for _, err := range result.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	select {
	case called := <-inputs:
		return called
	default:
		t.Fatal("Run did not reach the model")
		return ""
	}
}
