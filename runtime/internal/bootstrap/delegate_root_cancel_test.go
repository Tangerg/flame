package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/invocation"
)

func TestCancelingTheRootMidDelegationSettlesEveryUnconfirmedChild(t *testing.T) {
	testCancelDelegatedTree(t, false)
}

func TestConfirmedRootCancellationSettlesEveryDelegateFirst(t *testing.T) {
	testCancelDelegatedTree(t, true)
}

func testCancelDelegatedTree(t *testing.T, confirmed bool) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	childEntered := make(chan string, 2)
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		delegatedChild, hasResult := "", false
		for _, message := range request.Messages {
			hasResult = hasResult || message.Role == chat.RoleTool
			if message.Role != chat.RoleUser {
				continue
			}
			for _, name := range []string{"child work A", "child work B"} {
				if strings.Contains(message.Text(), name) {
					delegatedChild = name
				}
			}
		}
		meta := &chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}}
		switch {
		case delegatedChild != "":
			childEntered <- delegatedChild
			<-ctx.Done()
			if confirmed {
				return nil, &invocation.Failure{Kind: invocation.FailureNotExecuted, Cause: ctx.Err()}
			}
			return nil, ctx.Err()
		case hasResult:
			stop := chat.NewAssistantMessage(chat.NewTextPart("parent done"))
			return chat.NewResponse(&chat.Output{Message: &stop, FinishReason: chat.FinishReasonStop}, meta)
		default:
			call := chat.NewAssistantMessage(
				chat.NewToolCallPart(chat.ToolCall{
					ID: "delegate_a", Name: "delegate_task",
					Arguments: `{"summary":"A","instructions":"child work A"}`,
				}),
				chat.NewToolCallPart(chat.ToolCall{
					ID: "delegate_b", Name: "delegate_task",
					Arguments: `{"summary":"B","instructions":"child work B"}`,
				}),
			)
			return chat.NewResponse(&chat.Output{Message: &call, FinishReason: chat.FinishReasonToolCalls}, meta)
		}
	})}

	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{
		ProtocolVersion: protocol.Version,
		ClientCapabilities: &protocol.ClientCapabilities{
			Features: map[string]protocol.FeaturePreference{protocol.FeatureChildRuns: {Enabled: true}},
		},
	})
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "root cancel mid delegation",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "delegate both children"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	collected := collectRunEvents(events)
	entered := make(map[string]bool, 2)
	deadline := time.NewTimer(lifecycleWaitBudget(t))
	defer deadline.Stop()
	for len(entered) < 2 {
		select {
		case name := <-childEntered:
			entered[name] = true
		case <-deadline.C:
			t.Fatalf("only %d delegated children reached a provider call", len(entered))
		}
	}

	request := protocol.CancelRunRequest{RunID: started.RunID, Reason: "stop the whole tree"}
	canceled, err := api.CancelRun(ctx, request)
	if confirmed {
		if err != nil || canceled == nil || canceled.Run.Outcome == nil || canceled.Run.Outcome.Detail != request.Reason {
			t.Fatalf("the canceled delegation did not settle: %+v, %v", canceled, err)
		}
	} else if canceled != nil || !errors.Is(err, protocol.ErrRunFinished) {
		t.Fatalf("unconfirmed delegation became a confirmed cancellation: %+v, %v", canceled, err)
	}
	observed := waitForRunEvents(t, collected, "canceled delegation tree")

	root, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed {
		if root.Outcome == nil || root.Outcome.Type != protocol.OutcomeCanceled || root.Outcome.Detail != request.Reason {
			t.Fatalf("the root did not retain its confirmed cancellation: %+v", root.Outcome)
		}
	} else if root.Outcome == nil || root.Outcome.Type != protocol.OutcomeLost || root.Outcome.Loss == nil || root.Outcome.Loss.Reason != protocol.LossUnresolvedEffects {
		t.Fatalf("root cancellation lost its original uncertainty: %+v", root.Outcome)
	}

	childRunIDs := map[string]struct{}{}
	for _, event := range observed {
		if event.RunID != started.RunID {
			childRunIDs[event.RunID] = struct{}{}
		}
	}
	if len(childRunIDs) != 2 {
		t.Fatalf("child Runs in the stream = %d, want both", len(childRunIDs))
	}
	for childRunID := range childRunIDs {
		child, childErr := api.GetRun(ctx, protocol.GetRunRequest{RunID: childRunID})
		if childErr != nil {
			t.Fatal(childErr)
		}
		if confirmed {
			if child.Outcome == nil || child.Outcome.Type != protocol.OutcomeCanceled || child.Outcome.Detail != request.Reason ||
				child.FinishedAt.After(root.FinishedAt) {
				t.Fatalf("child %s did not finish its cancellation before the root: %+v", childRunID, child.Outcome)
			}
		} else if child.Outcome == nil || child.Outcome.Type != protocol.OutcomeLost || len(child.Outcome.UnresolvedEffects) != 1 {
			t.Fatalf("child %s cancellation lost its original uncertainty: %+v", childRunID, child.Outcome)
		}
	}

	items, err := api.ListItems(ctx, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{
			Type: protocol.ItemScopeRun, SessionID: session.ID, RunID: started.RunID,
			IncludeDescendants: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	delegateSettled := 0
	for _, item := range items.Data {
		if item.Status == protocol.ItemStatusRunning {
			t.Fatalf("item %q (%s) is still running after the tree was canceled", item.ID, item.Type)
		}
		if item.Tool != nil && item.Tool.Name == "delegate_task" {
			delegateSettled++
		}
	}
	if delegateSettled != 2 {
		t.Fatalf("settled delegate_task Items = %d among %d items, want both",
			delegateSettled, len(items.Data))
	}

	if _, _, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "after the cancel"}},
	}); err != nil {
		t.Fatalf("start after cancel = %v, want the Session slot freed", err)
	}
}
