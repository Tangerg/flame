package runs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	corechat "github.com/Tangerg/scope/core/chat"
)

type rootStartPreparation struct {
	command            StartCommand
	requestedSelection modelref.Selection
	session            session.Session
	initialSession     *session.Session
	draft              RootExecutionStart
	currentMessage     corechat.Message
	openingUserText    string
}

// Start validates and resolves the Session, claims the Session and working
// tree, stages execution, and commits the Run opening. That durable opening is
// the command's acceptance point; executor activation continues behind the
// package's lifecycle supervisor and cannot retain the accepted response.
func (c *Coordinator) Start(ctx context.Context, cmd StartCommand) (result StartResult, err error) {
	preparation, err := c.prepareRootStart(ctx, cmd)
	if err != nil {
		return StartResult{}, err
	}

	runAdmission, err := c.claimFreshRun(ctx, preparation.session)
	if err != nil {
		return StartResult{}, err
	}
	defer runAdmission.Release()

	draft := preparation.draft
	draft.SessionID = preparation.session.ID()
	workingContext, err := c.conversation.Read(ctx, preparation.session.ID())
	if err != nil {
		return StartResult{}, fmt.Errorf(
			"runs: read conversation for session %q: %w", preparation.session.ID(), err,
		)
	}
	workingContext = append(workingContext, preparation.currentMessage.Clone())
	draft.WorkingContext = workingContext
	execCWD, isolated, err := c.executionCWD(ctx, preparation.session)
	if err != nil {
		return StartResult{}, err
	}
	draft.CWD = execCWD
	draft.WorkspaceCWD = preparation.session.Workspace().Path()
	draft.Isolated = isolated
	draft.WorkingContext, err = c.workingContexts.ComposeWorkingContext(ctx, WorkingContextInput{
		SessionID:  preparation.session.ID(),
		CWD:        execCWD,
		PromptText: draft.Message,
		Seed:       draft.WorkingContext,
	})
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: compose working context: %w", err)
	}
	if admitErr := c.models.AdmitInput(preparation.command.ModelSelection, draft.WorkingContext); admitErr != nil {
		return StartResult{}, fmt.Errorf("%w: %w", ErrUnsupportedMedia, admitErr)
	}
	ref, err := c.rootStarts.StageRoot(ctx, draft)
	if err != nil {
		return StartResult{}, err
	}
	staged := c.segments.ownStagedExecution(ref)
	defer func() {
		err = staged.abandon(ctx, err, "staged root execution")
		if err != nil {
			result = StartResult{}
		}
	}()
	if validateForErr := staged.validateFor(preparation.session.ID()); validateForErr != nil {
		return StartResult{}, validateForErr
	}

	cmd = preparation.command
	runID := cmd.RunID
	if runID == "" {
		runID = c.newRunID()
	}
	segmentID := c.newSegmentID()
	createdAt := c.publications.nowUTC()
	modelOnlyInput := cmd.GoalIncarnationID != ""
	conversationInput := draft.WorkingContext[len(draft.WorkingContext)-1].Clone()
	sessionReplacement, err := prepareStartSessionReplacement(preparation, createdAt)
	if err != nil {
		return StartResult{}, err
	}
	// A fresh Segment owns rejection as soon as openSegment is entered. Until
	// this exact hand-off, every preparation failure remains Start's responsibility.
	ref = staged.transfer()
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:              runID,
		SegmentID:          segmentID,
		SessionID:          preparation.session.ID(),
		CWD:                preparation.session.Workspace().Path(),
		ExecutorID:         ref.ExecutorID,
		ModelSelection:     cmd.ModelSelection,
		GoalIncarnationID:  cmd.GoalIncarnationID,
		InitialSession:     preparation.initialSession,
		SessionReplacement: sessionReplacement,
		ScheduleFiring:     cmd.ScheduleFiring,
		ManualScheduleRun:  cmd.ManualScheduleRun,
		CreatedAt:          createdAt,
		OpeningUserText:    preparation.openingUserText,
		Input:              cmd.Input,
		ConversationInput:  &conversationInput,
		ModelOnlyInput:     modelOnlyInput,
		Limits:             cmd.Limits,
		Capabilities:       cmd.Capabilities,
		admission:          &runAdmission,
		DetachActivation:   true,
		BeginExecution: func(beginCtx context.Context) error {
			return c.rootStarts.BeginRoot(beginCtx, ref)
		},
	})
	if err != nil {
		return StartResult{}, c.resolveOpeningError(ctx, preparation.session.ID(), err)
	}
	c.publications.publishRunMoved(preparation.session.ID(), runID)
	userItemID := userMessageItemID(segmentID)
	if modelOnlyInput {
		userItemID = ""
	}
	return StartResult{
		RunID: runID, SegmentID: segmentID, SessionID: preparation.session.ID(),
		UserItemID: userItemID, Events: events,
	}, nil
}

