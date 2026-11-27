package runs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/completion"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/keylock"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
)

type Openings struct {
	commandAcceptances commandAcceptanceReader
	startAdmissions    *keylock.Set
	rootStarts         RootExecutionStarter
	conversation       ConversationReader
	workingContexts    WorkingContextComposer
	models             modelAdmitter
	modelSelector      models.Selector
	sessionReader      sessionReader
	sessionCreator     sessionCreator
	rootRuns           rootRunReader
	isolation          isolationProvider
	releases           ExecutionReleaser
	admission          *ownership.Gate
	segments           *Segments
}

type OpeningDependencies struct {
	CommandAcceptances commandAcceptanceReader
	RootStarts         RootExecutionStarter
	Releases           ExecutionReleaser
	Conversation       ConversationReader
	WorkingContexts    WorkingContextComposer
	Models             modelAdmitter
	ModelSelector      models.Selector
	Sessions           sessionReader
	SessionCreator     sessionCreator
	RootRuns           rootRunReader
	Segments           *Segments
	Admissions         *ownership.Gate
	Isolation          isolationProvider
}

func NewOpenings(deps OpeningDependencies) (*Openings, error) {
	required := []struct {
		name  string
		value any
	}{
		{"command acceptance reader", deps.CommandAcceptances},
		{"root execution starter", deps.RootStarts},
		{"execution releaser", deps.Releases},
		{"conversation reader", deps.Conversation},
		{"working context composer", deps.WorkingContexts},
		{"model admitter", deps.Models},
		{"session reader", deps.Sessions},
		{"session creator", deps.SessionCreator},
		{"root Run reader", deps.RootRuns},
		{"segment supervisor", deps.Segments},
		{"admission gate", deps.Admissions},
		{"isolation provider", deps.Isolation},
		{"model selector", deps.ModelSelector},
	}
	for _, required := range required {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("runs: %s is required", required.name)
		}
	}
	return &Openings{
		commandAcceptances: deps.CommandAcceptances,
		startAdmissions:    keylock.NewSet(),
		rootStarts:         deps.RootStarts,
		conversation:       deps.Conversation,
		workingContexts:    deps.WorkingContexts,
		models:             deps.Models,
		modelSelector:      deps.ModelSelector,
		sessionReader:      deps.Sessions,
		sessionCreator:     deps.SessionCreator,
		rootRuns:           deps.RootRuns,
		isolation:          deps.Isolation,
		releases:           deps.Releases,
		admission:          deps.Admissions,
		segments:           deps.Segments,
	}, nil
}

func (c *Openings) AdmitSelection(ctx context.Context, selection flamemodel.Selection) error {
	if err := c.models.AdmitSelection(ctx, selection); err != nil {
		if !errors.Is(err, flamemodel.ErrUnsupported) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrUnsupportedModelSelection, err)
	}
	return nil
}

func (c *Openings) WaitSessionStartable(ctx context.Context, sessionID identity.SessionID) error {
	for {
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			return err
		}
		// A Session edit can change which tree blocks the Run. Its release
		// must invalidate both the Session read and the readiness observation.
		changed := c.admission.Changes()
		sess, err := c.sessionReader.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		ready, err := c.admission.RunStartable(sess.ID(), sess.Workspace().Path())
		if err != nil {
			return err
		}
		if ready {
			active, err := c.rootRuns.ActiveRoot(ctx, sess.ID())
			if err := errors.Join(err, ctx.Err(), context.Cause(ctx)); err != nil {
				return err
			}
			if active == nil {
				select {
				case <-changed:
					continue
				default:
					return nil
				}
			}
		}
		if err := waitForAdmissionChange(ctx, changed); err != nil {
			return err
		}
	}
}

func waitForAdmissionChange(ctx context.Context, changed <-chan struct{}) error {
	ctx, span := otel.Tracer("github.com/Tangerg/flame/runtime/internal/application/agent/runs").Start(ctx, "wait for session admission")
	defer span.End()
	return completion.Wait(ctx, changed)
}

