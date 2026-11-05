package main

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// renderSessionMarkdown produces a human-readable transcript of a session — a
// header plus each canonical item rendered by type. It is not re-importable
// (use format=json for that).
func renderSessionMarkdown(session protocol.ArtifactSession, items []protocol.ArtifactItem, toolResults []protocol.ArtifactToolResult) (string, error) {
	modelText, err := modelTextByItem(items, toolResults)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	title := session.Title
	if title == "" {
		title = session.ID
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	if session.Workspace.Path != "" {
		fmt.Fprintf(&b, "- workspace: `%s`\n", session.Workspace.Path)
	}
	if session.Model != "" {
		fmt.Fprintf(&b, "- model: `%s`\n", session.Model)
	}
	b.WriteString("\n")
	if err := renderSessionItems(&b, items, modelText); err != nil {
		return "", err
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

func renderSessionItems(b *strings.Builder, items []protocol.ArtifactItem, modelText map[string]protocol.ArtifactToolResult) error {
	for _, item := range items {
		switch item.Type {
		case protocol.ItemTypeUserMessage:
			fmt.Fprintf(b, "## User\n\n%s\n\n", contentText(item))
		case protocol.ItemTypeAgentMessage:
			fmt.Fprintf(b, "## Assistant\n\n%s\n\n", contentText(item))
		case protocol.ItemTypeReasoning:
			if !item.Redacted && item.Text != "" {
				fmt.Fprintf(b, "> _(reasoning)_ %s\n\n", item.Text)
			}
		case protocol.ItemTypeToolCall:
			if err := renderToolMarkdown(b, item); err != nil {
				return fmt.Errorf("trajectory: render tool item %q: %w", item.ID, err)
			}
			if evidence, present := modelText[item.ID]; present {
				b.WriteString("### Model-visible text evidence\n\n")
				writeMarkdownCode(b, "text", evidence.Body)
			}
		case protocol.ItemTypeQuestion:
			if item.Question != nil {
				b.WriteString("## Question\n\n")
				for _, field := range item.Question.Fields {
					fmt.Fprintf(b, "- %s\n", field.Prompt)
				}
				b.WriteString("\n")
			}
		case protocol.ItemTypeCompaction:
			fmt.Fprintf(
				b,
				"## Context compacted\n\n- dropped messages: %d\n\n%s\n\n",
				item.DroppedMessages,
				item.Summary,
			)
		}
	}
	return nil
}

// contentText returns the text of an item (image blocks render as "[image]").
func contentText(item protocol.ArtifactItem) string {
	if item.Text != "" {
		return item.Text
	}
	var parts []string
	for _, block := range item.Content {
		switch block.Type {
		case protocol.ContentBlockText:
			parts = append(parts, block.Text)
		case protocol.ContentBlockImage:
			parts = append(parts, "[image]")
		}
	}
	return strings.Join(parts, "\n")
}
