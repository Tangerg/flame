package agentexec

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
)

func TestDelegateDiagnosticIsPublishedWithoutFlameReformatting(t *testing.T) {
	var observed *chat.ToolResult
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if !hasToolMessage(request.Messages) {
			return interactionToolResponse(chat.ToolCall{ID: "failed_delegate", Name: "delegate_task", Arguments: `{"summary":"worker","instructions":"run worker"}`}, 1, 1), nil
		}
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil {
					observed = new(part.ToolResult.Clone())
				}
			}
		}
		return interactionUsageTextResponse("failure understood", 1, 1), nil
	})
	fixture := startDelegateTreeWithCompactor(t, model, "delegate", rejectingChildCompactor{})
	result := fixture.finish(t, 2)
	rootID := result.rootRunID(t)
	rootCompleted, childFailed := false, false
	for _, event := range result.events {
		if terminal, ok := event.Payload.(runs.SegmentFinished); ok {
			if terminal.Run.ID() == rootID {
				rootCompleted = terminal.Run.State() == run.Completed
			} else {
				childFailed = terminal.Run.State() == run.Failed
			}
		}
	}
	if !rootCompleted || !childFailed || observed == nil || !observed.IsError {
		t.Fatalf("known child failure was not adopted: root=%t child=%t result=%+v", rootCompleted, childFailed, observed)
	}
	text, _ := observed.Output.Text()
	if len(text) < 2048 || !utf8.ValidString(text) || strings.Contains(text, diagnosticEllipsis) {
		t.Fatalf("delegate diagnostic changed: %q", text)
	}
	fixture.projection.mu.Lock()
	defer fixture.projection.mu.Unlock()
	var committed *chat.ToolResult
	for _, message := range fixture.projection.conversation {
		for _, part := range message.Parts {
			if part.ToolResult != nil && part.ToolResult.ID == "failed_delegate" {
				committed = part.ToolResult
			}
		}
	}
	if !reflect.DeepEqual(observed, committed) {
		t.Fatalf("model and durable result disagree: model=%+v committed=%+v", observed, committed)
	}
}

type rejectingChildCompactor struct{}

func (rejectingChildCompactor) CompactModelContext(_ context.Context, request ModelContextCompaction) (ModelContextCompactionResult, error) {
	if !request.Durable() {
		return ModelContextCompactionResult{}, errors.New(strings.Repeat("worker diagnostic 界", 400))
	}
	return NewModelContextCompactionResult(request.Candidate(), "", len(request.Candidate()), 100)
}
