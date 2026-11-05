package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestMarkdownToolCallKeepsRecordAndResultWithoutChangingSource(t *testing.T) {
	result := "first line\n````\n# injected heading\n<script>unsafe()</script>\n[open](https://example.invalid)\n```\nlast line"
	item := protocol.ArtifactItem{
		ID: "item-original", RunID: "run-original", Status: protocol.ItemStatusIncomplete, Type: protocol.ItemTypeToolCall,
		StartedAt: time.Unix(10, 0).UTC(), FinishedAt: time.Unix(11, 0).UTC(), DurationMillis: new(int64(735)),
		SafetyClass: protocol.SafetyClassExec, ApprovalDecision: protocol.ApprovalApprove,
		Error: &protocol.ArtifactProblem{Type: protocol.ArtifactProblemToolFailed, Detail: "original failure\n```\n# untrusted", DocURL: "https://example.invalid/help"},
		Tool: &protocol.ToolInvocation{
			Name: "inspect````input", Arguments: map[string]any{"command": "printf 'original'"}, Result: result,
			Source: &protocol.RecordedToolSource{ModelName: "inspect", Ref: protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: "shell"}},
		},
	}
	before, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderSessionMarkdown(protocol.ArtifactSession{ID: "session-original"}, []protocol.ArtifactItem{item}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []string{
		`"id": "item-original"`, `"runId": "run-original"`, `"status": "incomplete"`,
		`"durationMillis": 735`, `"safetyClass": "exec"`, `"approvalDecision": "approve"`,
		`"startedAt": "1970-01-01T00:00:10Z"`, `"finishedAt": "1970-01-01T00:00:11Z"`,
		`"type": "tool_failed"`, `"detail": "original failure\n`, `"docUrl": "https://example.invalid/help"`,
		`"source": {`, `"modelName": "inspect"`, `"name": "inspect`, `"command": "printf 'original'"`,
		"### Result (text)\n\n`````text\n" + result + "\n`````\n",
	} {
		if !strings.Contains(got, evidence) {
			t.Errorf("missing original evidence %q in %s", evidence, got)
		}
	}
	if strings.Count(got, "first line") != 1 || strings.Contains(got, `"result"`) {
		t.Fatal("result was duplicated in the call record")
	}
	after, err := json.Marshal(item)
	if err != nil || string(after) != string(before) {
		t.Fatalf("rendering changed its source: %v", err)
	}
}

func TestMarkdownToolResultsKeepJSONTextEmptyAndAbsentDistinct(t *testing.T) {
	for _, test := range []struct {
		name   string
		result any
		want   string
	}{
		{"structured", jsontext.Value(`{"revision":9007199254740993,"decimal":1.2300,"exponent":1e+100,"negativeZero":-0}`), `"revision": 9007199254740993`},
		{"null", jsontext.Value(`null`), "### Result (JSON)\n\n```json\nnull\n```"},
		{"empty object", map[string]any{}, "### Result (JSON)\n\n```json\n{}\n```"},
		{"empty array", []any{}, "### Result (JSON)\n\n```json\n[]\n```"},
		{"empty text", "", "### Result (text)\n\n```text\n\n```"},
		{"JSON-looking text", `{"result":"still text"}`, "### Result (text)\n\n```text\n{\"result\":\"still text\"}\n```"},
		{"absent", nil, "No result was recorded."},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := protocol.ArtifactItem{Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusCompleted, Tool: &protocol.ToolInvocation{Name: "inspect", Arguments: map[string]any{}, Result: test.result}}
			got, err := renderSessionMarkdown(protocol.ArtifactSession{ID: "session"}, []protocol.ArtifactItem{item}, nil)
			if err != nil || !strings.Contains(got, test.want) || !strings.Contains(got, `"status": "completed"`) {
				t.Fatalf("result evidence = %q, %v", got, err)
			}
			if test.name == "structured" {
				for _, literal := range []string{"1.2300", "1e+100", "-0"} {
					if !strings.Contains(got, literal) {
						t.Errorf("numeric literal %q changed", literal)
					}
				}
			}
		})
	}
}

