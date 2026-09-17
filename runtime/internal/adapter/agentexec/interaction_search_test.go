package agentexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
)

func TestInteractionSearchFailureCommitsFeedbackAndAllowsCorrection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "selected.go"), []byte("package selected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{
		Lifetime: t.Context(), DefaultCWD: root, UserHome: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, close := range built.Closers {
			if err := close(); err != nil {
				t.Error(err)
			}
		}
	})
	var committed []*chat.ToolResult
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		var feedback []*chat.ToolResult
		for _, message := range request.Messages {
			for _, part := range message.Parts {
				if part.ToolResult != nil {
					feedback = append(feedback, part.ToolResult)
				}
			}
		}
		if !reflect.DeepEqual(feedback, committed) {
			return nil, errors.New("search feedback differs from committed results")
		}
		switch len(feedback) {
		case 0:
			return interactionToolResponse(chat.ToolCall{ID: "bad_search", Name: "grep", Arguments: `{"path":"selected.go","pattern":"["}`}, 1, 1), nil
		case 1:
			text, _ := feedback[0].Output.Text()
			if !strings.Contains(text, "invalid grep pattern") {
				return nil, errors.New("missing search failure feedback")
			}
			return interactionToolResponse(chat.ToolCall{ID: "corrected_search", Name: "grep", Arguments: `{"path":"selected.go","pattern":"package"}`}, 1, 1), nil
		case 2:
			text, _ := feedback[1].Output.Text()
			if !strings.Contains(text, "package selected") {
				return nil, errors.New("missing selected-file result")
			}
			return interactionUsageTextResponse("recovered", 1, 1), nil
		default:
			return nil, errors.New("unexpected model continuation")
		}
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: built.Resolver, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	start := interactionTestStart()
	start.CWD = root
	events := runInteractionHarnessWithCommit(t, executor, start, func(fact runs.ExecutionFact) error {
		if batch, ok := fact.(runs.ToolResultsCommitted); ok {
			for _, result := range batch.Results {
				committed = append(committed, result.ModelResult)
			}
		}
		return nil
	})
	if len(committed) != 2 {
		t.Fatalf("committed results = %d, want failed and corrected search", len(committed))
	}
	if unknown := unresolvedTerminals(events); len(unknown) != 0 {
		t.Fatalf("search produced unknown Effects: %+v", unknown)
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted {
		t.Fatalf("search Run termination = %+v", ended)
	}
}
