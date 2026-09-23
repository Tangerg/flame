package agentexec

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/scope/core/chat"
)

func TestFilesystemGuardsPublishRefusalWithoutPostExecutionHooks(t *testing.T) {
	for _, test := range []struct {
		name   string
		path   string
		reason string
	}{
		{name: "protected directory", path: ".git/config", reason: "protected"},
		{name: "unread file", path: "note.txt", reason: "must read"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, test.path)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			built, err := toolset.Build(t.Context(), toolset.BuildConfig{Lifetime: t.Context(), DefaultCWD: root, UserHome: t.TempDir()})
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
			arguments, err := json.Marshal(map[string]string{"path": test.path, "old_string": "original", "new_string": "changed"})
			if err != nil {
				t.Fatal(err)
			}
			var committed *chat.ToolResult
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				if !hasToolMessage(request.Messages) {
					return interactionToolResponse(chat.ToolCall{ID: "edit", Name: "edit", Arguments: string(arguments)}, 1, 1), nil
				}
				for _, message := range request.Messages {
					for _, part := range message.Parts {
						if part.ToolResult != nil && !reflect.DeepEqual(part.ToolResult, committed) {
							return nil, errors.New("filesystem refusal differs from durable result")
						}
					}
				}
				return interactionTextResponse("request remains blocked"), nil
			})
			hooks := new(feedbackHooks)
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver: built.Resolver, ToolInterpreter: testInteractionToolInterpreter{},
				ToolAuthorizer: allowInteractionTools{}, ToolHooks: hooks,
			})
			start := interactionTestStart()
			start.CWD = root
			events := runInteractionHarnessWithCommit(t, executor, start, func(fact runs.ExecutionFact) error {
				if batch, ok := fact.(runs.ToolResultsCommitted); ok && len(batch.Results) == 1 {
					committed = batch.Results[0].ModelResult
				}
				return nil
			})
			finished := payloadsOf[runs.ToolCallFinished](events)
			if committed == nil || !committed.IsError || len(finished) != 1 || finished[0].Failure == nil || finished[0].Failure.Kind != domaintool.FailureDenied {
				t.Errorf("filesystem refusal was not classified: model=%+v product=%+v", committed, finished)
			} else if text, _ := committed.Output.Text(); !strings.Contains(text, test.reason) {
				t.Errorf("refusal = %q, want %q", text, test.reason)
			}
			ends := payloadsOf[runs.SegmentEnded](events)
			if hooks.after != 0 || len(ends) != 1 || ends[0].Reason != run.OutcomeCompleted || len(unresolvedTerminals(events)) != 0 {
				t.Errorf("refusal lifecycle: hooks=%d ends=%+v", hooks.after, ends)
			}
			if content, err := os.ReadFile(path); err != nil || string(content) != "original\n" {
				t.Errorf("refused edit changed file: %q, %v", content, err)
			}
		})
	}
}
