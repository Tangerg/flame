package terminal

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

var runtimeRecoveryBackoff = func() retry.Backoff {
	backoff, err := retry.NewBackoff(100*time.Millisecond, 5*time.Second)
	if err != nil {
		panic("invalid static runtime recovery backoff: " + err.Error())
	}
	return backoff
}()

func (a *app) applyRuntimeInvalidation(event changefeed.Event) {
	if event.Type == changefeed.Resync {
		a.applyRuntimeResync(event.Topics)
		return
	}
	a.refreshGoalReader(goalInvalidationAffectsSession(event, a.session.current.ID))
	a.refreshSkillReader(changefeed.Topic(event.Type) == changefeed.SkillsChanged)
	a.refreshMCPReader(changefeed.Topic(event.Type) == changefeed.MCPChanged)
	a.refreshScheduleReader(changefeed.Topic(event.Type) == changefeed.SchedulesChanged)
	a.refreshKnowledgeReader(changefeed.Topic(event.Type) == changefeed.KnowledgeChanged)
	a.refreshHooksReader(changefeed.Topic(event.Type) == changefeed.HooksChanged)
	a.refreshModelReader(changefeed.Topic(event.Type) == changefeed.ModelsChanged)
	a.refreshApprovalReader(changefeed.Topic(event.Type) == changefeed.ApprovalsChanged)
	a.refreshAgentMemoryReader(changefeed.Topic(event.Type) == changefeed.AgentMemoryChanged)
	a.applySessionInvalidation(
		invalidatesSessionCatalog(event),
		invalidationAffectsSession(event, a.session.current.ID, a.execution.conversation.RunID()),
	)
}

func (a *app) applyRuntimeResync(topics []changefeed.Topic) {
	a.refreshGoalReader(containsTopic(topics, changefeed.GoalsChanged))
	a.refreshSkillReader(containsTopic(topics, changefeed.SkillsChanged))
	a.refreshMCPReader(containsTopic(topics, changefeed.MCPChanged))
	a.refreshScheduleReader(containsTopic(topics, changefeed.SchedulesChanged))
	a.refreshKnowledgeReader(containsTopic(topics, changefeed.KnowledgeChanged))
	a.refreshHooksReader(containsTopic(topics, changefeed.HooksChanged))
	a.refreshModelReader(containsTopic(topics, changefeed.ModelsChanged))
	a.refreshApprovalReader(containsTopic(topics, changefeed.ApprovalsChanged))
	a.refreshAgentMemoryReader(containsTopic(topics, changefeed.AgentMemoryChanged))
	a.applySessionInvalidation(
		invalidatesSessionCatalog(changefeed.Event{Type: changefeed.Resync, Topics: topics}),
		resyncAffectsSession(topics),
	)
}

func (a *app) refreshKnowledgeReader(affected bool) {
	if !affected || a.knowledge == nil || a.dialogs.runtimeReader != runtimeReaderKnowledge || !a.dialogs.readerDialog.Open() {
		return
	}
	if a.dialogs.runtimeSelection.knowledgeEntry {
		a.refreshRuntimeReader(a.knowledgeDocumentReaderQuery(a.dialogs.runtimeSelection.knowledgeTarget))
		return
	}
	a.refreshRuntimeReader(a.knowledgeEntriesReaderQuery())
}

func (a *app) refreshHooksReader(affected bool) {
	if affected && a.hooks != nil && a.dialogs.runtimeReader == runtimeReaderHooks && a.dialogs.readerDialog.Open() {
		a.refreshRuntimeReader(a.hooksReaderQuery())
	}
}

func (a *app) refreshScheduleReader(affected bool) {
	if affected && a.schedules != nil && a.dialogs.runtimeReader == runtimeReaderSchedules && a.dialogs.readerDialog.Open() {
		a.refreshRuntimeReader(a.schedulesReaderQuery())
	}
}

func (a *app) refreshMCPReader(affected bool) {
	if !affected || a.mcp == nil || !a.dialogs.readerDialog.Open() {
		return
	}
	switch a.dialogs.runtimeReader {
	case runtimeReaderMCPServers:
		a.refreshRuntimeReader(a.mcpServersReaderQuery())
	case runtimeReaderMCPTools:
		a.refreshRuntimeReader(a.mcpToolsReaderQuery(a.dialogs.mcpToolServer))
	}
}

