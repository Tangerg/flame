package execution

import (
	"context"
	"crypto/rand"
	"log/slog"
	"strings"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/scope/core/content"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// evictToolContent reduces model-facing text without changing the original
// structured result, media, citations or opaque block metadata. One existing
// text blob owns the omitted bytes; block previews name their exact byte range.
func evictToolContent(ctx context.Context, store toolResultOffloader, policy toolResultOffloadPolicy, sessionID, toolName string, output toolcontract.Output) (toolcontract.Output, *transcript.ToolResultRef) {
	if store == nil || !policy.enabled || sessionID == "" || toolName == policy.readerName {
		return output, nil
	}
	eligible := func(block content.Block) bool {
		return block.Kind == content.Text && len(block.Citations) == 0 && len(block.Metadata) == 0
	}
	textBytes, blocks := 0, 0
	for _, block := range output.Content {
		if eligible(block) {
			textBytes += len(block.Text)
			blocks++
		}
	}
	if textBytes <= policy.threshold || blocks == 0 {
		return output, nil
	}
	reference := transcript.ToolResultRef{ID: transcript.ToolResultID(rand.Text()), ModelOnly: true}
	projected := output.Clone()
	var body strings.Builder
	previewBytes := min(toolResultPreviewBytes, policy.threshold) / blocks
	for index, block := range output.Content {
		if !eligible(block) {
			continue
		}
		preview := renderToolResultRangePreview(block.Text, reference.ID.String(), policy.readerName, body.Len(), previewBytes)
		if len(preview) >= len(block.Text) {
			continue
		}
		projected.Content[index].Text = preview
		withReference, err := modeladapter.WithToolResultBlockReference(projected.Content[index], reference, policy.readerName, modeladapter.ToolResultRange{OffsetBytes: body.Len(), LengthBytes: len(block.Text)})
		if err != nil {
			return output, nil
		}
		projected.Content[index] = withReference
		body.WriteString(block.Text)
	}
	if body.Len() == 0 {
		return output, nil
	}
	if err := store.Stage(ctx, transcript.ToolResultStage{ID: reference.ID, SessionID: sessionID, Body: body.String()}); err != nil {
		slog.WarnContext(ctx, "execution: persist offloaded tool content", "session.id", sessionID, "tool.name", toolName, "error", err)
		return output, nil
	}
	return projected, &reference
}
