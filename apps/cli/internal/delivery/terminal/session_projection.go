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

func (a *app) restore(snapshot conversation.SessionSnapshot) {
	if err := a.runs.conversation().RestoreSnapshot(snapshot); err != nil {
		a.runs.fail(err)
		return
	}
	if err := presentSnapshot(a.transcript, snapshot); err != nil {
		a.runs.fail(err)
		return
	}
	a.restoreActivity(snapshot)
	a.steering.restoreSteerReceipts(snapshot)
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

func (a *app) projectSession(snapshot conversation.SessionSnapshot, attached *conversation.SegmentStream) (sessionProjection, error) {
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
	transcript := a.newTranscript()
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
func (a *app) reconcileRunSnapshot(snapshot conversation.SessionSnapshot, stream conversation.SegmentStream) error {
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
	a.runs.adoptProjection(projection.conversation)
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
	a.steering.receipts.observeSnapshot(snapshot)

	resumed, err := a.runs.resumeReconciled(snapshot)
	if err != nil || !resumed {
		return err
	}
	a.steering.restoreSteerReceipts(snapshot)
	a.runs.syncAnimation()
	return nil
}

func (a *app) restoreActivity(snapshot conversation.SessionSnapshot) {
	current := a.runs.conversation()
	a.activity.Set(current.PlanItems())
	a.header.SetUsage(current.Usage())
	a.header.SetGoal(snapshot.Goal)
	a.status.setRunningDescendants(current.RunningDescendants())
	a.prompt.SetBusy(current.Busy())
	a.runs.resumeRestored(snapshot)
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

func (a *app) setActiveSession(session conversation.Session) {
	a.session.current = session
	a.header.SetSession(session)
	a.brand.SetSession(session)
	a.brand.SetOptions(a.displayOptions())
	if a.prompt != nil {
		a.prompt.SetOptions(a.displayOptions())
	}
	a.setWindowTitle()
}

func (a *app) displayOptions() authoring.RunOptions {
	return displayRunOptions(a.options, a.session.current)
}