func (a *app) refreshSkillReader(affected bool) {
	if !affected || a.skills == nil || !a.dialogs.readerDialog.Open() {
		return
	}
	switch a.dialogs.runtimeReader {
	case runtimeReaderDiscoveredSkills:
		a.refreshRuntimeReader(a.discoveredSkillsReaderQuery())
	case runtimeReaderManagedSkills:
		a.refreshRuntimeReader(a.managedSkillsReaderQuery())
	case runtimeReaderSkillProposals:
		a.refreshRuntimeReader(a.skillProposalsReaderQuery())
	}
}

func (a *app) refreshGoalReader(affected bool) {
	if affected && a.goals != nil && a.dialogs.runtimeReader == runtimeReaderGoal && a.dialogs.readerDialog.Open() {
		a.refreshRuntimeReader(a.goalReaderQuery())
	}
}

func (a *app) refreshModelReader(affected bool) {
	if !affected {
		return
	}
	if a.dialogs.modelDialog.Open() {
		a.loadModelPicker(false)
	}
	if !a.dialogs.readerDialog.Open() {
		return
	}
	switch a.dialogs.runtimeReader {
	case runtimeReaderModels:
		a.refreshRuntimeReader(a.modelsReaderQuery())
	case runtimeReaderModelRoles:
		if a.modelConfig != nil {
			a.refreshRuntimeReader(a.modelRolesReaderQuery())
		}
	case runtimeReaderProviders:
		if a.modelConfig != nil {
			a.refreshRuntimeReader(a.providersReaderQuery())
		}
	}
}

func (a *app) refreshApprovalReader(affected bool) {
	if affected && a.dialogs.runtimeReader == runtimeReaderApprovalRules && a.dialogs.readerDialog.Open() {
		a.refreshRuntimeReader(a.approvalRulesReaderQuery())
	}
}

func (a *app) refreshAgentMemoryReader(affected bool) {
	if affected && a.agentMemory != nil && a.dialogs.runtimeReader == runtimeReaderAgentMemory && a.dialogs.readerDialog.Open() {
		a.refreshRuntimeReader(a.agentMemoryReaderQuery(a.dialogs.runtimeSelection.agentMemoryTarget))
	}
}

func (a *app) refreshRuntimeReader(query runtimeReaderQuery) {
	read := query.read
	query.read = func(ctx context.Context) (readerDocument, error) {
		failures := 0
		for {
			document, err := read(ctx)
			if err == nil || !retry.IsReconnectable(err) {
				return document, err
			}
			failures++
			if err := runtimeRecoveryBackoff.Wait(ctx, failures); err != nil {
				return readerDocument{}, err
			}
		}
	}
	a.executeRuntimeReaderQuery(query)
}

func goalInvalidationAffectsSession(event changefeed.Event, sessionID string) bool {
	if event.Type == changefeed.Resync {
		return containsTopic(event.Topics, changefeed.GoalsChanged)
	}
	return changefeed.Topic(event.Type) == changefeed.GoalsChanged &&
		(len(event.SessionIDs) == 0 || containsString(event.SessionIDs, sessionID))
}

func (a *app) applySessionInvalidation(catalogChanged, currentSessionChanged bool) {
	if catalogChanged && a.dialogs.sessionDialog.Open() {
		a.dialogs.sessionCenter.Reset()
		a.loadSessionPage("", false)
	}
	if !currentSessionChanged {
		return
	}
	a.session.invalidated = true
	if a.execution.conversation.Phase() == agent.ConversationRunning || a.execution.following || a.execution.pendingCancel != nil ||
		a.operations.Active(sessionChangeOperation) {
		return
	}
	a.refreshInvalidatedSession(false)
}

func invalidatesSessionCatalog(event changefeed.Event) bool {
	if event.Type == changefeed.Resync {
		return containsTopic(event.Topics, changefeed.SessionsChanged) ||
			containsTopic(event.Topics, changefeed.RunsChanged)
	}
	return event.Type == changefeed.EventType(changefeed.SessionsChanged) ||
		event.Type == changefeed.EventType(changefeed.RunsChanged)
}

