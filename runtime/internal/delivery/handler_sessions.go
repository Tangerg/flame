package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

func wireSessionErr(err error) error {
	if errors.Is(err, session.ErrNotFound) {
		return protocol.ErrSessionNotFound
	}
	if errors.Is(err, session.ErrTitleRequired) {
		return protocol.NewFailure(protocol.ErrInvalidParams, "title must not be empty", err)
	}
	if wired, ok := workspaceFailure(err); ok {
		return wired
	}
	if errors.Is(err, session.ErrRevisionConflict) {
		return protocol.NewFailure(protocol.ErrRevisionConflict, "the session changed after it was read", err)
	}
	if flamemodel.IsInvalid(err) {
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	}
	if errors.Is(err, flamemodel.ErrUnsupported) {
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	}
	return wirePluginError(err)
}

func (s *Handler) ListSessions(ctx context.Context, q protocol.ListSessionsRequest) (*protocol.Page[protocol.Session], error) {
	limit, err := pagination.ParseLimit(q.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	filter := session.AllCatalogEntries()
	var workspace *session.Workspace
	if q.Workspace != nil {
		value, workspaceErr := session.NewWorkspace(q.Workspace.Path)
		if workspaceErr != nil {
			return nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("sessions: catalog workspace: %v", workspaceErr), workspaceErr)
		}
		workspace = &value
	}
	if q.Search != "" || q.Workspace != nil {
		filter, err = session.NewCatalogFilter(q.Search, workspace)
		if err != nil {
			return nil, protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
		}
	}
	page, err := s.sessions.ListViewPage(ctx, filter, q.Cursor, limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	data := make([]protocol.Session, 0, len(page.Rows))
	for _, view := range page.Rows {
		data = append(data, presentSession(view))
	}
	return protocol.NewPageWithCursor(data, page.NextCursor), nil
}

func (s *Handler) GetSession(ctx context.Context, id string) (*protocol.Session, error) {
	sessionID, err := sessionParameter(id)
	if err != nil {
		return nil, err
	}
	view, err := s.sessions.View(ctx, sessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := presentSession(view)
	return &out, nil
}

func requireAnswerableInterrupts(snapshot sessions.MaterialSnapshot, caller rundomain.Capabilities) error {
	for _, pending := range snapshot.Interrupts {
		root, found := snapshot.Run(pending.RootRunID)
		if !found {
			return fmt.Errorf("delivery: open interrupt set %q has no root Run in the snapshot", pending.RootRunID)
		}
		if gap := root.Capabilities().MissingFrom(caller); !gap.IsEmpty() {
			return capabilityGap(gap)
		}
	}
	return nil
}

func presentSteerInput(accepted runs.CommandAcceptance) protocol.SteerInput {
	input := protocol.SteerInput{RunID: accepted.RunID.String(), SegmentID: accepted.SegmentID.String(), UserItemID: accepted.UserItemID.String()}
	for _, block := range accepted.SteerInput {
		input.Input = append(input.Input, presentContent(block))
	}
	return input
}

func (s *Handler) GetSessionSnapshot(ctx context.Context, in protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error) {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, parseErr
	}
	snapshot, view, err := s.sessions.MaterialView(ctx, sessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireAnswerableInterrupts(snapshot, caller); err != nil {
		return nil, err
	}
	out := &protocol.SessionSnapshot{
		Session:    presentSession(view),
		Items:      make([]protocol.Item, 0, len(snapshot.Items)),
		Runs:       make([]protocol.RunRef, 0, len(snapshot.Runs)),
		Interrupts: make([]protocol.PendingInterruptSet, 0, len(snapshot.Interrupts)),
	}
	for _, item := range snapshot.Items {
		out.Items = append(out.Items, presentItem(item))
	}
	for _, accepted := range snapshot.SteerInputs {
		out.SteerInputs = append(out.SteerInputs, presentSteerInput(accepted))
	}
	for _, record := range snapshot.Runs {
		if !in.IncludeDescendants && record.Lineage().IsChild() {
			continue
		}
		out.Runs = append(out.Runs, presentRun(record))
	}
	sets, err := snapshot.InterruptSets()
	if err != nil {
		return nil, err
	}
	for _, set := range sets {
		out.Interrupts = append(out.Interrupts, protocol.PendingInterruptSet{
			RootRunID:  set.Pending.RootRunID.String(),
			SessionID:  set.SessionID.String(),
			Interrupts: presentInterrupts(set.Interrupts),
			CreatedAt:  set.Pending.CreatedAt,
		})
	}
	plan := presentStoredPlan(sessionID, snapshot.Plan)
	out.Plan = &plan
	if snapshot.Goal != nil {
		out.Goal, err = presentGoal(*snapshot.Goal)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Handler) CreateSession(ctx context.Context, in protocol.CreateSessionRequest) (*protocol.Session, error) {
	plan, err := s.prepareCreate(ctx, in)
	return applySessionCreation(ctx, plan, err)
}

func (s *Handler) commitCreateSession(ctx context.Context, in protocol.CreateSessionRequest, outcome atomicOutcome) error {
	creation, err := s.prepareCreate(ctx, in)
	return outcome.commitCreation(ctx, creation, err)
}

func (s *Handler) DeleteSession(ctx context.Context, id string) error {
	var deleted struct{}
	return s.deleteSession(ctx, id, directReply(&deleted))
}

func (s *Handler) deleteSession(ctx context.Context, id string, complete operation.Reply[struct{}]) error {
	sessionID, err := sessionParameter(id)
	if err != nil {
		return err
	}
	return wireSessionMutationErr(s.sessions.DeleteSession(ctx, sessionID, complete), sessionID)
}

func (s *Handler) commitDeleteSession(ctx context.Context, in protocol.DeleteSessionRequest, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.deleteSession(ctx, in.SessionID, outcome.acknowledgement()))
}

func (s *Handler) UpdateSession(ctx context.Context, in protocol.UpdateSessionRequest) (*protocol.Session, error) {
	return completeSessionView(func(complete operation.Reply[sessions.View]) error {
		return s.updateSession(ctx, in, complete)
	})
}

func (s *Handler) commitUpdateSession(ctx context.Context, in protocol.UpdateSessionRequest, outcome atomicOutcome) error {
	reply := outcome.sessionView(func(session *protocol.Session) any { return session })
	return outcome.settleRefusal(ctx, s.updateSession(ctx, in, reply))
}

func (s *Handler) updateSession(ctx context.Context, in protocol.UpdateSessionRequest, complete operation.Reply[sessions.View]) error {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return parseErr
	}
	var cwd *string
	if in.Workspace != nil {
		path := in.Workspace.Path
		cwd = &path
	}
	err := s.sessions.Update(ctx, sessionID, sessions.Patch{
		Title: in.Title,
		ModelSelection: flamemodel.Patch{
			Provider: in.Provider, Model: in.Model, ReasoningEffort: in.ReasoningEffort,
		},
		WorkspacePath:    cwd,
		Favorite:         in.Favorite,
		Isolated:         in.Isolated,
		ExpectedRevision: in.ExpectedRevision,
	}, complete)
	return wireSessionMutationErr(err, sessionID)
}

func completeSessionView(command func(operation.Reply[sessions.View]) error) (*protocol.Session, error) {
	var view sessions.View
	if err := command(directReply(&view)); err != nil {
		return nil, err
	}
	presented := presentSession(view)
	return &presented, nil
}

func wireSessionMutationErr(err error, sessionID identity.SessionID) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, rundomain.ErrSessionBusy) {
		return protocol.NewFailure(protocol.ErrSessionBusy, fmt.Sprintf("session %q is busy", sessionID), err)
	}
	return wireSessionErr(err)
}

