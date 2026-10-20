package terminal

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/protocol"
)

func (a *app) ShowAgentDocuments() {
	if a.authoringContext == nil {
		a.message("this runtime composition has no authoring context service")
		return
	}
	workspace := a.session.current.Workspace.Path
	a.runRuntimeReaderQuery("loading agent documents", runtimeReaderAgentDocuments,
		func(ctx context.Context) (readerDocument, error) {
			documents, err := a.authoringContext.Documents(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return agentDocumentsDocument(workspace, documents), nil
		})
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