func resyncAffectsSession(topics []changefeed.Topic) bool {
	return slices.ContainsFunc(topics, func(topic changefeed.Topic) bool {
		return topic == changefeed.SessionsChanged || topic == changefeed.RunsChanged ||
			topic == changefeed.PlanChanged || topic == changefeed.GoalsChanged ||
			topic == changefeed.InterruptsChanged
	})
}

func invalidationAffectsSession(event changefeed.Event, sessionID, runID string) bool {
	if event.Type == changefeed.Resync {
		return resyncAffectsSession(event.Topics)
	}
	switch changefeed.Topic(event.Type) {
	case changefeed.SessionsChanged:
		return len(event.SessionIDs) == 0 || containsString(event.SessionIDs, sessionID)
	case changefeed.PlanChanged, changefeed.GoalsChanged:
		return len(event.SessionIDs) == 0 || containsString(event.SessionIDs, sessionID)
	case changefeed.RunsChanged, changefeed.InterruptsChanged:
		if len(event.SessionIDs) != 0 {
			return containsString(event.SessionIDs, sessionID)
		}
		return len(event.RunIDs) == 0 || containsString(event.RunIDs, runID)
	default:
		return false
	}
}

func (a *app) refreshInvalidatedSession(settleAfter bool) {
	sessionID := a.session.current.ID
	a.session.invalidated = false
	started := a.runOperation(sessionInvalidationOperation, false,
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			return a.readInvalidatedSession(ctx, sessionID)
		},
		func(snapshot agent.SessionSnapshot, err error) {
			if a.session.current.ID != sessionID {
				return
			}
			if a.session.invalidated {
				a.refreshInvalidatedSession(settleAfter)
				return
			}
			if err != nil {
				a.session.invalidated = true
				if errors.Is(err, agent.ErrSessionNotFound) && a.execution.conversation.Phase() == agent.ConversationIdle && !a.execution.following {
					a.message("the active session was deleted; creating a replacement")
					a.replaceDeletedSessionInWorkspace(a.session.current.Workspace.Path)
					return
				}
				a.message("refresh session after runtime change failed: " + err.Error())
				return
			}
			if a.execution.conversation.Phase() == agent.ConversationRunning || a.execution.following {
				a.session.invalidated = true
				return
			}
			conversationMatches := a.execution.conversation.MatchesSnapshot(snapshot)
			sessionMatches := a.session.current.Equal(snapshot.Session)
			if conversationMatches && a.session.current.Workspace == snapshot.Session.Workspace {
				if !sessionMatches {
					a.installSessionMetadata(snapshot.Session)
				}
			} else {
				if err := a.installSnapshot(snapshot); err != nil {
					a.message("refresh session after runtime change failed: " + err.Error())
					return
				}
			}
			if settleAfter && a.execution.conversation.Phase() == agent.ConversationIdle {
				a.finishFollowing()
				return
			}
			if !conversationMatches || !sessionMatches {
				a.message("session refreshed after runtime change")
			}
		},
	)
	if !started {
		a.session.invalidated = true
	}
}

func (a *app) readInvalidatedSession(ctx context.Context, sessionID string) (agent.SessionSnapshot, error) {
	failures := 0
	for {
		snapshot, err := a.runtime.GetSession(ctx, sessionID)
		if err == nil || !retry.IsReconnectable(err) {
			return snapshot, err
		}
		failures++
		if err := runtimeRecoveryBackoff.Wait(ctx, failures); err != nil {
			return agent.SessionSnapshot{}, err
		}
	}
}

func (a *app) installSessionMetadata(session agent.Session) {
	a.setActiveSession(session)
	a.dialogs.sessionCenter.Upsert(session)
}

// dismissInteractionProjection drops only the obsolete terminal-side answer
// draft. It never answers or cancels the runtime interaction.
func (a *app) dismissInteractionProjection() {
	a.clearApprovalProjection()
	a.dialogs.questionnaire = nil
	a.dialogs.interactionReview = nil
	if a.dialogs.approvalDialog != nil {
		a.dialogs.approvalDialog.Dismiss()
	}
	if a.dialogs.questionDialog != nil {
		a.dialogs.questionDialog.Controller().Dismiss()
		a.dialogs.questionDialog = nil
	}
	if a.dialogs.reviewDialog != nil {
		a.dialogs.reviewDialog.Controller().Dismiss()
		a.dialogs.reviewDialog = nil
	}
}
