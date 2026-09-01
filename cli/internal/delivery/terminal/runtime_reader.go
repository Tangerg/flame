package terminal

import (
	"context"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

type runtimeReaderMode uint8

const (
	runtimeReaderNone runtimeReaderMode = iota
	runtimeReaderGoal
	runtimeReaderDiscoveredSkills
	runtimeReaderManagedSkills
	runtimeReaderSkillProposals
	runtimeReaderMCPServers
	runtimeReaderMCPTools
	runtimeReaderMCPAuthorization
	runtimeReaderSchedules
	runtimeReaderModels
	runtimeReaderModelRoles
	runtimeReaderProviders
	runtimeReaderApprovalRules
	runtimeReaderAgentMemory
	runtimeReaderKnowledge
	runtimeReaderDiagnosticTools
	runtimeReaderAgentDocuments
	runtimeReaderRecipes
	runtimeReaderHooks
)

// runtimeReaderQuery describes one authoritative runtime projection read.
// Keeping the request as a value lets user-initiated reads and event-driven
// convergence share the same query without sharing their failure policy.
type runtimeReaderQuery struct {
	status    string
	mode      runtimeReaderMode
	selection runtimeReaderSelection
	read      func(context.Context) (readerDocument, error)
}

// runtimeReaderSelection carries typed identity needed to converge an open
// reader after an authoritative change event. It deliberately contains no UI
// or transport state.
type runtimeReaderSelection struct {
	knowledgeTarget   workspace.KnowledgeTarget
	knowledgeEntry    bool
	agentMemoryTarget agent.MemoryTarget
}

func (a *app) setRuntimeReader(mode runtimeReaderMode) {
	a.dialogs.runtimeReader = mode
	a.dialogs.runtimeSelection = runtimeReaderSelection{}
	if mode != runtimeReaderMCPTools {
		a.dialogs.mcpToolServer = ""
	}
	if mode != runtimeReaderMCPAuthorization {
		a.dialogs.mcpAuthorizationID = ""
	}
}

func (a *app) runRuntimeReaderQuery(
	status string,
	mode runtimeReaderMode,
	read func(context.Context) (readerDocument, error),
) {
	a.executeRuntimeReaderQuery(runtimeReaderQuery{status: status, mode: mode, read: read})
}

func (a *app) executeRuntimeReaderQuery(query runtimeReaderQuery) {
	a.status.note(query.status)
	a.runOperation(readerDocumentOperation, true, query.read, func(document readerDocument, err error) {
		if err != nil {
			a.message(query.status + " failed: " + err.Error())
			return
		}
		a.setRuntimeReader(query.mode)
		a.dialogs.runtimeSelection = query.selection
		a.dialogs.workspaceReader = workspaceReaderNone
		a.openReaderDocument(document)
		a.status.note(strings.ToLower(document.Title))
	})
}