func (c *Coordinator) prepareRootStart(
	ctx context.Context,
	cmd StartCommand,
) (rootStartPreparation, error) {
	if err := cmd.ValidateScheduledIdentity(); err != nil {
		return rootStartPreparation{}, err
	}
	requestedSelection := cmd.ModelSelection
	message, media, openingUserText, err := cmd.MaterializeInput()
	if err != nil {
		return rootStartPreparation{}, err
	}
	sess, initialSession, effectiveSelection, err := c.resolveSessionSelection(ctx, cmd)
	if err != nil {
		return rootStartPreparation{}, err
	}
	cmd.ModelSelection = effectiveSelection
	draft := RootExecutionStart{
		Message:                  message,
		Media:                    media,
		ModelSelection:           effectiveSelection,
		Limits:                   cmd.Limits,
		Options:                  cmd.Options,
		InterruptKinds:           cmd.Capabilities.InterruptKinds,
		ChildRunAdmissionEnabled: cmd.Capabilities.ChildRuns,
		GoalIncarnationID:        cmd.GoalIncarnationID,
	}
	currentMessage, err := MaterializeUserMessage(cmd.Input)
	if err != nil {
		return rootStartPreparation{}, err
	}
	draft.WorkingContext = []corechat.Message{currentMessage}
	if err := draft.Validate(); err != nil {
		return rootStartPreparation{}, err
	}
	if err := c.models.AdmitInput(effectiveSelection, draft.WorkingContext); err != nil {
		return rootStartPreparation{}, fmt.Errorf("%w: %w", ErrUnsupportedMedia, err)
	}
	if err := c.rootStarts.ValidateRootStart(draft); err != nil {
		return rootStartPreparation{}, err
	}
	return rootStartPreparation{
		command: cmd, requestedSelection: requestedSelection, session: sess,
		initialSession: initialSession, draft: draft, currentMessage: currentMessage,
		openingUserText: openingUserText,
	}, nil
}

func prepareStartSessionReplacement(
	preparation rootStartPreparation,
	createdAt time.Time,
) (*SessionReplacement, error) {
	if preparation.initialSession != nil || !preparation.requestedSelection.Configured() {
		return nil, nil
	}
	next, changed, err := preparation.session.Apply(
		session.Patch{Selection: &preparation.requestedSelection}, createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("runs: prepare Session model-selection replacement: %w", err)
	}
	if !changed {
		return nil, nil
	}
	return &SessionReplacement{
		ExpectedRevision: preparation.session.Revision(), State: next,
	}, nil
}

func (c *Coordinator) resolveOpeningError(ctx context.Context, sessionID string, err error) error {
	if !errors.Is(err, run.ErrSessionBusy) {
		return err
	}
	// The durable unique index rejected the INSERT, which means another writer
	// got there first. Naming that Run is the pre-admission conflict answer too.
	if active, lookupErr := c.activeRunConflict(ctx, sessionID); lookupErr == nil && active != nil {
		return active
	}
	return fmt.Errorf("%w: %w", ErrSessionBusy, err)
}

