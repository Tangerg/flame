package terminal

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
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
	a.reader.follow(event)
	a.loadPluginViews(event.Type == protocol.RuntimePluginsChanged)
	a.followModelChanges(event.Type == protocol.RuntimeModelsChanged)
	a.lifecycle.applySessionInvalidation(
		invalidatesSessionCatalog(event),
		invalidationAffectsSession(event, a.lifecycle.state.current.ID, a.runs.conversation().RunID()),
	)
}

func (a *app) applyRuntimeResync(topics []protocol.RuntimeTopic) {
	a.reader.follow(changefeed.Event{Type: protocol.RuntimeResync, Topics: topics})
	a.loadPluginViews(slices.Contains(topics, protocol.TopicPluginsChanged))
	a.followModelChanges(slices.Contains(topics, protocol.TopicModelsChanged))
	a.lifecycle.applySessionInvalidation(
		invalidatesSessionCatalog(changefeed.Event{Type: protocol.RuntimeResync, Topics: topics}),
		resyncAffectsSession(topics),
	)
}

func (a *app) followModelChanges(affected bool) {
	if affected && a.screens.options.models.dialog.Open() {
		a.screens.options.refreshModelPicker()
	}
}

func goalInvalidationAffectsSession(event changefeed.Event, sessionID string) bool {
	if event.Type == protocol.RuntimeResync {
		return slices.Contains(event.Topics, protocol.TopicGoalsChanged)
	}
	return event.Type == protocol.RuntimeGoalsChanged &&
		(len(event.SessionIDs) == 0 || slices.Contains(event.SessionIDs, sessionID))
}

func (l *sessionLifecycle) applySessionInvalidation(catalogChanged, currentSessionChanged bool) {
	if catalogChanged && l.sessions.dialog.Open() {
		l.sessions.pane.Reset()
		l.sessions.loadSessionPage("", false)
	}
	if !currentSessionChanged {
		return
	}
	l.state.invalidated = true
	if l.runs.conversation().Phase() == conversation.Running || l.runs.following() || l.runs.cancelPending() ||
		l.tasks.operations.Active(sessionChangeOperation) {
		return
	}
	l.refreshInvalidatedSession(false)
}

func invalidatesSessionCatalog(event changefeed.Event) bool {
	if event.Type == protocol.RuntimeResync {
		return slices.Contains(event.Topics, protocol.TopicSessionsChanged) ||
			slices.Contains(event.Topics, protocol.TopicRunsChanged)
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
		return len(event.SessionIDs) == 0 || slices.Contains(event.SessionIDs, sessionID)
	case protocol.RuntimePlanChanged, protocol.RuntimeGoalsChanged:
		return len(event.SessionIDs) == 0 || slices.Contains(event.SessionIDs, sessionID)
	case protocol.RuntimeRunsChanged, protocol.RuntimeInterruptsChanged:
		if len(event.SessionIDs) != 0 {
			return slices.Contains(event.SessionIDs, sessionID)
		}
		return len(event.RunIDs) == 0 || slices.Contains(event.RunIDs, runID)
	default:
		return false
	}
}

// refreshInvalidatedSession normally defers replacement while a trusted live
// stream owns the projection. A settlement fence means Runtime has already
// returned a terminal fact, so the cold read must replace even a locally stale
// running phase before queued work can be admitted.
func (l *sessionLifecycle) refreshInvalidatedSession(settlementFence bool) {
	sessionID := l.state.current.ID
	l.state.invalidated = false
	read := func(ctx context.Context) (conversation.SessionSnapshot, error) {
		return l.readInvalidatedSession(ctx, sessionID)
	}
	apply := func(snapshot conversation.SessionSnapshot, err error) {
		l.applyInvalidatedSessionRefresh(sessionID, settlementFence, snapshot, err)
	}
	// Every authoritative refresh fences queued Run admission. Settlement
	// refreshes additionally replace an ordinary read already in flight so a
	// weaker coalesced request cannot discard that obligation.
	started := l.tasks.sessionFence(sessionInvalidationOperation, settlementFence, read, apply)
	if !started {
		l.state.invalidated = true
	}
}

// applyInvalidatedSessionRefresh reconciles one authoritative read on the UI
// loop after its operation lease has been released. Successful ordinary reads
// explicitly reopen queued admission; failed reads leave invalidation asserted.
func (l *sessionLifecycle) applyInvalidatedSessionRefresh(
	sessionID string,
	settlementFence bool,
	snapshot conversation.SessionSnapshot,
	err error,
) {
	if l.state.current.ID != sessionID {
		return
	}
	if l.state.invalidated {
		l.refreshInvalidatedSession(settlementFence)
		return
	}
	if err != nil {
		l.state.invalidated = true
		if errors.Is(err, conversation.ErrSessionNotFound) && l.runs.conversation().Phase() == conversation.Idle && !l.runs.following() {
			l.root.message("the active session was deleted; creating a replacement")
			l.replaceDeletedSessionInWorkspace(l.state.current.Workspace.Path)
			return
		}
		l.root.message("refresh session after runtime change failed: " + err.Error())
		return
	}
	if !settlementFence && (l.runs.conversation().Phase() == conversation.Running || l.runs.following()) {
		l.state.invalidated = true
		return
	}
	conversationMatches := l.runs.conversation().MatchesSnapshot(snapshot)
	sessionMatches := l.state.current.Equal(snapshot.Session)
	if conversationMatches && l.state.current.Workspace == snapshot.Session.Workspace {
		if !sessionMatches {
			l.installSessionMetadata(snapshot.Session)
		}
	} else {
		if err := l.installSnapshot(snapshot); err != nil {
			l.state.invalidated = true
			l.root.message("refresh session after runtime change failed: " + err.Error())
			return
		}
	}
	if settlementFence && l.runs.conversation().Phase() == conversation.Idle {
		l.runs.finishFollowing()
		return
	}
	if !conversationMatches || !sessionMatches {
		l.root.message("session refreshed after runtime change")
	}
	if !settlementFence {
		l.runs.drainQueue()
	}
}

func (l *sessionLifecycle) readInvalidatedSession(ctx context.Context, sessionID string) (conversation.SessionSnapshot, error) {
	failures := 0
	for {
		snapshot, err := l.runtime.GetSession(ctx, sessionID)
		if err == nil || !retry.IsReconnectable(err) {
			return snapshot, err
		}
		failures++
		if err := runtimeRecoveryBackoff.Wait(ctx, failures); err != nil {
			return conversation.SessionSnapshot{}, err
		}
	}
}

func (l *sessionLifecycle) installSessionMetadata(session conversation.Session) {
	l.setActiveSession(session)
	l.sessions.pane.Replace(session)
}
