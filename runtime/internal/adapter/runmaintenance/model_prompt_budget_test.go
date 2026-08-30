package runmaintenance

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

type budgetInputCounter struct {
	calls int
	count int64
}

func (b *budgetInputCounter) CountInputTokens(context.Context, []chat.Message) (int64, error) {
	b.calls++
	return b.count, nil
}

func TestModelContextBudgetUsesProviderCountOnlyAtLocalThresholdOrForMedia(t *testing.T) {
	const threshold = 10_000
	counter := &budgetInputCounter{count: threshold - 1}
	budget := newModelContextBudget(
		newMessageCountTrigger(100),
		threshold,
		[]chat.Message{chat.NewSystemMessage("frozen instructions")},
		nil,
		nil,
		chat.Options{},
		0,
		counter,
	)
	over, _, _, err := budget.triggered(t.Context(), []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("well below threshold")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if over || counter.calls != 0 {
		t.Fatalf("text-only preflight = over:%t provider_counts:%d, want false and zero", over, counter.calls)
	}

	over, _, _, err = budget.triggered(t.Context(), []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("inspect"), chat.NewMediaPart(mustBudgetImage(t, []byte{0}))),
	})
	if err != nil {
		t.Fatal(err)
	}
	if over || counter.calls != 1 {
		t.Fatalf("media preflight = over:%t provider_counts:%d, want false and one", over, counter.calls)
	}
}

func TestModelContextBudgetDoesNotTreatMessageTriggerAsRequestCapacity(t *testing.T) {
	messages := make([]chat.Message, 0, 32)
	for range 32 {
		messages = append(messages, chat.NewAssistantMessage(chat.NewTextPart("short progress update")))
	}
	budget := newModelContextBudget(
		newMessageCountTrigger(24),
		10_000,
		nil,
		nil,
		nil,
		chat.Options{},
		0,
		nil,
	)

	triggered, tokenTriggered, _, err := budget.triggered(t.Context(), messages)
	if err != nil {
		t.Fatal(err)
	}
	if !triggered || tokenTriggered {
		t.Fatalf("trigger = maintenance:%t token:%t, want true/false", triggered, tokenTriggered)
	}
	overCapacity, _, err := budget.exceeded(t.Context(), messages)
	if err != nil {
		t.Fatal(err)
	}
	if overCapacity {
		t.Fatal("a message-count maintenance trigger was treated as a model-capacity limit")
	}
}

func TestMaintenanceModelTranscriptHasAggregateInputBound(t *testing.T) {
	const maximumInputBytes = 384 * 1024

	largeResult := strings.Repeat("x", 8*1024)
	messages := make([]chat.Message, 0, 128)
	for index := range 128 {
		messages = append(messages, chat.NewToolMessage(chat.ToolResult{
			ID:   "call",
			Name: "shell",
			Output: chat.NewTextToolOutput(
				largeResult + string(rune('a'+index%26)),
			),
		}))
	}

	transcript := renderTranscript(messages)
	if len(transcript) > maximumInputBytes {
		t.Errorf("renderTranscript = %d bytes, want at most %d", len(transcript), maximumInputBytes)
	}
	if measured := transcriptBytes(messages); measured <= len(transcript) {
		t.Fatalf("raw transcript measurement = %d, want greater than bounded rendering %d", measured, len(transcript))
	}
	if tokens := mustEstimateModelContextTokens(t, messages, nil, chat.Options{}); tokens <= len(transcript)/asciiBytesPerEstimatedToken {
		t.Fatalf("compaction estimate = %d, want raw footprint rather than bounded rendering", tokens)
	}
}

func TestModelContextEstimateCountsEveryModelVisiblePart(t *testing.T) {
	const repeatedRunes = 1_000
	cjk := strings.Repeat("界", repeatedRunes)
	arguments := `{"payload":"` + strings.Repeat("x", 16_000) + `"}`

	baseline := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("inspect")),
		chat.NewAssistantMessage(chat.NewTextPart("working")),
	}
	complete := []chat.Message{
		baseline[0],
		chat.NewAssistantMessage(
			chat.NewTextPart("working"),
			chat.NewReasoningPart(cjk, []byte(strings.Repeat("s", 2_000))),
			chat.NewToolCallPart(chat.ToolCall{
				ID: "call_budget", Name: "write", Arguments: arguments,
			}),
		),
	}

	baselineTokens := mustEstimateModelContextTokens(t, baseline, nil, chat.Options{})
	completeTokens := mustEstimateModelContextTokens(t, complete, nil, chat.Options{})
	if completeTokens <= baselineTokens+repeatedRunes {
		t.Fatalf(
			"complete estimate = %d, baseline = %d; reasoning, signature, and ToolCall arguments were not all counted",
			completeTokens,
			baselineTokens,
		)
	}
}

func TestModelContextEstimateDoesNotTreatInlineImageBytesAsText(t *testing.T) {
	small := mustBudgetImage(t, []byte{0})
	large := mustBudgetImage(t, make([]byte, 2<<20))

	smallTokens := mustEstimateModelContextTokens(t, []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("inspect"), chat.NewMediaPart(small)),
	}, nil, chat.Options{})
	largeTokens := mustEstimateModelContextTokens(t, []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("inspect"), chat.NewMediaPart(large)),
	}, nil, chat.Options{})
	if largeTokens != smallTokens {
		t.Fatalf(
			"inline image estimates = small:%d large:%d; provider-priced media bytes must not be tokenized as text",
			smallTokens,
			largeTokens,
		)
	}
}

func TestModelContextEstimateCountsToolManifestAndOptions(t *testing.T) {
	messages := []chat.Message{chat.NewUserMessage(chat.NewTextPart("inspect"))}
	baseline := mustEstimateModelContextTokens(t, messages, nil, chat.Options{})
	description := strings.Repeat("工具说明", 1_000)
	tools := []chat.ToolDefinition{{
		Name:        "inspect",
		Description: description,
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
	}}
	options := chat.Options{Stop: []string{strings.Repeat("停止", 500)}}
	complete := mustEstimateModelContextTokens(t, messages, tools, options)
	if complete <= baseline+1_000 {
		t.Fatalf(
			"complete estimate = %d, baseline = %d; Tool manifest and Options were not counted",
			complete,
			baseline,
		)
	}
}

func mustEstimateModelContextTokens(
	t *testing.T,
	messages []chat.Message,
	tools []chat.ToolDefinition,
	options chat.Options,
) int {
	t.Helper()
	tokens, err := estimateModelContextTokens(messages, tools, options)
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

func mustBudgetImage(t *testing.T, data []byte) *media.Media {
	t.Helper()
	value, err := media.NewBytes("image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