type rootStartPreparation struct {
	command            StartCommand
	requestedSelection flamemodel.Selection
	session            session.Session
	initialSession     *session.Session
	draft              RootExecutionStart
	currentMessage     corechat.Message
	promptText         string
	openingUserText    string
}

func (c *Openings) Start(ctx context.Context, cmd StartCommand) (StartResult, error) {
	if cmd.NewSession != nil {
		if !cmd.SessionID.IsZero() || cmd.Acceptance == nil {
			return StartResult{}, fmt.Errorf("%w: new Session opening requires its own target and command identity", ErrInvalidStart)
		}
		if err := cmd.NewSession.SessionID.Validate(); err != nil {
			return StartResult{}, fmt.Errorf("%w: %w", ErrInvalidStart, err)
		}
		if err := cmd.NewSession.RunID.Validate(); err != nil {
			return StartResult{}, fmt.Errorf("%w: %w", ErrInvalidStart, err)
		}
	} else if err := cmd.SessionID.Validate(); err != nil {
		return StartResult{}, fmt.Errorf("%w: %w", ErrInvalidStart, err)
	}
	if cmd.Acceptance != nil {
		if err := cmd.Acceptance.Validate(); err != nil {
			return StartResult{}, fmt.Errorf("%w: %w", ErrInvalidStart, err)
		}
		release, err := c.startAdmissions.Acquire(ctx, cmd.Acceptance.Key)
		if err != nil {
			return StartResult{}, err
		}
		defer release()
		reopened, found, err := lookupAcceptedCommand(ctx, c.commandAcceptances, *cmd.Acceptance, startOperation(cmd.Acceptance.Key, cmd.GoalIncarnationID))
		if err != nil {
			return StartResult{}, err
		}
		if found {
			return reopened, nil
		}
	}
	return c.startRoot(ctx, cmd.clone())
}

func (c *Openings) startRoot(ctx context.Context, cmd StartCommand) (result StartResult, err error) {
	runAdmission, preparation, err := c.admitRootStart(ctx, cmd)
	if err != nil {
		return StartResult{}, err
	}
	defer runAdmission.Release()

	cmd = preparation.command
	draft := preparation.draft
	draft.SessionID = preparation.session.ID()
	hasRootRun, err := c.rootRuns.HasRootRun(ctx, draft.SessionID)
	if err != nil {
		return StartResult{}, err
	}
	if cmd.NewSession != nil {
		draft.RunID = cmd.NewSession.RunID
	} else {
		draft.RunID = c.segments.newRunID()
	}
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
	draft.Project = preparation.session.Project()
	draft.Isolated = isolated
	draft.WorkingContext, err = c.workingContexts.ComposeWorkingContext(ctx, WorkingContextInput{
		SessionID:    preparation.session.ID(),
		CWD:          execCWD,
		WorkspaceCWD: draft.WorkspaceCWD,
		Project:      draft.Project,
		PromptText:   preparation.promptText,
		Seed:         draft.WorkingContext,
		FirstRun:     !hasRootRun,
	})
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: compose working context: %w", err)
	}
	request := &corechat.Request{Messages: draft.WorkingContext}
	if draft.Options != nil {
		request.Options = draft.Options.Clone()
	}
	selected, err := c.modelSelector.SelectModel(ctx, models.SelectionInput{Context: flamemodel.SelectionContext{Use: flamemodel.Interaction}, Proposed: draft.ModelSelection, Request: request.Clone()})
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: select opening model: %w", err)
	}
	if err := selected.ValidateExact(); err != nil {
		return StartResult{}, err
	}
	if err := c.AdmitSelection(ctx, selected); err != nil {
		return StartResult{}, err
	}
	draft.ModelSelection = selected
	ref, err := c.rootStarts.StageRoot(ctx, draft)
	if err != nil {
		return StartResult{}, err
	}
	staged := ownStagedExecution(c.releases, ref)
	defer func() {
		err = staged.abandon(ctx, err, "staged root execution")
		if err != nil {
			result = StartResult{}
		}
	}()

	runID := draft.RunID
	segmentID := c.segments.newSegmentID()
	createdAt := c.segments.publications.nowUTC()
	modelOnlyInput := !cmd.GoalIncarnationID.IsZero()
	conversationInput := draft.WorkingContext[len(draft.WorkingContext)-1].Clone()
	sessionWrite, err := prepareStartSessionWrite(preparation, createdAt)
	if err != nil {
		return StartResult{}, err
	}
	events, err := c.segments.openSegment(ctx, segmentSpec{
		RunID:        runID,
		SegmentID:    segmentID,
		SessionID:    preparation.session.ID(),
		WorkspaceCWD: preparation.session.Workspace().Path(),
		Isolated:     preparation.session.Isolated(),
		ExecutorID:   ref.ExecutorID,
		Opening: rootOpening{
			ModelSelection:    draft.ModelSelection,
			GoalIncarnationID: cmd.GoalIncarnationID,
			SessionWrite:      sessionWrite,
			Actor:             cmd.Actor,
			CreatedAt:         createdAt,
			OpeningUserText:   preparation.openingUserText,
			Input:             cmd.Input,
			ConversationInput: &conversationInput,
			ModelOnlyInput:    modelOnlyInput,
			Capabilities:      cmd.Capabilities,
		},
		CommandAcceptance: cmd.Acceptance,
		admission:         &runAdmission,
		staged:            staged,
		DetachActivation:  true,
		BeginExecution: func(beginCtx context.Context) error {
			return c.rootStarts.BeginRoot(beginCtx, ref)
		},
	})
	if err != nil {
		return StartResult{}, c.resolveOpeningError(ctx, preparation.session.ID(), err)
	}
	c.segments.publications.publishRunMoved(preparation.session.ID(), runID)
	userItemID := userMessageItemID(segmentID)
	if modelOnlyInput {
		userItemID = identity.ItemID{}
	}
	return StartResult{
		RunID: runID, SegmentID: segmentID, UserItemID: userItemID, Events: events,
	}, nil
}

