package terminal

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// agentDocumentScreens shows the AGENTS.md documents that apply to the workspace.
type agentDocumentScreens struct {
	screenHost
	service AuthoringContext
}

func (c *agentDocumentScreens) ShowAgentDocuments() {
	workspace := c.session.current.Workspace.Path
	c.reader.show(&readerQuery{status: "loading agent documents", read: func(ctx context.Context) (readerDocument, error) {
		documents, err := c.service.Documents(ctx, workspace)
		if err != nil {
			return readerDocument{}, err
		}
		return agentDocumentsDocument(workspace, documents), nil
	}})
}

func agentDocumentsDocument(workspacePath string, documents []protocol.AgentDoc) readerDocument {
	if len(documents) == 0 {
		return paragraphDocument("Agent documents", workspacePath, []string{"No AGENTS.md documents apply to this workspace."})
	}
	lines := make([]string, 0, len(documents))
	for _, document := range documents {
		label := string(document.Scope) + "  " + document.Path
		lines = append(lines, label)
	}
	return paragraphDocument("Agent documents", fmt.Sprintf("%d applicable · %s", len(documents), workspacePath), lines)
}
