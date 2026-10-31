package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// statusWaitingForAnswers is the status every projection shows while a Run waits on
// the operator's interrupt answers.
const statusWaitingForAnswers = "waiting for your answers"

func (l *sessionLifecycle) restore(snapshot conversation.SessionSnapshot) {
	if err := l.runs.restoreSnapshot(snapshot); err != nil {
		l.runs.fail(err)
		return
	}
	if err := presentSnapshot(l.root.currentTranscript(), snapshot); err != nil {
		l.runs.fail(err)
		return
	}
	l.restoreActivity(snapshot)
	l.steering.restoreSteerReceipts(snapshot)
}

func presentSnapshot(view *transcriptView, snapshot conversation.SessionSnapshot) error {
	view.SetRuns(snapshot.Runs)
	for _, block := range snapshot.Transcript {
		var event conversation.Event = conversation.BlockCompleted{Block: block}
		if block.Status == conversation.BlockStatusRunning {
			event = conversation.BlockStarted{Block: block}
		}
		if err := view.Apply(event); err != nil {
			return fmt.Errorf("restore transcript block %s: %w", block.ID, err)
		}
	}
	if err := view.reconcilePendingQuestions(snapshot.Interrupts); err != nil {
		return fmt.Errorf("restore pending questions: %w", err)
	}
	view.SealToolGroups()
	return nil
}

type sessionProjection struct {
	conversation *conversation.Conversation
	transcript   *transcriptView
}

func (s sessionProjection) close() {
	if s.transcript != nil {
		s.transcript.Close()
	}
}

func (l *sessionLifecycle) projectSession(snapshot conversation.SessionSnapshot, attached *conversation.SegmentStream) (sessionProjection, error) {
	if err := snapshot.Validate(); err != nil {
		return sessionProjection{}, err
	}
	projection := conversation.New()
	var err error
	if active, ok := snapshot.ActiveRun(); attached != nil && ok && active.Status == protocol.RunStatusRunning {
		err = projection.RestoreAttachedSnapshot(snapshot, *attached)
	} else {
		err = projection.RestoreSnapshot(snapshot)
	}
	if err != nil {
		return sessionProjection{}, err
	}
	transcript := l.root.newTranscript()
	if err := presentSnapshot(transcript, snapshot); err != nil {
		transcript.Close()
		return sessionProjection{}, err
	}
	return sessionProjection{conversation: projection, transcript: transcript}, nil
}

func (a *app) newTranscript() *transcriptView {
	transcript := newTranscriptView(
		a.transcript.theme, a.transcript.glyphs, a.loop.Environment().Locale(), a.transcript.wheel, a.syntax,
		a.settings.UI.TranscriptRetain, a.transcript.details, a.transcript.clipboard,
	)
	transcript.images = a.transcript.images
	a.configureMarkdown(transcript)
	return transcript
}

// reconcileRunSnapshot atomically replaces the in-memory projection after a
// segment can no longer be replayed. It deliberately keeps the current stream
// operation alive: run recovery already attached the replacement stream before
// taking this snapshot, so canceling that operation here would reopen a gap.
func (l *sessionLifecycle) reconcileRunSnapshot(snapshot conversation.SessionSnapshot, stream conversation.SegmentStream) error {
	if snapshot.Session.ID != l.state.current.ID {
		return fmt.Errorf("reconcile run snapshot: session %s does not match %s", snapshot.Session.ID, l.state.current.ID)
	}
	projection, err := l.projectSession(snapshot, &stream)
	if err != nil {
		return fmt.Errorf("reconcile run snapshot: %w", err)
	}

	l.prepareSessionProjectionReplacement(snapshot.Session, projection.conversation)
	l.setActiveSession(snapshot.Session)
	l.runs.adoptProjection(projection.conversation)
	previousTranscript := l.root.presentTranscript(projection.transcript)
	l.header.SetUsage(projection.conversation.Usage())
	l.header.SetGoal(snapshot.Goal)
	l.activity.Set(projection.conversation.PlanItems())
	l.status.setRunningDescendants(projection.conversation.RunningDescendants())
	l.prompt.SetBusy(projection.conversation.Busy())
	previousTranscript.Close()
	l.steering.receipts.observeSnapshot(snapshot)

	resumed, err := l.runs.resumeReconciled(snapshot)
	if err != nil || !resumed {
		return err
	}
	l.steering.restoreSteerReceipts(snapshot)
	l.runs.syncAnimation()
	return nil
}

func (l *sessionLifecycle) restoreActivity(snapshot conversation.SessionSnapshot) {
	current := l.runs.conversation()
	l.activity.Set(current.PlanItems())
	l.header.SetUsage(current.Usage())
	l.header.SetGoal(snapshot.Goal)
	l.status.setRunningDescendants(current.RunningDescendants())
	l.prompt.SetBusy(current.Busy())
	l.runs.resumeRestored(snapshot)
}

func (r *runs) observeCurrentRunStatus() {
	run, ok := r.execution.conversation.CurrentRun()
	if ok {
		r.status.observeRun(run)
	}
}

func (r *runs) settleCurrentRunStatus() {
	run, ok := r.execution.conversation.CurrentRun()
	if ok {
		r.status.settled(run)
	}
}

func displayTitle(session conversation.Session) string {
	if strings.TrimSpace(session.Title) == "" {
		return "untitled"
	}
	return session.Title
}

func (l *sessionLifecycle) setActiveSession(session conversation.Session) {
	l.state.current = session
	l.root.presentSession(session)
}

// presentSession shows session in the header, banner, prompt and title.
func (a *app) presentSession(session conversation.Session) {
	a.header.SetSession(session)
	a.brand.SetSession(session)
	a.brand.SetOptions(a.displayOptions())
	if a.prompt != nil {
		a.prompt.SetOptions(a.displayOptions())
	}
	a.setWindowTitle()
}

func (a *app) displayOptions() authoring.RunOptions {
	return displayRunOptions(a.options, a.lifecycle.state.current)
}
