package agentexec

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/workspace"
)

const agentDocPromptMaxBytes = 32 * 1024

const agentDocPromptHeader = "## Project context (from AGENTS.md cascade)"

// agentDocumentsPrompt formats discovered files for the agent system prompt. The
// provenance marker and byte budget are part of the model-facing prompt, not
// the agent-document domain value.
type agentDocumentsPrompt struct {
	text    string
	sources contextSources
}

type agentDocumentBlock struct {
	text string
	path string
}

func newAgentDocumentsPrompt(files []workspace.AgentDocFile, maxBytes int) (agentDocumentsPrompt, error) {
	if len(files) == 0 || maxBytes <= 0 {
		return agentDocumentsPrompt{}, nil
	}
	if err := workspace.ValidateAgentDocumentCascade(files); err != nil {
		return agentDocumentsPrompt{}, err
	}
	blocks, total := buildAgentDocumentBlocks(files)
	selected, total, err := selectAgentDocumentBlocks(blocks, total, maxBytes)
	if err != nil {
		return agentDocumentsPrompt{}, err
	}
	return renderAgentDocumentBlocks(selected, total), nil
}

func buildAgentDocumentBlocks(files []workspace.AgentDocFile) ([]agentDocumentBlock, int) {
	blocks := make([]agentDocumentBlock, len(files))
	total := len(agentDocPromptHeader) + 2 + max(0, len(files)-1)
	for i, file := range files {
		text := "<!-- From: " + file.Path + " -->\n" + file.Content + "\n"
		blocks[i] = agentDocumentBlock{text: text, path: file.Path}
		total += len(text)
	}
	return blocks, total
}

func selectAgentDocumentBlocks(
	blocks []agentDocumentBlock,
	total int,
	maxBytes int,
) ([]agentDocumentBlock, int, error) {
	start := 0
	for start < len(blocks) && total > maxBytes {
		total -= len(blocks[start].text)
		if start+1 < len(blocks) {
			total--
		}
		start++
	}
	if start == len(blocks) {
		return nil, 0, fmt.Errorf(
			"%w: agent document %q cannot fit the %d-byte Run guidance budget",
			workspace.ErrPromptSourceTooLarge,
			blocks[len(blocks)-1].path,
			maxBytes,
		)
	}
	return blocks[start:], total, nil
}

func renderAgentDocumentBlocks(blocks []agentDocumentBlock, total int) agentDocumentsPrompt {
	var prompt strings.Builder
	prompt.Grow(total)
	sources := make(contextSources, 0, len(blocks))
	for i, block := range blocks {
		if i > 0 {
			prompt.WriteByte('\n')
		}
		prompt.WriteString(block.text)
		sources = append(sources, contextSourceAgentDocument.source(block.path))
	}
	return agentDocumentsPrompt{text: prompt.String(), sources: sources}
}

func (a agentDocumentsPrompt) appendTo(composition *promptComposition) {
	if a.text == "" {
		return
	}
	composition.append(
		agentDocPromptHeader+"\n\n"+a.text,
		a.sources[0],
		a.sources[1:]...,
	)
}