// resolveSessionSelection admits an explicitly requested selection, resolves
// the Session that owns the default, and returns the one immutable selection
// the opening will freeze. Existing Sessions never fall back to Runtime config.
func (c *Coordinator) resolveSessionSelection(
	ctx context.Context,
	cmd StartCommand,
) (session.Session, *session.Session, modelref.Selection, error) {
	requested := cmd.ModelSelection
	if err := requested.Validate(); err != nil {
		return session.Session{}, nil, modelref.Selection{}, fmt.Errorf("runs: model selection: %w", err)
	}
	if requested.Configured() {
		if err := c.AdmitSelection(requested); err != nil {
			return session.Session{}, nil, modelref.Selection{}, err
		}
	}
	sess, initial, err := c.resolveSession(
		ctx, cmd.SessionID, cmd.NewSessionID, cmd.DefaultWorkspacePath,
		cmd.NewSessionTitle, requested,
	)
	if err != nil {
		return session.Session{}, nil, modelref.Selection{}, err
	}
	effective := requested
	if !effective.Configured() {
		effective = sess.Selection()
	}
	if err := effective.Validate(); err != nil {
		return session.Session{}, nil, modelref.Selection{}, fmt.Errorf("runs: effective model selection: %w", err)
	}
	if !effective.Configured() {
		return session.Session{}, nil, modelref.Selection{}, errors.New("runs: effective model selection is required")
	}
	if effective != requested {
		if err := c.AdmitSelection(effective); err != nil {
			return session.Session{}, nil, modelref.Selection{}, err
		}
	}
	return sess, initial, effective, nil
}

func (c *Coordinator) resolveSession(
	ctx context.Context,
	id, newID, defaultWorkspacePath, title string,
	selection modelref.Selection,
) (session.Session, *session.Session, error) {
	if newID != "" {
		return c.sessionCreator.PrepareScheduled(ctx, newID, title, defaultWorkspacePath, selection)
	}
	if id == "" {
		sess, err := c.sessionCreator.Create(ctx, title, defaultWorkspacePath)
		return sess, nil, err
	}
	sess, err := c.sessionReader.Get(ctx, id)
	return sess, nil, err
}

func (c *Coordinator) claimFreshRun(ctx context.Context, sess session.Session) (ownership.RunAdmission, error) {
	runAdmission, ok := c.admission.AcquireRun(sess.ID(), sess.Workspace().Path())
	if !ok {
		// The in-process gate also guards working-tree mutations, so what it refuses is
		// not always a Run and cannot always be named.
		return ownership.RunAdmission{}, ErrRunAdmissionBusy
	}
	// A Run the Session already holds is reported WITH its identity: the caller has to
	// choose between steering it, answering it and canceling it, and it cannot choose
	// without knowing which run and what state. Waiting counts — a Run parked on a
	// person is still the Session's Run.
	active, err := c.activeRunConflict(ctx, sess.ID())
	if err != nil {
		runAdmission.Release()
		return ownership.RunAdmission{}, err
	}
	if active != nil {
		runAdmission.Release()
		return ownership.RunAdmission{}, active
	}
	return runAdmission, nil
}

// activeRunConflict reports the Session's non-terminal Run as a conflict, or nil when
// it has none. One author, because the same conflict is reachable twice: this process
// can see the Run before admission, and the durable unique index can reject the
// INSERT after another process created one.
func (c *Coordinator) activeRunConflict(ctx context.Context, sessionID string) (error, error) {
	run, found, err := c.activeRuns.ActiveRun(ctx, sessionID)
	if err != nil || !found {
		return nil, err
	}
	return &ActiveRunConflictError{RunID: run.ID(), Status: run.State().Status()}, nil
}

// executionCWD resolves where a Session's tools operate: the sandbox copy
// for an isolated session (created on first use), else the project directory.
// It fails closed when isolation is requested but unavailable — an isolated run
// must never fall back to the real tree.
func (c *Coordinator) executionCWD(ctx context.Context, sess session.Session) (cwd string, isolated bool, err error) {
	if !sess.Isolated() {
		return sess.Workspace().Path(), false, nil
	}
	if c.isolation == nil {
		return "", false, fmt.Errorf("%w: isolation is not configured", ErrIsolationUnavailable)
	}
	copyDir, err := c.isolation.Workspace(ctx, sess.ID(), sess.Workspace().Path())
	if err != nil {
		return "", false, fmt.Errorf("%w: %w", ErrIsolationUnavailable, err)
	}
	return copyDir, true, nil
}
