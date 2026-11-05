package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const minimumMarkdownFenceLength = 3

func modelTextByItem(items []protocol.ArtifactItem, results []protocol.ArtifactToolResult) (map[string]protocol.ArtifactToolResult, error) {
	toolItems := make(map[string]struct{})
	for _, item := range items {
		if item.Type == protocol.ItemTypeToolCall {
			toolItems[item.ID] = struct{}{}
		}
	}
	byItem := make(map[string]protocol.ArtifactToolResult)
	for _, result := range results {
		if !result.ModelOnly {
			continue
		}
		if _, found := toolItems[result.ItemID]; !found {
			return nil, fmt.Errorf("trajectory: model-visible text evidence %q has no matching tool item", result.ID)
		}
		if _, duplicate := byItem[result.ItemID]; duplicate {
			return nil, fmt.Errorf("trajectory: tool item %q has duplicate model-visible text evidence", result.ItemID)
		}
		byItem[result.ItemID] = result
	}
	return byItem, nil
}

func renderToolMarkdown(b *strings.Builder, item protocol.ArtifactItem) error {
	if item.Tool == nil {
		return errors.New("tool invocation is absent")
	}
	invocation := *item.Tool
	result := invocation.Result
	invocation.Result = nil
	item.Tool = &invocation
	record, err := json.Marshal(item, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("encode call record: %w", err)
	}
	b.WriteString("## Tool call\n\n### Call record\n\n")
	writeMarkdownCode(b, "json", string(record))
	if result == nil {
		b.WriteString("### Result\n\nNo result was recorded.\n\n")
		return nil
	}
	if text, ok := result.(string); ok {
		b.WriteString("### Result (text)\n\n")
		writeMarkdownCode(b, "text", text)
		return nil
	}
	encoded, err := json.Marshal(result, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	b.WriteString("### Result (JSON)\n\n")
	writeMarkdownCode(b, "json", string(encoded))
	return nil
}

// Tool values remain literal evidence even when they contain Markdown fences,
// headings or HTML. A longer delimiter cannot be closed by the captured text.
func writeMarkdownCode(b *strings.Builder, language, text string) {
	longest, consecutive := minimumMarkdownFenceLength-1, 0
	for index := range len(text) {
		if text[index] == '`' {
			consecutive++
			longest = max(longest, consecutive)
		} else {
			consecutive = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	fmt.Fprintf(b, "%s%s\n%s", fence, language, text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, "%s\n\n", fence)
}
