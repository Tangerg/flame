package delivery

import (
	"context"
	"errors"
	"fmt"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// wireSessionErr maps the session domain's not-found sentinel onto the wire
// sentinel, passing every other error through unchanged.
func wireSessionErr(err error) error {
	if errors.Is(err, session.ErrNotFound) {
		return protocol.ErrSessionNotFound
	}
	if errors.Is(err, session.ErrTitleRequired) {
		return protocol.NewFailure(protocol.ErrInvalidParams, "title must not be empty", err)
	}
	// A Session carries a workspace, so any workspace failure can surface here.
	// wireWorkspaceError owns that whole vocabulary and returns anything outside
	// it unchanged, which leaves the Session cases below intact.
	if wired := wireWorkspaceError(err); wired != err {
		return wired
	}
	if errors.Is(err, session.ErrRevisionConflict) {
		return protocol.NewFailure(protocol.ErrRevisionConflict, "the session changed after it was read", err)
	}
	if modelchoice.IsInvalid(err) {
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	}
	if errors.Is(err, modelchoice.ErrUnsupported) {
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	}
	return wirePluginError(err)
}

// ListSessions returns one page of the session list (sessions.list). A non-empty
// NextCursor is the "has more" signal — never a silent truncation. The page is
// cut by the read, so only the sessions on it are resolved.
func (s *Handler) ListSessions(ctx context.Context, q protocol.ListSessionsRequest) (*protocol.Page[protocol.Session], error) {
	limit, err := requestedPageLimit(q.Limit)
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
	view, err := s.sessions.View(ctx, id)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := presentSession(view)
	return &out, nil
}

// GetSessionSnapshot projects one Application-owned storage snapshot into the
// complete material read used by mounted clients. Capability checks apply to
// the snapshot as a whole, so no response can silently trim a waiting set or a
// child Run that the caller could not fold.
func (s *Handler) GetSessionSnapshot(ctx context.Context, in protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error) {
	snapshot, view, err := s.sessions.MaterialView(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	for _, pending := range snapshot.Interrupts {
		root, found := snapshot.Run(pending.RootRunID)
		if !found {
			return nil, fmt.Errorf("delivery: open interrupt set %q has no root Run in the snapshot", pending.RootRunID)
		}
		if gap := root.Capabilities().MissingFrom(caller); !gap.IsEmpty() {
			return nil, capabilityGap(gap)
		}
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
		input := protocol.SteerInput{RunID: accepted.RunID, SegmentID: accepted.SegmentID, UserItemID: accepted.UserItemID}
		for _, block := range accepted.SteerInput {
			input.Input = append(input.Input, presentContent(block))
		}
		out.SteerInputs = append(out.SteerInputs, input)
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
			RootRunID:  set.Pending.RootRunID,
			SessionID:  set.SessionID,
			Interrupts: presentInterrupts(set.Interrupts),
			CreatedAt:  set.Pending.CreatedAt,
		})
	}
	plan := presentStoredPlan(in.SessionID, snapshot.Plan)
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

func (s *Handler) DeleteSession(ctx context.Context, id string) error {
	// The session use case claims the addressed session and every owned
	// internal-subtask descendant before deleting their durable state atomically.
	// User-created forks remain independent conversations.
	if err := s.sessions.DeleteSession(ctx, id); err != nil {
		if errors.Is(err, rundomain.ErrSessionBusy) {
			return protocol.NewFailure(protocol.ErrSessionBusy, fmt.Sprintf("session %q or its subtask tree has a run in flight", id), err)
		}
		return wireSessionErr(err)
	}
	return nil
}

// UpdateSession applies a sessions.update edit: title (rename), model, cwd
// (relocate, gated by features.relocate), and favorite are all live. Nil
// fields are left alone; the updated session is returned. The
// dispatch layer already rejects an empty SessionID.
func (s *Handler) UpdateSession(ctx context.Context, in protocol.UpdateSessionRequest) (*protocol.Session, error) {
	var cwd *string
	if in.Workspace != nil {
		path := in.Workspace.Path
		cwd = &path
	}
	view, err := s.sessions.UpdateView(ctx, in.SessionID, sessions.Patch{
		Title: in.Title,
		ModelSelection: modelchoice.Patch{
			Provider: in.Provider, Model: in.Model, ReasoningEffort: in.ReasoningEffort,
		},
		WorkspacePath:    cwd,
		Favorite:         in.Favorite,
		Isolated:         in.Isolated,
		ExpectedRevision: in.ExpectedRevision,
	})
	if err != nil {
		if errors.Is(err, rundomain.ErrSessionBusy) {
			return nil, protocol.NewFailure(protocol.ErrSessionBusy, fmt.Sprintf("session %q has a run in flight", in.SessionID), err)
		}
		return nil, wireSessionErr(err)
	}
	out := presentSession(view)
	return &out, nil
}

// ForkSession branches a session into a fresh child that continues from the
// parent's conversation: the child inherits the
// parent's cwd and a copy of its chat history, then diverges. An optional title
// overrides the parent's title, which the child otherwise keeps.
//
// FromRunID (run-boundary fork — "branch from this run", B4) truncate-copies
// history up to and including that Run; omit it for a whole-conversation
// fork. Snapshot semantics: only terminal runs are copied; an in-flight run and
// all of its mutable history tail are excluded. Forking deletes nothing, so
// unlike rollback it needs no session_busy guard.
func (s *Handler) ForkSession(ctx context.Context, in protocol.ForkSessionRequest) (*protocol.Session, error) {
	plan, err := s.prepareFork(ctx, in)
	return applySessionCreation(ctx, plan, err)
}

func (s *Handler) prepareCreate(ctx context.Context, in protocol.CreateSessionRequest) (sessions.Creation, error) {
	return s.sessions.PrepareCreate(ctx, in.Title, s.sessionWorkspacePath(in.Workspace))
}

func (s *Handler) prepareFork(ctx context.Context, in protocol.ForkSessionRequest) (sessions.Creation, error) {
	plan, err := s.sessions.PrepareFork(ctx, sessions.ForkSpec{ParentID: in.SessionID, FromRunID: in.FromRunID, Title: in.Title})
	if err != nil {
		if in.FromRunID != "" {
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

// presentSession projects the complete Application read model into the
// selected protocol shape. It intentionally performs no filesystem, live-run,
// or model-default lookup.
func presentSession(view sessions.View) protocol.Session {
	return protocol.Session{
		ID:              view.ID,
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

// presentSessionStatus projects the application-resolved activity without
// reproducing its running/waiting precedence in Delivery.
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
