package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

// TestCancelingTheRootMidDelegationCancelsTheWholeTree cancels a root Run while
// both of its delegated children are inside a provider call.
//
// The executor ends a canceled subtree by cancelling the children's work and
// publishing the parent's terminal first, so the root's boundary arrives at the
// projection ahead of the children it just ended. Committing it there would
// close the tree over a live descendant row, which the durable Run projection
// cannot represent — publication order is the only thing enforcing it.
//
// What the user must get is one cancellation, not a defect: the root canceled
// with the reason they gave, every child canceled rather than reported lost,
// the parent's delegate_task Items settled instead of spinning forever, and the
// Session's single-writer slot back.
//
// Two children, not one, because the boundary must wait for the last of them.
// With a single child the difference between "release when a child finishes"
// and "release when none is left" cannot be observed.
func TestCancelingTheRootMidDelegationCancelsTheWholeTree(t *testing.T) {
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
			// Hold the child inside the provider call so the cancellation lands
			// on a live Effect rather than between steps.
			childEntered <- delegatedChild
			<-ctx.Done()
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
	for len(entered) < 2 {
		select {
		case name := <-childEntered:
			entered[name] = true
		case <-time.After(lifecycleWaitBudget):
			t.Fatalf("only %d delegated children reached a provider call", len(entered))
		}
	}
	// Let the parent settle into waiting on the delegate Tool, so the cancel
	// meets a tree that is genuinely mid-delegation.
	time.Sleep(200 * time.Millisecond)

	canceled, err := api.CancelRun(ctx, protocol.CancelRunRequest{
		RunID: started.RunID, Reason: "stop the whole tree",
	})
	if err != nil {
		t.Fatalf("cancel root mid-delegation: %v", err)
	}
	if canceled.Run.Outcome == nil || canceled.Run.Outcome.Type != protocol.OutcomeCanceled {
		t.Fatalf("cancel reported %+v, want a canceled root", canceled.Run.Outcome)
	}
	observed := waitForRunEvents(t, collected, "canceled delegation tree")

	root, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if root.Outcome == nil || root.Outcome.Type != protocol.OutcomeCanceled ||
		root.Outcome.Detail != "stop the whole tree" {
		t.Fatalf("root after cancel = %+v, want canceled with the caller's reason", root.Outcome)
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
		// Canceled, not lost, and never left non-terminal: each child's own
		// terminal fact is what settles it, which is only possible because the
		// root's boundary waited for the last of them.
		if child.Outcome == nil || child.Outcome.Type != protocol.OutcomeCanceled {
			t.Fatalf("child %s after root cancel = %+v, want canceled", childRunID, child.Outcome)
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
