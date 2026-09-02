package terminal

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

var runtimeRecoveryBackoff = func() retry.Backoff {
	backoff, err := retry.NewBackoff(100*time.Millisecond, 5*time.Second)
	if err != nil {
		panic("invalid static runtime recovery backoff: " + err.Error())
	}
	return backoff
}()

func (a *app) applyRuntimeInvalidation(event changefeed.Event) {
	if event.Type == protocol.RuntimeResync {
		a.applyRuntimeResync(event.Topics)
		return
	}
	a.refreshGoalReader(goalInvalidationAffectsSession(event, a.session.current.ID))
	a.refreshSkillReader(event.Type == protocol.RuntimeSkillsChanged)
	a.refreshMCPReader(event.Type == protocol.RuntimeMCPChanged)
	a.refreshScheduleReader(event.Type == protocol.RuntimeSchedulesChanged)
	a.refreshKnowledgeReader(event.Type == protocol.RuntimeKnowledgeChanged)
	a.refreshHooksReader(event.Type == protocol.RuntimeHooksChanged)
	a.refreshModelReader(event.Type == protocol.RuntimeModelsChanged)
	a.refreshApprovalReader(event.Type == protocol.RuntimeApprovalsChanged)
	a.refreshAgentMemoryReader(event.Type == protocol.RuntimeAgentMemoryChanged)
	a.applySessionInvalidation(
		invalidatesSessionCatalog(event),
		invalidationAffectsSession(event, a.session.current.ID, a.execution.conversation.RunID()),
	)
}

func (a *app) applyRuntimeResync(topics []protocol.RuntimeTopic) {
	a.refreshGoalReader(containsTopic(topics, protocol.TopicGoalsChanged))
	a.refreshSkillReader(containsTopic(topics, protocol.TopicSkillsChanged))
	a.refreshMCPReader(containsTopic(topics, protocol.TopicMCPChanged))
	a.refreshScheduleReader(containsTopic(topics, protocol.TopicSchedulesChanged))
	a.refreshKnowledgeReader(containsTopic(topics, protocol.TopicKnowledgeChanged))
	a.refreshHooksReader(containsTopic(topics, protocol.TopicHooksChanged))
	a.refreshModelReader(containsTopic(topics, protocol.TopicModelsChanged))
	a.refreshApprovalReader(containsTopic(topics, protocol.TopicApprovalsChanged))
	a.refreshAgentMemoryReader(containsTopic(topics, protocol.TopicAgentMemoryChanged))
	a.applySessionInvalidation(
		invalidatesSessionCatalog(changefeed.Event{Type: protocol.RuntimeResync, Topics: topics}),
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
	if event.Type == protocol.RuntimeResync {
		return containsTopic(event.Topics, protocol.TopicGoalsChanged)
	}
	return event.Type == protocol.RuntimeGoalsChanged &&
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
	if event.Type == protocol.RuntimeResync {
		return containsTopic(event.Topics, protocol.TopicSessionsChanged) ||
			containsTopic(event.Topics, protocol.TopicRunsChanged)
	}
	return event.Type == protocol.RuntimeSessionsChanged ||
		event.Type == protocol.RuntimeRunsChanged
}

func resyncAffectsSession(topics []protocol.RuntimeTopic) bool {
	return slices.ContainsFunc(topics, func(topic protocol.RuntimeTopic) bool {
		return topic == protocol.TopicSessionsChanged || topic == protocol.TopicRunsChanged ||
			topic == protocol.TopicPlanChanged || topic == protocol.TopicGoalsChanged ||
			topic == protocol.TopicInterruptsChanged
	})
}

func invalidationAffectsSession(event changefeed.Event, sessionID, runID string) bool {
	if event.Type == protocol.RuntimeResync {
		return resyncAffectsSession(event.Topics)
	}
	switch event.Type {
	case protocol.RuntimeSessionsChanged:
		return len(event.SessionIDs) == 0 || containsString(event.SessionIDs, sessionID)
	case protocol.RuntimePlanChanged, protocol.RuntimeGoalsChanged:
		return len(event.SessionIDs) == 0 || containsString(event.SessionIDs, sessionID)
	case protocol.RuntimeRunsChanged, protocol.RuntimeInterruptsChanged:
		if len(event.SessionIDs) != 0 {
			return containsString(event.SessionIDs, sessionID)
		}
		return len(event.RunIDs) == 0 || containsString(event.RunIDs, runID)
	default:
		return false
	}
}

// refreshInvalidatedSession normally defers replacement while a trusted live
// stream owns the projection. A settlement fence means Runtime has already
// returned a terminal fact, so the cold read must replace even a locally stale
// running phase before queued work can be admitted.
func (a *app) refreshInvalidatedSession(settlementFence bool) {
	sessionID := a.session.current.ID
	a.session.invalidated = false
	read := func(ctx context.Context) (agent.SessionSnapshot, error) {
		return a.readInvalidatedSession(ctx, sessionID)
	}
	apply := func(snapshot agent.SessionSnapshot, err error) {
		if a.session.current.ID != sessionID {
			return
		}
		if a.session.invalidated {
			a.refreshInvalidatedSession(settlementFence)
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
		if !settlementFence && (a.execution.conversation.Phase() == agent.ConversationRunning || a.execution.following) {
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
				a.session.invalidated = true
				a.message("refresh session after runtime change failed: " + err.Error())
				return
			}
		}
		if settlementFence && a.execution.conversation.Phase() == agent.ConversationIdle {
			a.finishFollowing()
			return
		}
		if !conversationMatches || !sessionMatches {
			a.message("session refreshed after runtime change")
		}
	}
	// Settlement refreshes are an ordering fence before queued Run admission.
	// They replace an ordinary metadata refresh already in flight so a weaker
	// coalesced request cannot discard that obligation.
	var started bool
	if settlementFence {
		started = a.runSessionAdmissionFence(sessionInvalidationOperation, true, read, apply)
	} else {
		started = a.runOperation(sessionInvalidationOperation, false, read, apply)
	}
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