func TestMarkdownKeepsRejectedInputAndRefusesInvalidToolEvidence(t *testing.T) {
	item := protocol.ArtifactItem{Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusIncomplete,
		Tool:  &protocol.ToolInvocation{Name: "inspect", Arguments: map[string]any{}, ArgumentsText: "{broken\n```\n# input"},
		Error: &protocol.ArtifactProblem{Type: protocol.ArtifactProblemDeniedByUser, Detail: "refused"}}
	got, err := renderSessionMarkdown(protocol.ArtifactSession{ID: "session"}, []protocol.ArtifactItem{item}, nil)
	if err != nil || !strings.Contains(got, `"argumentsText": "{broken\n`) || !strings.Contains(got, `"type": "denied_by_user"`) {
		t.Fatalf("rejected input = %q, %v", got, err)
	}
	for _, invocation := range []*protocol.ToolInvocation{
		nil,
		{Name: "invalid-input", Arguments: map[string]any{"unsupported": make(chan int)}},
		{Name: "invalid-result", Arguments: map[string]any{}, Result: make(chan int)},
	} {
		item.Tool = invocation
		if body, err := renderSessionMarkdown(protocol.ArtifactSession{ID: "session"}, []protocol.ArtifactItem{item}, nil); err == nil || body != "" {
			t.Fatalf("invalid tool evidence appeared successful: %q, %v", body, err)
		}
	}
}

func TestMarkdownKeepsModelOnlyTextBesideItsOriginalStructuredResult(t *testing.T) {
	diagnostic := "unique diagnostic not present in result\n````\n# literal model text\n<script>captured()</script>"
	const plain = "complete ordinary result"
	items := []protocol.ArtifactItem{
		{ID: "structured-item", RunID: "original-run", Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusCompleted,
			Tool: &protocol.ToolInvocation{Name: "inspect", Arguments: map[string]any{}, Result: jsontext.Value(`{"revision":7}`)}},
		{ID: "plain-item", RunID: "original-run", Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusCompleted,
			Tool: &protocol.ToolInvocation{Name: "ordinary", Arguments: map[string]any{}, Result: plain}},
	}
	results := []protocol.ArtifactToolResult{
		{ID: "plain-blob", ItemID: "plain-item", Body: plain},
		{ID: "model-blob", ItemID: "structured-item", ModelOnly: true, Body: diagnostic},
	}
	beforeItems, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	beforeResults, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderSessionMarkdown(protocol.ArtifactSession{ID: "session"}, items, results)
	if err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []string{
		"### Result (JSON)\n\n```json\n{\n  \"revision\": 7\n}\n```",
		"### Model-visible text evidence\n\n`````text\n" + diagnostic + "\n`````",
	} {
		if !strings.Contains(got, evidence) {
			t.Errorf("original evidence missing from %s", got)
		}
	}
	if strings.Index(got, diagnostic) > strings.Index(got, `"id": "plain-item"`) || strings.Count(got, diagnostic) != 1 || strings.Count(got, plain) != 1 {
		t.Fatal("model-only text changed its Item binding or duplicated an expanded ordinary result")
	}
	afterItems, err := json.Marshal(items)
	if err != nil || string(afterItems) != string(beforeItems) {
		t.Fatalf("rendering changed original Items: %v", err)
	}
	afterResults, err := json.Marshal(results)
	if err != nil || string(afterResults) != string(beforeResults) {
		t.Fatalf("rendering changed original model-text records: %v", err)
	}
}

func TestMarkdownRefusesAmbiguousOrUnmatchedModelOnlyText(t *testing.T) {
	items := []protocol.ArtifactItem{
		{ID: "tool-item", Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusCompleted,
			Tool: &protocol.ToolInvocation{Name: "inspect", Arguments: map[string]any{}, Result: jsontext.Value(`{"revision":7}`)}},
		{ID: "user-item", Type: protocol.ItemTypeUserMessage, Text: "original user input"},
	}
	for _, fixture := range []struct {
		name    string
		results []protocol.ArtifactToolResult
		want    string
	}{
		{"duplicate", []protocol.ArtifactToolResult{
			{ID: "first", ItemID: "tool-item", ModelOnly: true, Body: "first original text"},
			{ID: "second", ItemID: "tool-item", ModelOnly: true, Body: "second original text"},
		}, "duplicate model-visible text evidence"},
		{"missing item", []protocol.ArtifactToolResult{{ID: "missing", ItemID: "absent-item", ModelOnly: true, Body: "original text"}}, "no matching tool item"},
		{"non-tool item", []protocol.ArtifactToolResult{{ID: "wrong-kind", ItemID: "user-item", ModelOnly: true, Body: "original text"}}, "no matching tool item"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body, err := renderSessionMarkdown(protocol.ArtifactSession{ID: "session"}, items, fixture.results)
			if err == nil || !strings.Contains(err.Error(), fixture.want) || body != "" {
				t.Fatalf("invalid model-text binding appeared successful: %q, %v", body, err)
			}
		})
	}
}
