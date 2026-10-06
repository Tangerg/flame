package render

import (
	"bytes"
	json "encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestTextRendersStreamedAnswerToolAndUsage(t *testing.T) {
	var output bytes.Buffer
	renderer := NewText(&output)
	for _, event := range testEvents() {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{"hello world", "go test ./...", "PASS", "↑ 1,200", "cached 600"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text output does not contain %q:\n%s", want, text)
		}
	}
}

func TestTextRendersRunRecoveryMetadata(t *testing.T) {
	var output bytes.Buffer
	renderer := NewText(&output)
	if err := renderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Render(testEvent("failed", conversation.RunFinished{Outcome: conversation.Outcome{
		Status: protocol.OutcomeFailed,
		Problem: &protocol.ProblemData{
			Type: "rate_limited", Detail: "quota exhausted", RetryAfterSeconds: 12,
		},
	}})); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"quota exhausted", "retry after 12s"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("text output omitted %q: %s", want, output.String())
		}
	}
}

func TestRunOptionsJSONPreservesGenerationParameterPresence(t *testing.T) {
	temperature, topP, maxTokens := 0.7, 0.9, int64(2_048)
	configured, err := json.Marshal(encodeRunOptions(prompt.RunOptions{
		Provider: "mock", Model: "balanced", ReasoningEffort: "high",
		Generation: protocol.GenerationParams{
			Temperature: &temperature, MaxTokens: &maxTokens, TopP: &topP, Stop: []string{"END"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(configured), `{"provider":"mock","model":"balanced","reasoningEffort":"high","params":{"temperature":0.7,"maxTokens":2048,"topP":0.9,"stop":["END"]}}`; got != want {
		t.Fatalf("configured options = %s, want %s", got, want)
	}

	inherited, err := json.Marshal(encodeRunOptions(prompt.RunOptions{Provider: "mock", Model: "balanced"}))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(inherited), `{"provider":"mock","model":"balanced"}`; got != want {
		t.Fatalf("inherited options = %s, want %s", got, want)
	}
}

func TestTextStreamsOrderedDeltasUntilAuthoritativeCompletion(t *testing.T) {
	var output bytes.Buffer
	renderer := NewText(&output)
	if err := renderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, event := range []conversation.RunEvent{
		testEvent("start", conversation.BlockStarted{Block: conversation.Block{ID: "answer", Kind: conversation.BlockAssistant}}),
		testEvent("first", conversation.BlockDelta{BlockID: "answer", Text: "first"}),
		testEvent("second", conversation.BlockDelta{BlockID: "answer", Text: " second"}),
	} {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if got := output.String(); !strings.Contains(got, "first second") {
		t.Fatalf("ordered content stream = %q", got)
	}
	if err := renderer.Render(testEvent("complete", conversation.BlockCompleted{Block: conversation.Block{
		ID: "answer", Kind: conversation.BlockAssistant, Text: "first second",
	}})); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); strings.Count(got, "first") != 1 || strings.Count(got, "second") != 1 {
		t.Fatalf("completed output = %q", got)
	}
}

func TestNDJSONCarriesSegmentIdentityAndInterruptSet(t *testing.T) {
	var output bytes.Buffer
	renderer := NewNDJSON(&output)
	events := []conversation.RunEvent{
		testEvent("evt_start", conversation.SegmentStarted{Run: testRun()}),
		testEvent("evt_wait", conversation.RunInterrupted{Usage: conversation.Usage{InputTokens: 42}, Interrupts: []conversation.Interrupt{
			testApproval("tool_1", "shell"),
			conversation.Question{RunID: "run_1", ItemID: "question_1", Title: "choose", Fields: []conversation.QuestionField{{Prompt: "Target", Kind: conversation.QuestionSingle, Options: []protocol.QuestionOption{{Label: "linux"}, {Label: "darwin"}}}}},
		}}),
	}
	for _, event := range events {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON lines = %d", len(lines))
	}
	var frame map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &frame); err != nil {
		t.Fatal(err)
	}
	if frame["segmentId"] != "seg_1" || frame["eventId"] != "evt_wait" {
		t.Fatalf("event identity = %+v", frame)
	}
	interrupts, ok := frame["interrupts"].([]any)
	if !ok || len(interrupts) != 2 {
		t.Fatalf("interrupts = %#v", frame["interrupts"])
	}
	approval := interrupts[0].(map[string]any)
	tool := approval["tool"].(map[string]any)
	arguments := tool["arguments"].(map[string]any)
	if approval["rememberable"] != true || tool["name"] != "shell" || arguments["command"] != "go test ./..." {
		t.Fatalf("approval interrupt = %#v", approval)
	}
	usage, ok := frame["usage"].(map[string]any)
	if !ok || usage["inputTokens"] != float64(42) {
		t.Fatalf("interrupt usage = %#v", frame["usage"])
	}
}

func TestCompletedQuestionAnswersReachTextAndNDJSON(t *testing.T) {
	t.Parallel()

	question := conversation.Question{
		RunID: "run_1", ItemID: "question_1", Title: "choose",
		Fields: []conversation.QuestionField{{
			Prompt: "Target", Kind: conversation.QuestionSingle,
			Options: []protocol.QuestionOption{{Label: "linux"}, {Label: "darwin"}},
		}},
		Answers: [][]string{{"linux"}},
	}
	block := conversation.Block{
		ID: "question_1", RunID: "run_1", Kind: conversation.BlockQuestion,
		Status: conversation.BlockStatusCompleted, Question: &question,
	}
	event := testEvent("question", conversation.BlockCompleted{Block: block})

	var textOutput bytes.Buffer
	textRenderer := NewText(&textOutput)
	if err := textRenderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := textRenderer.Render(event); err != nil {
		t.Fatal(err)
	}
	if err := textRenderer.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textOutput.String(), "answer: linux") {
		t.Fatalf("text output omitted accepted answer: %q", textOutput.String())
	}

	var jsonOutput bytes.Buffer
	jsonRenderer := NewNDJSON(&jsonOutput)
	if err := jsonRenderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := jsonRenderer.Render(event); err != nil {
		t.Fatal(err)
	}
	if err := jsonRenderer.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jsonOutput.String(), `"answers":[["linux"]]`) {
		t.Fatalf("NDJSON omitted accepted answer: %s", jsonOutput.String())
	}
}

func TestNDJSONPreservesProgressToolArgumentsAndCustomPayloads(t *testing.T) {
	var output bytes.Buffer
	renderer := NewNDJSON(&output)
	if err := renderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	step, contextTokens := 4, int64(16_384)
	events := []conversation.RunEvent{
		testEvent("progress", conversation.RunProgress{Step: &step, ContextTokens: &contextTokens, Activity: "thinking", Usage: &conversation.Usage{InputTokens: 22}}),
		testEvent("arguments", conversation.ToolArgumentsDelta{BlockID: "tool_1", Text: `{"path":"/tmp`}),
		testEvent("custom", conversation.CustomEvent{Name: "vendor.trace", PayloadJSON: []byte(`{"span":"abc","sampled":true}`)}),
	}
	for _, event := range events {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("NDJSON lines = %d: %s", len(lines), output.String())
	}
	var progress, arguments, custom map[string]any
	for index, target := range []*map[string]any{&progress, &arguments, &custom} {
		if err := json.Unmarshal([]byte(lines[index]), target); err != nil {
			t.Fatal(err)
		}
	}
	if progress["type"] != "run.progress" || progress["step"] != float64(step) || progress["contextTokens"] != float64(contextTokens) {
		t.Fatalf("progress frame = %+v", progress)
	}
	if arguments["type"] != "tool.arguments.delta" || arguments["blockId"] != "tool_1" || arguments["text"] != `{"path":"/tmp` {
		t.Fatalf("arguments frame = %+v", arguments)
	}
	payload, ok := custom["payload"].(map[string]any)
	if custom["type"] != "custom" || custom["name"] != "vendor.trace" || !ok || payload["span"] != "abc" {
		t.Fatalf("custom frame = %+v", custom)
	}
}

func TestNDJSONCarriesContentDelta(t *testing.T) {
	var output bytes.Buffer
	renderer := NewNDJSON(&output)
	if err := renderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Render(testEvent("content", conversation.BlockDelta{BlockID: "answer", Text: "third"})); err != nil {
		t.Fatal(err)
	}
	var frame map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &frame); err != nil {
		t.Fatal(err)
	}
	if frame["type"] != "block.delta" || frame["text"] != "third" {
		t.Fatalf("content delta frame = %+v", frame)
	}
	if _, present := frame["index"]; present {
		t.Fatalf("content delta published a fabricated index: %+v", frame)
	}
}

func TestBlockJSONPreservesRuntimeItemMetadata(t *testing.T) {
	created := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	reasoning := encodeBlock(conversation.Block{
		ID: "reasoning", RunID: "run_1", Status: conversation.BlockStatusCompleted, Kind: conversation.BlockReasoning,
		CreatedAt: created, Redacted: true, Text: "Reasoning redacted by provider.",
	})
	if !reasoning.CreatedAt.Equal(created) || !reasoning.Redacted {
		t.Fatalf("reasoning frame = %+v", reasoning)
	}

	started, finished := created, created.Add(2*time.Second)
	tool := encodeBlock(conversation.Block{
		ID: "tool", RunID: "run_1", Status: conversation.BlockStatusIncomplete, Kind: conversation.BlockTool,
		Tool: &conversation.ToolCall{
			Kind: conversation.ToolShell, Name: "shell", Status: conversation.ToolError, Safety: protocol.SafetyClassExec,
			StartedAt: started, FinishedAt: finished,
			Problem: &protocol.ProblemData{Type: protocol.ProblemRateLimited, RetryAfterSeconds: 2},
		},
	})
	if tool.Tool == nil || tool.Tool.Safety != "exec" || !tool.Tool.StartedAt.Equal(started) || !tool.Tool.FinishedAt.Equal(finished) {
		t.Fatalf("tool frame = %+v", tool)
	}
	if tool.Tool.Problem == nil || tool.Tool.Problem.RetryAfterSeconds != 2 {
		t.Fatalf("tool problem frame = %+v", tool.Tool.Problem)
	}

	compaction := encodeBlock(conversation.Block{
		ID: "compact", RunID: "run_1", Status: conversation.BlockStatusCompleted, Kind: conversation.BlockNotice,
		CreatedAt: created, DroppedMessages: 17, Text: "Conversation compacted.",
	})
	if compaction.DroppedMessages != 17 {
		t.Fatalf("compaction frame = %+v", compaction)
	}
}

func TestRunJSONPreservesLifecycleTimestamps(t *testing.T) {
	created := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	finished := created.Add(2 * time.Second)
	frame := encodeRun(conversation.Run{
		ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusFinished,
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh",
		ContextTokens: 32_768,
		CreatedAt:     created, FinishedAt: finished, Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted},
	})
	if frame.ReasoningEffort != "xhigh" || frame.ContextTokens != 32_768 ||
		!frame.CreatedAt.Equal(created) || !frame.FinishedAt.Equal(finished) {
		t.Fatalf("run frame = %+v", frame)
	}
}

