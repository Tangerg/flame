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

const (
	runtimeRecoveryFirstDelay = 100 * time.Millisecond
	runtimeRecoveryMaxDelay   = 5 * time.Second
)

var runtimeRecoveryBackoff = func() retry.Backoff {
	backoff, err := retry.NewBackoff(runtimeRecoveryFirstDelay, runtimeRecoveryMaxDelay)
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
	if invalidatesSessionCatalog(event) {
		a.lifecycle.reloadSessionCatalog()
	}
	if invalidationAffectsSession(event, a.lifecycle.state.current.ID, a.runs.conversation().RunID()) {
		a.lifecycle.invalidateCurrentSession()
	}
}

func (a *app) applyRuntimeResync(topics []protocol.RuntimeTopic) {
	a.reader.follow(changefeed.Event{Type: protocol.RuntimeResync, Topics: topics})
	a.loadPluginViews(slices.Contains(topics, protocol.TopicPluginsChanged))
	a.followModelChanges(slices.Contains(topics, protocol.TopicModelsChanged))
	if invalidatesSessionCatalog(changefeed.Event{Type: protocol.RuntimeResync, Topics: topics}) {
		a.lifecycle.reloadSessionCatalog()
	}
	if resyncAffectsSession(topics) {
		a.lifecycle.invalidateCurrentSession()
	}
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

func (l *sessionLifecycle) reloadSessionCatalog() {
	if l.sessions.dialog.Open() {
		l.sessions.pane.Reset()
		l.sessions.loadSessionPage(firstSessionPage)
	}
}

func (l *sessionLifecycle) invalidateCurrentSession() {
	l.state.invalidated = true
	if l.runs.conversation().Phase() == conversation.Running || l.runs.following() || l.runs.cancelPending() ||
		l.tasks.operations.Active(sessionChangeOperation) {
		return
	}
	l.refreshInvalidatedSession(refreshAfterChange)
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

type sessionRefresh uint8

const (
	refreshAfterChange sessionRefresh = iota
	refreshSettlingRun
)

func (r sessionRefresh) settles() bool { return r == refreshSettlingRun }

func (l *sessionLifecycle) refreshInvalidatedSession(purpose sessionRefresh) {
	sessionID := l.state.current.ID
	l.state.invalidated = false
	read := func(ctx context.Context) (conversation.SessionSnapshot, error) {
		return l.readInvalidatedSession(ctx, sessionID)
	}
	apply := func(snapshot conversation.SessionSnapshot, err error) {
		l.applyInvalidatedSessionRefresh(sessionID, purpose, snapshot, err)
	}
	slot := sessionInvalidationOperation
	if purpose.settles() {
		slot = slot.superseding()
	}
	started := l.tasks.sessionFence(slot, read, apply)
	if !started {
		l.state.invalidated = true
	}
}

func (l *sessionLifecycle) applyInvalidatedSessionRefresh(
	sessionID string,
	purpose sessionRefresh,
	snapshot conversation.SessionSnapshot,
	err error,
) {
	if l.state.current.ID != sessionID {
		return
	}
	if l.state.invalidated {
		l.refreshInvalidatedSession(purpose)
		return
	}
	if err != nil {
		l.failInvalidatedRefresh(err)
		return
	}
	if !purpose.settles() && (l.runs.conversation().Phase() == conversation.Running || l.runs.following()) {
		l.state.invalidated = true
		return
	}
	changed, installed := l.installRefreshedSnapshot(snapshot)
	if !installed {
		return
	}
	if purpose.settles() && l.runs.conversation().Phase() == conversation.Idle {
		l.runs.finishFollowing()
		return
	}
	if changed {
		l.root.message("session refreshed after runtime change")
	}
	if !purpose.settles() {
		l.runs.drainQueue()
	}
}

func (l *sessionLifecycle) failInvalidatedRefresh(err error) {
	l.state.invalidated = true
	if errors.Is(err, conversation.ErrSessionNotFound) && l.runs.conversation().Phase() == conversation.Idle && !l.runs.following() {
		l.root.message("the active session was deleted; creating a replacement")
		l.replaceDeletedSessionInWorkspace(l.state.current.Workspace.Path)
		return
	}
	l.root.message("refresh session after runtime change failed: " + err.Error())
}

func (l *sessionLifecycle) installRefreshedSnapshot(snapshot conversation.SessionSnapshot) (changed, installed bool) {
	conversationMatches := l.runs.conversation().MatchesSnapshot(snapshot)
	sessionMatches := l.state.current.Equal(snapshot.Session)
	if conversationMatches && l.state.current.Workspace == snapshot.Session.Workspace {
		if !sessionMatches {
			l.installSessionMetadata(snapshot.Session)
		}
		return !sessionMatches, true
	}
	if err := l.installSnapshot(snapshot); err != nil {
		l.state.invalidated = true
		l.root.message("refresh session after runtime change failed: " + err.Error())
		return false, false
	}
	return !conversationMatches || !sessionMatches, true
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
