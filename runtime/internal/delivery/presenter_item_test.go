package delivery

import (
	json "encoding/json/v2"
	"strings"
	"testing"
	"time"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// Only built-in tools carry a declared presentation; plugin and MCP tools carry
// none, whatever their names suggest.
func TestPresentToolInvocationProjectsTheBuiltinPresentation(t *testing.T) {
	for _, test := range []struct {
		name     string
		category builtincontract.ToolCategory
		shape    extension.ResultShape
	}{
		{string(builtincontract.Shell), builtincontract.CategoryShell, extension.ResultShapeCommand},
		{string(builtincontract.ApplyPatch), builtincontract.CategoryEdit, extension.ResultShapePatch},
		{string(builtincontract.Grep), builtincontract.CategorySearch, extension.ResultShapeSearch},
		{string(builtincontract.Read), builtincontract.CategoryRead, ""},
		{"web_search", "", ""},
		{"search_memory", "", ""},
	} {
		presented := presentTool(transcript.ToolInvocation{Name: test.name})
		if presented.Category != test.category || presented.ResultShape != test.shape {
			t.Errorf("%s presentation = (%q, %q), want (%q, %q)", test.name, presented.Category, presented.ResultShape, test.category, test.shape)
		}
	}
}

func TestPresentToolCallTiming(t *testing.T) {
	startedAt := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	running := presentItem(testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_1", RunID: "run_1", Kind: transcript.ToolCall,
		Status: transcript.ItemRunning, OccurredAt: startedAt,
		Tool: &transcript.ToolInvocation{Name: "shell"},
	}))
	if !running.CreatedAt.IsZero() || running.StartedAt != startedAt || !running.FinishedAt.IsZero() || running.DurationMillis != nil {
		t.Fatalf("running timing = started %s finished %s duration %v", running.StartedAt, running.FinishedAt, running.DurationMillis)
	}

	finishedAt := startedAt.Add(10 * time.Second)
	executionDuration := 1250 * time.Millisecond
	completed := presentItem(testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_1", RunID: "run_1", Kind: transcript.ToolCall,
		Status: transcript.ItemCompleted, OccurredAt: startedAt, FinishedAt: finishedAt,
		ExecutionDuration: &executionDuration,
		Tool:              &transcript.ToolInvocation{Name: "shell"},
	}))
	if !completed.CreatedAt.IsZero() || completed.StartedAt != startedAt || completed.FinishedAt != finishedAt ||
		completed.DurationMillis == nil || *completed.DurationMillis != 1250 {
		t.Fatalf("completed timing = started %s finished %s duration %v", completed.StartedAt, completed.FinishedAt, completed.DurationMillis)
	}
	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatal(err)
	}
	if body := string(encoded); strings.Contains(body, `"createdAt"`) || !strings.Contains(body, `"startedAt"`) {
		t.Fatalf("tool-call wire timing is not exclusive: %s", body)
	}

	message := presentItem(testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_2", RunID: "run_1", Kind: transcript.AgentMessage,
		Status: transcript.ItemCompleted, OccurredAt: startedAt,
	}))
	encoded, err = json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if body := string(encoded); !strings.Contains(body, `"createdAt"`) ||
		!strings.Contains(body, `"phase":"finalAnswer"`) || strings.Contains(body, `"startedAt"`) {
		t.Fatalf("message wire timing is not exclusive: %s", body)
	}
}