// An edit committed between the read and the claim would stage the Segment under a
// policy no admission covers, so preparation is re-derived until the admitted
// Session is the prepared one.
func (c *Openings) admitRootStart(
	ctx context.Context,
	cmd StartCommand,
) (ownership.RunAdmission, rootStartPreparation, error) {
	for {
		preparation, err := c.prepareRootStart(ctx, cmd)
		if err != nil {
			return ownership.RunAdmission{}, rootStartPreparation{}, err
		}
		runAdmission, err := c.claimFreshRun(ctx, preparation.session)
		if err != nil {
			return ownership.RunAdmission{}, rootStartPreparation{}, err
		}
		if preparation.initialSession != nil {
			return runAdmission, preparation, nil
		}
		committed, err := c.sessionReader.Get(ctx, preparation.session.ID())
		if err != nil {
			runAdmission.Release()
			return ownership.RunAdmission{}, rootStartPreparation{}, err
		}
		if committed.Revision() == preparation.session.Revision() {
			return runAdmission, preparation, nil
		}
		runAdmission.Release()
	}
}

func (c *Openings) prepareRootStart(
	ctx context.Context,
	cmd StartCommand,
) (rootStartPreparation, error) {
	requestedSelection := cmd.ModelSelection
	currentMessage, err := MaterializeUserMessage(cmd.Input)
	if err != nil {
		return rootStartPreparation{}, err
	}
	promptText := currentMessage.Text()
	sess, initialSession, effectiveSelection, err := c.resolveSessionSelection(ctx, cmd)
	if err != nil {
		return rootStartPreparation{}, err
	}
	cmd.ModelSelection = effectiveSelection
	draft := RootExecutionStart{
		ModelSelection:           effectiveSelection,
		Options:                  cmd.Options,
		InterruptTypes:           cmd.Capabilities.InterruptTypes,
		ChildRunAdmissionEnabled: cmd.Capabilities.ChildRuns,
		GoalIncarnationID:        cmd.GoalIncarnationID,
	}
	draft.WorkingContext = []corechat.Message{currentMessage}
	if err := draft.Validate(); err != nil {
		return rootStartPreparation{}, err
	}
	return rootStartPreparation{
		command: cmd, requestedSelection: requestedSelection, session: sess,
		initialSession: initialSession, draft: draft, currentMessage: currentMessage,
		promptText: promptText, openingUserText: strings.TrimSpace(promptText),
	}, nil
}

