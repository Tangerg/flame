package execution

import (
	"crypto/rand"
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/scope/core/content"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

const toolResultReaderName = string(builtin.ReadToolResult)

// The preview includes a retrieval marker in addition to retained body bytes.
const toolResultPreviewBytes = 2000

type toolResultOffloadPolicy struct {
	threshold int
}

type toolResultOffloadIdentity struct {
	Threshold  int    `json:"threshold"`
	ReaderName string `json:"readerName"`
}

func newToolResultOffloadPolicy(threshold *int) (toolResultOffloadPolicy, error) {
	if threshold == nil {
		return toolResultOffloadPolicy{}, nil
	}
	if *threshold <= 0 {
		return toolResultOffloadPolicy{}, errors.New("execution: Tool-result offload threshold must be positive")
	}
	return toolResultOffloadPolicy{threshold: *threshold}, nil
}

func (p toolResultOffloadPolicy) identity() *toolResultOffloadIdentity {
	if p.threshold == 0 {
		return nil
	}
	return &toolResultOffloadIdentity{Threshold: p.threshold, ReaderName: toolResultReaderName}
}

func (p toolResultOffloadPolicy) project(toolName string, output toolcontract.Output) (toolcontract.Output, *transcript.ToolResultOffload, error) {
	if p.threshold == 0 || toolName == toolResultReaderName {
		return output, nil, nil
	}
	if (len(output.Content) > 0 && len(output.Details) > 0) || slices.ContainsFunc(output.Content, func(block content.Block) bool {
		return !offloadableToolText(block)
	}) {
		return p.projectRichText(output)
	}
	text, textual := output.Text()
	if !textual || len(text) <= p.threshold {
		return output, nil, nil
	}
	ref := transcript.ToolResultRef{ID: transcript.ToolResultID(rand.Text()), ModelOnly: len(output.Details) > 0}
	preview := renderToolResultPreview(text, ref.ID.String(), min(toolResultPreviewBytes, p.threshold))
	if len(preview) >= len(text) {
		return output, nil, nil
	}
	projected := toolcontract.NewTextOutput(preview)
	if ref.ModelOnly {
		// Details-only output keeps its structured value for transcript readers.
		projected.Details = output.Clone().Details
	}
	projected, err := modeladapter.WithToolResultReference(projected, ref, modeladapter.ToolResultRange{LengthBytes: len(text)})
	if err != nil {
		return toolcontract.Output{}, nil, err
	}
	return projected, &transcript.ToolResultOffload{Ref: ref, Body: text}, nil
}

func offloadableToolText(block content.Block) bool {
	return block.Kind == content.Text && len(block.Citations) == 0 && len(block.Metadata) == 0
}

// Rich results keep their structured value. Model previews name exact byte
// ranges in one body so retrieving one block does not depend on the others.
func (p toolResultOffloadPolicy) projectRichText(output toolcontract.Output) (toolcontract.Output, *transcript.ToolResultOffload, error) {
	textBytes, blocks := 0, 0
	for _, block := range output.Content {
		if offloadableToolText(block) {
			textBytes += len(block.Text)
			blocks++
		}
	}
	if textBytes <= p.threshold {
		return output, nil, nil
	}
	reference := transcript.ToolResultRef{ID: transcript.ToolResultID(rand.Text()), ModelOnly: true}
	projected := output.Clone()
	var body strings.Builder
	previewBytes := min(toolResultPreviewBytes, p.threshold) / blocks
	for index, block := range output.Content {
		if !offloadableToolText(block) {
			continue
		}
		preview := renderToolResultRangePreview(block.Text, reference.ID.String(), body.Len(), previewBytes)
		if len(preview) >= len(block.Text) {
			continue
		}
		projected.Content[index].Text = preview
		withReference, err := modeladapter.WithToolResultBlockReference(projected.Content[index], reference, modeladapter.ToolResultRange{OffsetBytes: body.Len(), LengthBytes: len(block.Text)})
		if err != nil {
			return toolcontract.Output{}, nil, err
		}
		projected.Content[index] = withReference
		body.WriteString(block.Text)
	}
	if body.Len() == 0 {
		return output, nil, nil
	}
	return projected, &transcript.ToolResultOffload{Ref: reference, Body: body.String()}, nil
}
