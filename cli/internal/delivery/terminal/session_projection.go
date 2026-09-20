package terminal

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (a *app) restore(snapshot agent.SessionSnapshot) {
	if err := a.execution.conversation.RestoreSnapshot(snapshot); err != nil {
		a.fail(err)
		return
	}
	if err := presentSnapshot(a.transcript, snapshot, a.registry); err != nil {
		a.fail(err)
		return
	}
	a.restoreActivity(snapshot)
}

func presentSnapshot(view *transcriptView, snapshot agent.SessionSnapshot, registry *extensions.Registry) error {
	view.SetRuns(snapshot.Runs)
	for _, block := range snapshot.Transcript {
		var event agent.Event = agent.BlockCompleted{Block: block}
		if block.Status == agent.BlockStatusRunning {
			event = agent.BlockStarted{Block: block}
		}
		if err := view.Apply(event, registry); err != nil {
			return fmt.Errorf("restore transcript block %s: %w", block.ID, err)
		}
	}
	if err := view.reconcilePendingQuestions(snapshot.Interactions); err != nil {
		return fmt.Errorf("restore pending questions: %w", err)
	}
	view.SealToolGroups()
	return nil
}

type sessionProjection struct {
	conversation *agent.Conversation
	transcript   *transcriptView
}

func (s sessionProjection) close() {
	if s.transcript != nil {
		s.transcript.Close()
	}
}

func (a *app) projectSession(snapshot agent.SessionSnapshot, attached *agent.SegmentStream) (sessionProjection, error) {
	if err := snapshot.Validate(); err != nil {
		return sessionProjection{}, err
	}
	conversation := agent.NewConversation()
	var err error
	if active, ok := snapshot.ActiveRun(); attached != nil && ok && active.Status == protocol.RunStatusRunning {
		err = conversation.RestoreAttachedSnapshot(snapshot, *attached)
	} else {
		err = conversation.RestoreSnapshot(snapshot)
	}
	if err != nil {
		return sessionProjection{}, err
	}
	transcript := a.newTranscript()
	if err := presentSnapshot(transcript, snapshot, a.registry); err != nil {
		transcript.Close()
		return sessionProjection{}, err
	}
	return sessionProjection{conversation: conversation, transcript: transcript}, nil
}

func (a *app) newTranscript() *transcriptView {
	transcript := newTranscriptView(
		a.transcript.theme, a.transcript.glyphs, a.transcript.wheel, a.syntax,
		a.settings.UI.TranscriptRetain, a.transcript.details, a.transcript.clipboard,
	)
	transcript.images = a.transcript.images
	return transcript
}

// reconcileRunSnapshot atomically replaces the in-memory projection after a
// segment can no longer be replayed. It deliberately keeps the current stream
// operation alive: run recovery already attached the replacement stream before
// taking this snapshot, so canceling that operation here would reopen a gap.
func (a *app) reconcileRunSnapshot(snapshot agent.SessionSnapshot, stream agent.SegmentStream) error {
	if snapshot.Session.ID != a.session.current.ID {
		return fmt.Errorf("reconcile run snapshot: session %s does not match %s", snapshot.Session.ID, a.session.current.ID)
	}
	projection, err := a.projectSession(snapshot, &stream)
	if err != nil {
		return fmt.Errorf("reconcile run snapshot: %w", err)
	}

	a.prepareSessionProjectionReplacement(snapshot.Session, projection.conversation)
	previousTranscript := a.transcript
	a.setActiveSession(snapshot.Session)
	a.execution.conversation = projection.conversation
	a.execution.projectionFailed = false
	a.transcript = projection.transcript
	a.wireTranscript(projection.transcript)
	a.shell.SetTranscript(projection.transcript)
	a.header.SetUsage(projection.conversation.Usage())
	a.header.SetGoal(snapshot.Goal)
	a.activity.Set(projection.conversation.PlanItems())
	a.status.setRunningDescendants(projection.conversation.RunningDescendants())
	a.prompt.SetBusy(projection.conversation.Busy())
	previousTranscript.Close()
	a.listenForSearch()

	switch projection.conversation.Phase() {
	case agent.ConversationRunning:
		a.execution.following = true
		a.execution.clock.start(projection.conversation.Usage().Duration, time.Now())
		active, ok := snapshot.ActiveRun()
		if !ok {
			return errors.New("reconcile run snapshot: running conversation has no active run")
		}
		a.showRecoveredRunStatus("reconnected", active)
	case agent.ConversationWaiting:
		a.execution.following = false
		if a.dialogs.interactionReview == nil {
			a.openInteractions(projection.conversation.Interactions())
		}
		a.observeCurrentRunStatus()
		a.status.note("waiting for your answers")
	case agent.ConversationIdle:
		a.execution.following = false
		if projection.conversation.Outcome().Status != "" {
			a.settleCurrentRunStatus()
		}
		if a.drainQueue() {
			return nil
		}
		if projection.conversation.Outcome().Status != "" {
			a.raiseAttention(outcomeAttention(projection.conversation.Outcome()))
		}
	default:
		return errors.New("reconcile run snapshot: unknown conversation phase")
	}
	a.syncAnimation()
	return nil
}

func (a *app) restoreActivity(snapshot agent.SessionSnapshot) {
	a.activity.Set(a.execution.conversation.PlanItems())
	a.header.SetUsage(a.execution.conversation.Usage())
	a.header.SetGoal(snapshot.Goal)
	a.status.setRunningDescendants(a.execution.conversation.RunningDescendants())
	a.prompt.SetBusy(a.execution.conversation.Busy())
	switch a.execution.conversation.Phase() {
	case agent.ConversationWaiting:
		if a.dialogs.interactionReview == nil {
			a.openInteractions(a.execution.conversation.Interactions())
		}
		a.observeCurrentRunStatus()
		a.status.note("waiting for your answers")
	case agent.ConversationRunning:
		active, ok := snapshot.ActiveRun()
		if !ok {
			a.fail(errors.New("session snapshot has a running conversation without an active run"))
			return
		}
		a.execution.clock.start(a.execution.conversation.Usage().Duration, time.Now())
		a.showRecoveredRunStatus("reconnecting", active)
		a.followRecoveredSession()
	case agent.ConversationIdle:
		if a.execution.conversation.Outcome().Status != "" {
			a.settleCurrentRunStatus()
		}
	default:
		a.fail(errors.New("session snapshot has an unknown conversation phase"))
	}
}

func (a *app) showRecoveredRunStatus(activity string, run agent.Run) {
	a.status.observeRun(run)
	a.status.progress(agent.RunProgress{Activity: activity})
}

func (a *app) observeCurrentRunStatus() {
	run, ok := a.execution.conversation.CurrentRun()
	if ok {
		a.status.observeRun(run)
	}
}

func (a *app) settleCurrentRunStatus() {
	run, _ := a.execution.conversation.CurrentRun()
	run.Outcome = a.execution.conversation.Outcome()
	run.Usage = a.execution.conversation.Usage()
	a.status.settled(run)
}

func displayTitle(session agent.Session) string {
	if strings.TrimSpace(session.Title) == "" {
		return "untitled"
	}
	return session.Title
}

func (a *app) setActiveSession(session agent.Session) {
	a.session.current = session
	a.header.SetSession(session)
	a.brand.SetSession(session)
	a.brand.SetOptions(a.displayOptions())
	if a.prompt != nil {
		a.prompt.SetOptions(a.displayOptions())
	}
	a.setWindowTitle()
}

func (a *app) displayOptions() agent.RunOptions {
	return displayRunOptions(a.options, a.session.current)
}