func (s *Handler) ForkSession(ctx context.Context, in protocol.ForkSessionRequest) (*protocol.Session, error) {
	plan, err := s.prepareFork(ctx, in)
	return applySessionCreation(ctx, plan, err)
}

func (s *Handler) commitForkSession(ctx context.Context, in protocol.ForkSessionRequest, outcome atomicOutcome) error {
	creation, err := s.prepareFork(ctx, in)
	return outcome.commitCreation(ctx, creation, err)
}

func (s *Handler) prepareCreate(ctx context.Context, in protocol.CreateSessionRequest) (sessions.Creation, error) {
	return s.sessions.PrepareCreate(ctx, in.Title, s.sessionWorkspacePath(in.Workspace))
}

func (s *Handler) prepareFork(ctx context.Context, in protocol.ForkSessionRequest) (sessions.Creation, error) {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return sessions.Creation{}, parseErr
	}
	fromRunID, err := wireIdentity("fromRunId", in.FromRunID, identity.ParseOptionalRun)
	if err != nil {
		return sessions.Creation{}, err
	}
	plan, err := s.sessions.PrepareFork(ctx, sessions.ForkSpec{ParentID: sessionID, FromRunID: fromRunID, Title: in.Title})
	if err != nil {
		if !fromRunID.IsZero() {
			err = wireBoundaryErr(err)
		}
		return sessions.Creation{}, wireSessionErr(err)
	}
	return plan, nil
}

func applySessionCreation(ctx context.Context, creation sessions.Creation, err error) (*protocol.Session, error) {
	if err != nil {
		return nil, err
	}
	if err := creation.Commit(ctx); err != nil {
		return nil, wireSessionErr(err)
	}
	creation.Publish()
	view := presentSession(creation.View)
	return &view, nil
}

func presentSession(view sessions.View) protocol.Session {
	return protocol.Session{
		ID:              view.ID.String(),
		Title:           view.Title,
		Workspace:       presentWorkspaceInfo(view.Workspace.Path, view.Workspace.ProjectRoot, view.Workspace.Missing),
		Provider:        view.Provider,
		Model:           view.Model,
		ReasoningEffort: view.ReasoningEffort,
		Status:          presentSessionStatus(view.Activity),
		CreatedAt:       view.CreatedAt,
		UpdatedAt:       view.UpdatedAt,
		Favorite:        view.Favorite,
		Isolated:        view.Isolated,
		Revision:        view.Revision,
	}
}

func presentSessionStatus(activity sessions.Activity) protocol.SessionStatus {
	switch activity {
	case sessions.ActivityRunning:
		return protocol.SessionStatusRunning
	case sessions.ActivityWaiting:
		return protocol.SessionStatusWaiting
	default:
		return protocol.SessionStatusIdle
	}
}