func TestOutcomeJSONPreservesStructuredProblem(t *testing.T) {
	encoded, err := json.Marshal(encodeOutcome(conversation.Outcome{
		Status: protocol.OutcomeFailed,
		Problem: &protocol.ProblemData{
			Type: "rate_limited", Detail: "quota exhausted", RetryAfterSeconds: 2,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"error":"quota exhausted"`, `"problem"`, `"type":"rate_limited"`, `"retryAfterSeconds":2`} {
		if !bytes.Contains(encoded, []byte(want)) {
			t.Fatalf("outcome JSON omitted %s: %s", want, encoded)
		}
	}
}

func TestResultJSONRetainsLatestRootProgressUsageBeforeSettlement(t *testing.T) {
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	if err := renderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Render(testEvent("progress", conversation.RunProgress{Usage: &conversation.Usage{InputTokens: 33, OutputTokens: 5}})); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	var result resultFrame
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "running" || result.Usage == nil || result.Usage.InputTokens != 33 || result.Usage.OutputTokens != 5 {
		t.Fatalf("incomplete result = %+v", result)
	}
}

func TestResultJSONUsesAuthoritativeAssistantCompletionAfterDeltas(t *testing.T) {
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	for _, event := range []conversation.RunEvent{
		testEvent("segment", conversation.SegmentStarted{Run: testRun()}),
		testEvent("start", conversation.BlockStarted{Block: conversation.Block{ID: "answer", Kind: conversation.BlockAssistant}}),
		testEvent("first", conversation.BlockDelta{BlockID: "answer", Text: "provisional first"}),
		testEvent("second", conversation.BlockDelta{BlockID: "answer", Text: " provisional second"}),
		testEvent("complete", conversation.BlockCompleted{Block: conversation.Block{ID: "answer", Kind: conversation.BlockAssistant, Text: "authoritative"}}),
		testEvent("finished", conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}),
	} {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["text"] != "authoritative" || strings.Contains(output.String(), "provisional") {
		t.Fatalf("result = %s", output.String())
	}
}

func TestResultJSONKeepsPartialAssistantOutputInEventOrder(t *testing.T) {
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	for _, event := range []conversation.RunEvent{
		testEvent("segment", conversation.SegmentStarted{Run: testRun()}),
		testEvent("start", conversation.BlockStarted{Block: conversation.Block{ID: "answer", Kind: conversation.BlockAssistant}}),
		testEvent("first", conversation.BlockDelta{BlockID: "answer", Text: "first"}),
		testEvent("second", conversation.BlockDelta{BlockID: "answer", Text: " second"}),
	} {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "running" || result["text"] != "first second" {
		t.Fatalf("partial result = %s", output.String())
	}
}

func TestResultJSONDoesNotRetainProvisionalTextForEmptyCompletion(t *testing.T) {
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	for _, event := range []conversation.RunEvent{
		testEvent("segment", conversation.SegmentStarted{Run: testRun()}),
		testEvent("start", conversation.BlockStarted{Block: conversation.Block{ID: "answer", Kind: conversation.BlockAssistant}}),
		testEvent("delta", conversation.BlockDelta{BlockID: "answer", Text: "provisional"}),
		testEvent("complete", conversation.BlockCompleted{Block: conversation.Block{ID: "answer", Kind: conversation.BlockAssistant}}),
		testEvent("finished", conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}),
	} {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "provisional") {
		t.Fatalf("empty authoritative completion retained provisional text: %s", output.String())
	}
}

func TestRenderersPreserveAssistantInlineImages(t *testing.T) {
	image := conversation.InlineImage{
		ID: "answer:image:0", Name: "chart.png", MIMEType: "image/png", Data: []byte("png bytes"),
	}
	completed := testEvent("image", conversation.BlockCompleted{Block: conversation.Block{
		ID: "answer", Kind: conversation.BlockAssistant, Text: "Generated chart", Images: []conversation.InlineImage{image},
	}})

	var stream bytes.Buffer
	ndjson := NewNDJSON(&stream)
	if err := ndjson.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := ndjson.Render(completed); err != nil {
		t.Fatal(err)
	}
	var event eventRecord
	if err := json.Unmarshal(stream.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.Block == nil || len(event.Block.Images) != 1 || event.Block.Images[0].Name != image.Name ||
		!bytes.Equal(event.Block.Images[0].Data, image.Data) {
		t.Fatalf("stream image = %+v", event.Block)
	}

	var resultOutput bytes.Buffer
	result := NewResultJSON(&resultOutput)
	if err := result.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := result.Render(completed); err != nil {
		t.Fatal(err)
	}
	if err := result.Close(); err != nil {
		t.Fatal(err)
	}
	var final resultFrame
	if err := json.Unmarshal(resultOutput.Bytes(), &final); err != nil {
		t.Fatal(err)
	}
	if len(final.Images) != 1 || !bytes.Equal(final.Images[0].Data, image.Data) {
		t.Fatalf("result images = %+v", final.Images)
	}

	var textOutput bytes.Buffer
	plain := NewText(&textOutput)
	if err := plain.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := plain.Render(completed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textOutput.String(), "@ chart.png (image/png, 9 bytes)") {
		t.Fatalf("text image fallback = %q", textOutput.String())
	}
}

func TestNDJSONPreservesCompleteToolArgumentsAndResult(t *testing.T) {
	var output bytes.Buffer
	renderer := NewNDJSON(&output)
	if err := renderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	event := testEvent("tool", conversation.BlockCompleted{Block: conversation.Block{
		ID: "tool_1", Kind: conversation.BlockTool, Tool: &conversation.ToolCall{
			Kind: conversation.ToolUnknown, Name: "mcp__calendar__create", Status: conversation.ToolOK,
			ArgumentsJSON: []byte(`{"guests":["a@example.com"]}`), ResultJSON: []byte(`{"eventId":"evt_123"}`),
		},
	}})
	if err := renderer.Render(event); err != nil {
		t.Fatal(err)
	}
	var frame map[string]any
	if err := json.Unmarshal(output.Bytes(), &frame); err != nil {
		t.Fatal(err)
	}
	block := frame["block"].(map[string]any)
	tool := block["tool"].(map[string]any)
	if tool["arguments"].(map[string]any)["guests"].([]any)[0] != "a@example.com" ||
		tool["result"].(map[string]any)["eventId"] != "evt_123" {
		t.Fatalf("tool frame = %+v", tool)
	}
}

func TestResultJSONClearsPriorInterruptWhenANewSegmentStarts(t *testing.T) {
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	if err := renderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	question := conversation.Question{
		RunID: "run_1", ItemID: "question_1", Title: "choose",
		Fields: []conversation.QuestionField{{Prompt: "Target", Kind: conversation.QuestionSingle, Options: []protocol.QuestionOption{{Label: "linux"}, {Label: "darwin"}}}},
	}
	resumed := testRun()
	resumed.ActiveSegmentID = "seg_2"
	for _, event := range []conversation.RunEvent{
		testEvent("start", conversation.SegmentStarted{Run: testRun()}),
		testEvent("wait", conversation.RunInterrupted{Interrupts: []conversation.Interrupt{question}}),
		{EventID: "resume", RunID: "run_1", SegmentID: "seg_2", Event: conversation.SegmentStarted{Run: resumed}},
	} {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "running" || result["interrupts"] != nil {
		t.Fatalf("resumed result = %+v", result)
	}
}

func TestResultJSONFoldsFinalAssistantProjection(t *testing.T) {
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	if err := renderer.Begin(testRun(), prompt.RunOptions{Provider: "mock", Model: "balanced"}); err != nil {
		t.Fatal(err)
	}
	for _, event := range testEvents() {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "finished" || result["text"] != "hello world" || result["runId"] != "run_1" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRenderersPreserveChildRunIdentityWithoutSettlingTheRoot(t *testing.T) {
	events := runTreeEvents(t)

	var textOutput bytes.Buffer
	textRenderer := NewText(&textOutput)
	if err := textRenderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := textRenderer.Render(event); err != nil {
			t.Fatalf("render text %s: %v", event.EventID, err)
		}
		if event.EventID == "child-finished" && textRenderer.settled {
			t.Fatal("child completion settled the root text renderer")
		}
	}
	if err := textRenderer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"child answer", "root answer"} {
		if !strings.Contains(textOutput.String(), want) {
			t.Fatalf("tree text output is missing %q:\n%s", want, textOutput.String())
		}
	}

	var streamOutput bytes.Buffer
	streamRenderer := NewNDJSON(&streamOutput)
	if err := streamRenderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := streamRenderer.Render(event); err != nil {
			t.Fatalf("render NDJSON %s: %v", event.EventID, err)
		}
	}
	lines := strings.Split(strings.TrimSpace(streamOutput.String()), "\n")
	var childStart eventRecord
	if err := json.Unmarshal([]byte(lines[1]), &childStart); err != nil {
		t.Fatal(err)
	}
	if childStart.Type != "segment.started" || childStart.RunID != "run_child" ||
		childStart.ParentRunID != "run_1" || childStart.RootRunID != "run_1" ||
		childStart.SpawnedByBlockID != "spawn" || childStart.StreamSegmentID != "seg_1" || childStart.SegmentID != "seg_child" {
		t.Fatalf("child segment frame = %+v", childStart)
	}

	var resultOutput bytes.Buffer
	resultRenderer := NewResultJSON(&resultOutput)
	if err := resultRenderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := resultRenderer.Render(event); err != nil {
			t.Fatalf("render result %s: %v", event.EventID, err)
		}
	}
	if err := resultRenderer.Close(); err != nil {
		t.Fatal(err)
	}
	var result resultFrame
	if err := json.Unmarshal(resultOutput.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.RunID != "run_1" || result.Status != "finished" || result.Text != "root answer" {
		t.Fatalf("tree result = %+v", result)
	}
}

func TestColdReconciliationKeepsOneShotOutputScopedToItsRun(t *testing.T) {
	snapshot := reconciliationSnapshot(t)
	requireScopedResultJSON(t, snapshot)
	requireScopedText(t, snapshot)
	requireScopedStream(t, snapshot)
}

func reconciliationSnapshot(t testing.TB) conversation.SessionSnapshot {
	t.Helper()
	plan := protocol.Plan{SessionID: "ses_1", State: &protocol.PlanState{Revision: 3, UpdatedAt: time.Unix(1, 0).UTC(), Steps: []protocol.PlanStep{
		{ID: "1", Description: "newer plan", Status: protocol.PlanStatusCompleted},
	}}}
	return conversation.SessionSnapshot{
		Session: conversation.Session{ID: "ses_1", Status: protocol.SessionStatusIdle, Provider: "mock", Model: "balanced", Workspace: workspace.Workspace{
			Path: "/tmp/demo", ProjectRoot: "/tmp/demo", Availability: protocol.WorkspaceAvailable,
		}, Revision: 1},
		Transcript: []conversation.Block{
			{ID: "old", RunID: "run_old", Status: conversation.BlockStatusCompleted, Kind: conversation.BlockAssistant, Text: "historical answer"},
			{ID: "current", RunID: "run_1", Status: conversation.BlockStatusCompleted, Kind: conversation.BlockAssistant, Text: "current answer"},
			{ID: "new", RunID: "run_new", Status: conversation.BlockStatusCompleted, Kind: conversation.BlockAssistant, Text: "newer answer"},
		},
		Runs: []conversation.Run{
			{ID: "run_old", SessionID: "ses_1", Lineage: conversation.RootRunLineage(), Status: protocol.RunStatusFinished, Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}},
			{ID: "run_1", SessionID: "ses_1", Lineage: conversation.RootRunLineage(), Status: protocol.RunStatusFinished, Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}},
			{ID: "run_new", SessionID: "ses_1", Lineage: conversation.RootRunLineage(), Status: protocol.RunStatusFinished, Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}},
		},
		Plan: &plan,
	}
}

func requireScopedResultJSON(t *testing.T, snapshot conversation.SessionSnapshot) {
	t.Helper()
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	if err := renderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Reconcile(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "historical answer") || strings.Contains(output.String(), "newer answer") || !strings.Contains(output.String(), "current answer") {
		t.Fatalf("reconciled result = %s", output.String())
	}
}

func requireScopedText(t *testing.T, snapshot conversation.SessionSnapshot) {
	t.Helper()
	var output bytes.Buffer
	textRenderer := NewText(&output)
	if err := textRenderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := textRenderer.Reconcile(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := textRenderer.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "historical answer") || strings.Contains(output.String(), "newer answer") || !strings.Contains(output.String(), "current answer") {
		t.Fatalf("reconciled text = %s", output.String())
	}
}

func requireScopedStream(t *testing.T, snapshot conversation.SessionSnapshot) {
	t.Helper()
	var output bytes.Buffer
	streamRenderer := NewNDJSON(&output)
	if err := streamRenderer.Begin(testRun(), prompt.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := streamRenderer.Reconcile(snapshot); err != nil {
		t.Fatal(err)
	}
	var frame eventRecord
	if err := json.Unmarshal(output.Bytes(), &frame); err != nil {
		t.Fatal(err)
	}
	if frame.RunID != "run_1" || len(frame.Transcript) != 1 || frame.Transcript[0].Text != "current answer" {
		t.Fatalf("reconciled stream = %+v", frame)
	}
	if frame.Revision != 0 || len(frame.Plan) != 0 {
		t.Fatalf("reconciled stream leaked newer plan = %+v", frame.Plan)
	}
}

func TestRenderersRejectInvalidEvents(t *testing.T) {
	invalid := conversation.RunEvent{EventID: "evt", RunID: "run_1", SegmentID: "seg_1", Event: conversation.BlockDelta{}}
	var output bytes.Buffer
	if err := NewText(&output).Render(invalid); err == nil {
		t.Fatal("text accepted invalid event")
	}
	if err := NewNDJSON(&output).Render(invalid); err == nil {
		t.Fatal("NDJSON accepted invalid event")
	}
	if err := NewResultJSON(&output).Render(invalid); err == nil {
		t.Fatal("result JSON accepted invalid event")
	}
}

func TestUsageJSONDistinguishesUnknownFromKnownZeroCost(t *testing.T) {
	unknown, err := json.Marshal(encodeUsage(conversation.Usage{}))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(unknown, []byte(`"costUsd"`)) {
		t.Fatalf("unknown cost was serialized: %s", unknown)
	}
	known, err := json.Marshal(encodeUsage(conversation.Usage{CostUSD: new(0.0)}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(known, []byte(`"costUsd":0`)) {
		t.Fatalf("known zero cost was omitted: %s", known)
	}
	modelCost := 0.25
	detailed, err := json.Marshal(encodeUsage(conversation.Usage{
		Steps: 3,
		ByModel: map[string]protocol.ModelUsage{
			"deepseek/v4": {InputTokens: 12, ReasoningTokens: 4, CostUSD: &modelCost},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"steps":3`, `"byModel"`, `"deepseek/v4"`, `"reasoningTokens":4`, `"costUsd":0.25`} {
		if !bytes.Contains(detailed, []byte(want)) {
			t.Fatalf("detailed usage omitted %s: %s", want, detailed)
		}
	}
}

func testEvents() []conversation.RunEvent {
	code := 0
	return []conversation.RunEvent{
		testEvent("evt_start", conversation.SegmentStarted{Run: testRun()}),
		testEvent("evt_message_start", conversation.BlockStarted{Block: conversation.Block{ID: "msg_1", Kind: conversation.BlockAssistant}}),
		testEvent("evt_message_delta_1", conversation.BlockDelta{BlockID: "msg_1", Text: "hello "}),
		testEvent("evt_message_delta_2", conversation.BlockDelta{BlockID: "msg_1", Text: "world"}),
		testEvent("evt_message_done", conversation.BlockCompleted{Block: conversation.Block{ID: "msg_1", Kind: conversation.BlockAssistant, Text: "hello world"}}),
		testEvent("evt_tool", conversation.BlockCompleted{Block: conversation.Block{ID: "tool_1", Kind: conversation.BlockTool, Tool: &conversation.ToolCall{
			Kind: conversation.ToolShell, Name: "shell", Summary: "go test ./...", Status: conversation.ToolOK,
			Command: "go test ./...", Output: "PASS", ExitCode: &code,
		}}}),
		testEvent("evt_done", conversation.RunFinished{
			Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted},
			Usage:   conversation.Usage{InputTokens: 1_200, OutputTokens: 80, CacheReadTokens: 600, CostUSD: new(0.01), Duration: time.Second},
		}),
	}
}

func runTreeEvents(t *testing.T) []conversation.RunEvent {
	t.Helper()
	root := testRun()
	lineage, err := conversation.NewChildRunLineage("run_child", "spawn", root.ID, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	child := conversation.Run{
		ID: "run_child", SessionID: root.SessionID,
		Lineage:  lineage,
		Provider: root.Provider, Model: root.Model, Status: protocol.RunStatusRunning, ActiveSegmentID: "seg_child",
	}
	event := func(id, runID, segmentID string, payload conversation.Event) conversation.RunEvent {
		return conversation.RunEvent{
			EventID: id, RunID: runID, SegmentID: segmentID, StreamSegmentID: root.ActiveSegmentID,
			At: time.Unix(1, 0), Event: payload,
		}
	}
	block := func(runID, text string, status conversation.BlockStatus) conversation.Block {
		return conversation.Block{ID: "answer", RunID: runID, Kind: conversation.BlockAssistant, Status: status, Text: text}
	}
	return []conversation.RunEvent{
		event("root-started", root.ID, root.ActiveSegmentID, conversation.SegmentStarted{Run: root}),
		event("child-started", child.ID, child.ActiveSegmentID, conversation.SegmentStarted{Run: child}),
		event("child-block-started", child.ID, child.ActiveSegmentID, conversation.BlockStarted{Block: block(child.ID, "", conversation.BlockStatusRunning)}),
		event("child-delta", child.ID, child.ActiveSegmentID, conversation.BlockDelta{BlockID: "answer", Text: "child answer"}),
		event("child-block-completed", child.ID, child.ActiveSegmentID, conversation.BlockCompleted{Block: block(child.ID, "child answer", conversation.BlockStatusCompleted)}),
		event("child-finished", child.ID, child.ActiveSegmentID, conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}),
		event("root-block-started", root.ID, root.ActiveSegmentID, conversation.BlockStarted{Block: block(root.ID, "", conversation.BlockStatusRunning)}),
		event("root-delta", root.ID, root.ActiveSegmentID, conversation.BlockDelta{BlockID: "answer", Text: "root answer"}),
		event("root-block-completed", root.ID, root.ActiveSegmentID, conversation.BlockCompleted{Block: block(root.ID, "root answer", conversation.BlockStatusCompleted)}),
		event("root-finished", root.ID, root.ActiveSegmentID, conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}),
	}
}

func testEvent(id string, event conversation.Event) conversation.RunEvent {
	switch item := event.(type) {
	case conversation.BlockStarted:
		item.Block.RunID = "run_1"
		item.Block.Status = conversation.BlockStatusRunning
		event = item
	case conversation.BlockCompleted:
		item.Block.RunID = "run_1"
		item.Block.Status = conversation.BlockStatusCompleted
		event = item
	}
	return conversation.RunEvent{EventID: id, RunID: "run_1", SegmentID: "seg_1", At: time.Unix(1, 0), Event: event}
}

func testRun() conversation.Run {
	return conversation.Run{
		ID: "run_1", SessionID: "ses_1", Provider: "mock", Model: "balanced",
		Lineage: conversation.RootRunLineage(),
		Status:  protocol.RunStatusRunning, ActiveSegmentID: "seg_1",
	}
}

func testApproval(itemID, title string) conversation.Approval {
	return conversation.Approval{
		RunID: "run_1", ItemID: itemID, Title: title, Rememberable: true,
		Tool: &conversation.ToolCall{
			Kind: conversation.ToolShell, Name: "shell", Command: "go test ./...", Status: conversation.ToolRunning,
			ArgumentsJSON: []byte(`{"command":"go test ./...","timeoutMs":30000}`),
		},
	}
}
