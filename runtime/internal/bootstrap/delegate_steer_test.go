package bootstrap

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

// TestSteeringARootMidDelegationLandsAfterTheChildResult steers a root Run while
// its delegated child is still executing.
//
// Steering is addressed to a tree, and the tree's root is the only member that
// owns the conversation the guidance belongs to — runs.steer refuses a child
// outright. So a steer submitted while the root is parked on a delegate Tool
// has to survive the wait and arrive at the root's next model request, behind
// the Tool result that request was made to carry. Landing it earlier would put
// a User turn between a Tool call and its result, which is a model context no
// provider is required to accept.
func TestSteeringARootMidDelegationLandsAfterTheChildResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	childEntered := make(chan struct{}, 1)
	releaseChild := make(chan struct{})
	var mu sync.Mutex
	var parentContinuation []chat.Message

	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		delegated, hasResult := false, false
		for _, message := range request.Messages {
			hasResult = hasResult || message.Role == chat.RoleTool
			if message.Role == chat.RoleUser && strings.Contains(message.Text(), "child work") {
				delegated = true
			}
		}
		meta := &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}}
		switch {
		case delegated:
			childEntered <- struct{}{}
			select {
			case <-releaseChild:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			done := chat.NewAssistantMessage(chat.NewTextPart("child done"))
			return chat.NewResponse(&chat.Output{Message: &done, FinishReason: chat.FinishReasonStop}, meta)
		case hasResult:
			mu.Lock()
			parentContinuation = append([]chat.Message(nil), request.Messages...)
			mu.Unlock()
			stop := chat.NewAssistantMessage(chat.NewTextPart("parent done"))
			return chat.NewResponse(&chat.Output{Message: &stop, FinishReason: chat.FinishReasonStop}, meta)
		default:
			call := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
				ID: "delegate_child", Name: "delegate_task",
				Arguments: `{"summary":"child","instructions":"child work"}`,
			}))
			return chat.NewResponse(&chat.Output{Message: &call, FinishReason: chat.FinishReasonToolCalls}, meta)
		}
	})}

	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientCapabilities: &protocol.ClientCapabilities{
			Features: map[string]protocol.FeaturePreference{protocol.FeatureSubagents: {Enabled: true}},
		},
	})
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "steer mid delegation",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "delegate one child"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	collected := collectRunEvents(events)
	select {
	case <-childEntered:
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the delegated child never reached its provider call")
	}

	const guidance = "prefer the shorter approach"
	root, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	// Accepted while the root is parked on the delegate Tool: a tree with work in
	// flight is exactly when a user reaches for steering.
	if _, steerErr := api.SteerRun(ctx, protocol.SteerRunRequest{
		RunID:             started.RunID,
		ExpectedSegmentID: root.ActiveSegmentID,
		Input:             []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: guidance}},
	}); steerErr != nil {
		t.Fatalf("steer the root mid-delegation: %v", steerErr)
	}

	close(releaseChild)
	observed := waitForRunEvents(t, collected, "steered delegation tree")
	waitForProtocolRunTerminal(t, ctx, api, started.RunID)

	mu.Lock()
	continuation := parentContinuation
	mu.Unlock()
	if len(continuation) == 0 {
		t.Fatal("the root never made a model request carrying the child's result")
	}
	toolResultAt, steerAt := -1, -1
	for index, message := range continuation {
		if message.Role == chat.RoleTool {
			toolResultAt = index
		}
		if message.Role == chat.RoleUser && strings.Contains(message.Text(), guidance) {
			steerAt = index
		}
	}
	if steerAt < 0 {
		t.Fatalf("the steer never reached the root's continuation: %+v", continuation)
	}
	if toolResultAt < 0 || steerAt < toolResultAt {
		t.Fatalf("steer at %d, Tool result at %d — guidance must follow the result it waited for",
			steerAt, toolResultAt)
	}

	// The child is not separately steerable: the conversation the guidance joins
	// belongs to the root.
	childRunID := ""
	for _, event := range observed {
		if event.RunID != started.RunID {
			childRunID = event.RunID
		}
	}
	if childRunID == "" {
		t.Fatalf("no child Run appeared in %d events", len(observed))
	}
	_, steerErr := api.SteerRun(ctx, protocol.SteerRunRequest{
		RunID: childRunID, ExpectedSegmentID: "seg_child",
		Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: guidance}},
	})
	// By name, not by any failure: a wrong segment id or a finished run would
	// also refuse, and neither would say that the child is the wrong addressee.
	if !errors.Is(steerErr, protocol.ErrRunNotRoot) {
		t.Fatalf("steering a child Run = %v, want run_not_root", steerErr)
	}
}