func prepareStartSessionWrite(
	preparation rootStartPreparation,
	createdAt time.Time,
) (OpeningSessionWrite, error) {
	if preparation.initialSession != nil {
		return SessionInsert{Session: *preparation.initialSession}, nil
	}
	if !preparation.requestedSelection.Configured() {
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
	replacement, err := session.NextReplacement(preparation.session, next)
	if err != nil {
		return nil, fmt.Errorf("runs: prepare Session replacement: %w", err)
	}
	return SessionSave{Replacement: replacement}, nil
}

func (c *Openings) resolveOpeningError(ctx context.Context, sessionID identity.SessionID, err error) error {
	if !errors.Is(err, run.ErrSessionBusy) {
		return err
	}
	if conflict, lookupErr := c.activeRootConflict(ctx, sessionID); lookupErr == nil && conflict != nil {
		return conflict
	}
	return err
}

func (c *Openings) resolveSessionSelection(
	ctx context.Context,
	cmd StartCommand,
) (session.Session, *session.Session, flamemodel.Selection, error) {
	requested := cmd.ModelSelection
	if requested.Configured() {
		if err := c.AdmitSelection(ctx, requested); err != nil {
			return session.Session{}, nil, flamemodel.Selection{}, err
		}
	}
	sess, initial, err := c.resolveSession(ctx, cmd, requested)
	if err != nil {
		return session.Session{}, nil, flamemodel.Selection{}, err
	}
	effective := requested
	if !effective.Configured() {
		effective = sess.Selection()
	}
	if err := effective.ValidateExact(); err != nil {
		return session.Session{}, nil, flamemodel.Selection{}, fmt.Errorf("runs: effective model selection: %w", err)
	}
	if effective != requested {
		if err := c.AdmitSelection(ctx, effective); err != nil {
			return session.Session{}, nil, flamemodel.Selection{}, err
		}
	}
	return sess, initial, effective, nil
}

func (c *Openings) resolveSession(
	ctx context.Context,
	cmd StartCommand,
	selection flamemodel.Selection,
) (session.Session, *session.Session, error) {
	if cmd.NewSession != nil {
		return c.sessionCreator.PrepareOpening(ctx, *cmd.NewSession, selection)
	}
	sess, err := c.sessionReader.Get(ctx, cmd.SessionID)
	return sess, nil, err
}

func (c *Openings) claimFreshRun(ctx context.Context, sess session.Session) (ownership.RunAdmission, error) {
	runAdmission, ok, leaseErr := c.admission.AcquireRun(ctx, sess.ID(), sess.Workspace().Path())
	if leaseErr != nil {
		return ownership.RunAdmission{}, leaseErr
	}
	conflict, err := c.activeRootConflict(ctx, sess.ID())
	if err != nil {
		runAdmission.Release()
		return ownership.RunAdmission{}, err
	}
	if conflict != nil {
		runAdmission.Release()
		return ownership.RunAdmission{}, conflict
	}
	if !ok {
		return ownership.RunAdmission{}, ErrRunAdmissionBusy
	}
	return runAdmission, nil
}

func (c *Openings) activeRootConflict(ctx context.Context, sessionID identity.SessionID) (*ActiveRunConflictError, error) {
	root, err := c.rootRuns.ActiveRoot(ctx, sessionID)
	if err != nil || root == nil {
		return nil, err
	}
	return &ActiveRunConflictError{Root: *root}, nil
}

func (c *Openings) executionCWD(ctx context.Context, sess session.Session) (cwd string, isolated bool, err error) {
	if !sess.Isolated() {
		return sess.Workspace().Path(), false, nil
	}
	copyDir, err := c.isolation.Workspace(ctx, sess.ID(), sess.Workspace().Path())
	if err != nil {
		return "", false, fmt.Errorf("%w: %w", errIsolationUnavailable, err)
	}
	return copyDir, true, nil
}
