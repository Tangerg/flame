package runtimefixture

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/cli/internal/exactint"
)

var errSessionRevisionExhausted = errors.New("mock: session revision is exhausted")

type sessionRevisionChanges uint64

func sessionEventRevisionChange() sessionRevisionChanges {
	return 1
}

func sessionEventRevisionChanges(count int) sessionRevisionChanges {
	if count < 0 {
		panic("mock: negative session event revision count")
	}
	return sessionRevisionChanges(count)
}

func sessionStatusRevisionChanges(session *sessionState, status agent.SessionStatus) sessionRevisionChanges {
	if session.meta.Status == status {
		return 0
	}
	return 1
}

func (c sessionRevisionChanges) plus(other sessionRevisionChanges) sessionRevisionChanges {
	if uint64(other) > math.MaxUint64-uint64(c) {
		panic("mock: session revision change count overflow")
	}
	return c + other
}

func (s *sessionState) requireRevisionCapacity(changes sessionRevisionChanges) error {
	current, err := exactint.Restore(s.meta.Revision)
	if err != nil {
		return fmt.Errorf("mock: session revision: %w", err)
	}
	_, err = current.Advance(uint64(changes))
	return classifySessionRevisionAdvance(err)
}

func (s *sessionState) commitMeta(candidate agent.Session) error {
	committed, err := nextSessionMeta(s.meta, candidate)
	if err != nil {
		return err
	}
	s.meta = committed
	return nil
}

func nextSessionMeta(current, candidate agent.Session) (agent.Session, error) {
	revision, err := exactint.Restore(current.Revision)
	if err != nil {
		return agent.Session{}, fmt.Errorf("mock: session revision: %w", err)
	}
	next, err := revision.Next()
	if err := classifySessionRevisionAdvance(err); err != nil {
		return agent.Session{}, err
	}
	candidate.Revision = next.Value()
	return candidate, nil
}

func classifySessionRevisionAdvance(err error) error {
	if errors.Is(err, exactint.ErrExhausted) {
		return errSessionRevisionExhausted
	}
	return err
}

func (r *Runtime) ListSessions(ctx context.Context, query agent.SessionQuery) (agent.SessionPage, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.SessionPage{}, err
	}
	query, err := query.Normalize()
	if err != nil {
		return agent.SessionPage{}, fmt.Errorf("mock: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]agent.Session, 0, len(r.sessions))
	needle := strings.ToLower(query.Search)
	workspace := query.Workspace
	for _, state := range r.sessions {
		if workspace != "" && state.meta.Workspace.Path != workspace {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(state.meta.Title+"\n"+state.meta.Workspace.Path+"\n"+state.meta.Workspace.ProjectRoot), needle) {
			continue
		}
		items = append(items, state.meta)
	}
	slices.SortStableFunc(items, func(a, b agent.Session) int {
		if a.Favorite != b.Favorite {
			if a.Favorite {
				return -1
			}
			return 1
		}
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})

	offset, err := pageOffset("session", query.Cursor, len(items))
	if err != nil {
		return agent.SessionPage{}, err
	}
	limit, err := query.PageSize.Rows()
	if err != nil {
		return agent.SessionPage{}, fmt.Errorf("mock: %w", err)
	}
	end := min(offset+limit, len(items))
	page := agent.SessionPage{Items: slices.Clone(items[offset:end])}
	if end < len(items) {
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}

func pageOffset(collection, cursor string, length int) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 || offset > length {
		return 0, fmt.Errorf("mock: invalid %s page cursor %q", collection, cursor)
	}
	return offset, nil
}

func (r *Runtime) GetSession(ctx context.Context, id string) (agent.SessionSnapshot, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.SessionSnapshot{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.sessions[id]
	if !ok {
		return agent.SessionSnapshot{}, fmt.Errorf("%w: %s", agent.ErrSessionNotFound, id)
	}
	snapshot := agent.SessionSnapshot{
		Session:    state.meta,
		Transcript: make([]agent.Block, len(state.items)),
		Runs:       make([]agent.Run, 0, len(state.runs)),
		Plan:       cloneCommittedPlan(state.plan),
	}
	for i, item := range state.items {
		snapshot.Transcript[i] = item.block.Clone()
	}
	for _, runID := range state.runs {
		if run := r.runs[runID]; run != nil {
			snapshot.Runs = append(snapshot.Runs, projectRun(run))
		}
	}
	if active := r.runs[state.active]; active != nil {
		snapshot.Interactions = agent.CloneInteractions(active.interactions)
	}
	if err := snapshot.Validate(); err != nil {
		return agent.SessionSnapshot{}, fmt.Errorf("mock: invalid session snapshot: %w", err)
	}
	return snapshot, nil
}

func (r *Runtime) CreateSession(ctx context.Context, in agent.CreateSession) (agent.Session, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.Session{}, err
	}
	workspace := strings.TrimSpace(in.Workspace)
	if workspace == "" {
		return agent.Session{}, errors.New("mock: workspace is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.identities.next(sessionIdentity)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "Untitled session"
	}
	now := r.now()
	session := agent.Session{
		ID: id, Title: title, Status: agent.SessionIdle,
		Provider: defaultProvider, Model: defaultModel,
		Workspace: availableWorkspace(workspace), CreatedAt: now, UpdatedAt: now, Revision: 1,
	}
	r.sessions[session.ID] = &sessionState{meta: session}
	return session, nil
}

func (r *Runtime) UpdateSession(ctx context.Context, in agent.UpdateSession) (agent.Session, error) {
	if err := in.Validate(); err != nil {
		return agent.Session{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return agent.Session{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.sessions[in.SessionID]
	if !ok {
		return agent.Session{}, fmt.Errorf("%w: %s", agent.ErrSessionNotFound, in.SessionID)
	}
	if in.ExpectedRevision != state.meta.Revision {
		return agent.Session{}, fmt.Errorf("%w: session %s is at revision %d", agent.ErrRevisionConflict, in.SessionID, state.meta.Revision)
	}
	candidate := state.meta
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			return agent.Session{}, errors.New("mock: session title is empty")
		}
		candidate.Title = title
	}
	if in.Workspace != nil {
		candidate.Workspace = availableWorkspace(strings.TrimSpace(*in.Workspace))
	}
	if in.Model != nil {
		candidate.Provider = in.Model.Provider
		candidate.Model = in.Model.Model
	}
	if in.Favorite != nil {
		candidate.Favorite = *in.Favorite
	}
	candidate.UpdatedAt = r.now()
	if err := state.commitMeta(candidate); err != nil {
		return agent.Session{}, err
	}
	return state.meta, nil
}

func availableWorkspace(path string) workspace.Workspace {
	return workspace.Workspace{Path: path, ProjectRoot: path, Availability: workspace.Available}
}

func (r *Runtime) ForkSession(ctx context.Context, in agent.ForkSession) (agent.Session, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.Session{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	source, ok := r.sessions[in.SessionID]
	if !ok {
		return agent.Session{}, fmt.Errorf("%w: %s", agent.ErrSessionNotFound, in.SessionID)
	}
	boundary, err := r.resolveForkBoundary(source, in.FromRunID)
	if err != nil {
		return agent.Session{}, err
	}
	id := r.identities.next(sessionIdentity)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = source.meta.Title + " (fork)"
	}
	now := r.now()
	meta := agent.Session{ID: id, Title: title, Status: agent.SessionIdle, Provider: source.meta.Provider, Model: source.meta.Model, Workspace: source.meta.Workspace, CreatedAt: now, UpdatedAt: now, Revision: 1}
	state := &sessionState{meta: meta}
	if boundary.plan != nil {
		state.plan, err = commitInitialPlan(boundary.plan.Content())
		if err != nil {
			return agent.Session{}, fmt.Errorf("mock: fork plan: %w", err)
		}
	}
	r.sessions[id] = state
	return meta, nil
}

func (r *Runtime) RollbackSession(ctx context.Context, in agent.RollbackSession) (agent.RollbackResult, error) {
	if err := in.Validate(); err != nil {
		return agent.RollbackResult{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return agent.RollbackResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.sessions[in.SessionID]
	if state == nil {
		return agent.RollbackResult{}, fmt.Errorf("%w: %s", agent.ErrSessionNotFound, in.SessionID)
	}
	if state.active != "" {
		return agent.RollbackResult{}, fmt.Errorf("%w: %s", agent.ErrSessionBusy, in.SessionID)
	}
	if in.Scope == agent.RestoreFiles {
		return agent.RollbackResult{Session: state.meta}, nil
	}
	keep := -1
	if in.ToRunID != "" {
		keep = slices.Index(state.runs, in.ToRunID)
		if keep < 0 {
			return agent.RollbackResult{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, in.ToRunID)
		}
	}
	droppedIDs := slices.Clone(state.runs[keep+1:])
	droppedSet := make(map[string]struct{}, len(droppedIDs))
	result := agent.RollbackResult{Dropped: make([]agent.DroppedRun, 0, len(droppedIDs))}
	planAtRun := maps.Clone(state.planAtRun)
	for _, runID := range droppedIDs {
		droppedSet[runID] = struct{}{}
		dropped := agent.DroppedRun{RunID: runID}
		for _, item := range state.items {
			if item.runID == runID && item.block.Kind == agent.BlockUser && strings.TrimSpace(item.block.Text) != "" {
				dropped.Input = append(dropped.Input, agent.InputContent{Kind: agent.InputText, Text: item.block.Text})
				break
			}
		}
		result.Dropped = append(result.Dropped, dropped)
		delete(planAtRun, runID)
	}
	runs := slices.Clone(state.runs[:keep+1])
	items := slices.DeleteFunc(slices.Clone(state.items), func(item durableItem) bool {
		_, dropped := droppedSet[item.runID]
		return dropped
	})
	content, err := agent.NewPlanContent(nil)
	if err != nil {
		return agent.RollbackResult{}, fmt.Errorf("mock: empty rollback Plan: %w", err)
	}
	if in.ToRunID != "" {
		if boundary := state.planAtRun[in.ToRunID]; boundary != nil {
			content = boundary.Content()
		}
	}
	plan, err := commitNextPlan(state.plan, content)
	if err != nil {
		return agent.RollbackResult{}, fmt.Errorf("mock: rollback Plan: %w", err)
	}
	meta := state.meta
	meta.UpdatedAt = r.now()
	meta, err = nextSessionMeta(state.meta, meta)
	if err != nil {
		return agent.RollbackResult{}, err
	}
	result.Session = meta
	if err := result.Validate(); err != nil {
		return agent.RollbackResult{}, fmt.Errorf("mock: %w", err)
	}
	for _, runID := range droppedIDs {
		delete(r.runs, runID)
	}
	state.runs = runs
	state.items = items
	state.planAtRun = planAtRun
	state.plan = plan
	state.meta = meta
	return result, nil
}

type forkBoundary struct {
	plan *agent.Plan
}

// resolveForkBoundary mirrors the runtime's durable boundary rule: an explicit
// target must be terminal, while an implicit fork stops at the newest terminal
// root and excludes every running or waiting tail.
func (r *Runtime) resolveForkBoundary(source *sessionState, fromRunID string) (forkBoundary, error) {
	boundaryIndex := -1
	if fromRunID != "" {
		boundaryIndex = slices.Index(source.runs, fromRunID)
		if boundaryIndex < 0 || r.runs[fromRunID] == nil || r.runs[fromRunID].status != agent.RunStatusFinished {
			return forkBoundary{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, fromRunID)
		}
	} else {
		for i, runID := range slices.Backward(source.runs) {
			if run := r.runs[runID]; run != nil && run.status == agent.RunStatusFinished {
				boundaryIndex = i
				break
			}
		}
	}
	if boundaryIndex < 0 {
		return forkBoundary{}, nil
	}

	boundaryRunID := source.runs[boundaryIndex]
	return forkBoundary{plan: cloneCommittedPlan(source.planAtRun[boundaryRunID])}, nil
}

func (r *Runtime) DeleteSession(ctx context.Context, in agent.DeleteSession) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.sessions[in.SessionID]
	if !ok {
		return fmt.Errorf("%w: %s", agent.ErrSessionNotFound, in.SessionID)
	}
	if state.active != "" {
		return fmt.Errorf("%w: %s", agent.ErrSessionBusy, in.SessionID)
	}
	for _, runID := range state.runs {
		delete(r.runs, runID)
	}
	delete(r.sessions, in.SessionID)
	return nil
}

func (r *Runtime) seedHistory() {
	state := r.sessions["ses_demo_1"]
	if state == nil {
		return
	}
	run := &runState{
		id: "run_demo_history", sessionID: state.meta.ID, provider: "mock", model: "balanced",
		lineage: agent.RootRunLineage(),
		limits:  agent.UnlimitedRunLimits(), status: agent.RunStatusFinished, segments: make(map[string]*segmentState),
		outcome: agent.Outcome{Status: agent.OutcomeCompleted},
		usage:   agent.Usage{InputTokens: 820, OutputTokens: 94, CacheReadTokens: 512, Duration: 3 * time.Second},
	}
	r.runs[run.id] = run
	r.runOrder = append(r.runOrder, run.id)
	state.runs = append(state.runs, run.id)
	state.items = append(state.items,
		durableItem{runID: run.id, block: agent.Block{ID: "demo_prompt", RunID: run.id, Status: agent.BlockStatusCompleted, Kind: agent.BlockUser, Text: "Why is the cache expiry test flaky?"}},
		durableItem{runID: run.id, block: agent.Block{ID: "demo_answer", RunID: run.id, Status: agent.BlockStatusCompleted, Kind: agent.BlockAssistant, Text: "The fixed sleep races the janitor. Wait for its sweep signal instead."}},
	)
	state.planAtRun = map[string]*agent.Plan{run.id: nil}
}

func cloneCommittedPlan(plan *agent.Plan) *agent.Plan {
	if plan == nil {
		return nil
	}
	cloned := plan.Clone()
	return &cloned
}

func commitInitialPlan(content agent.PlanContent) (*agent.Plan, error) {
	return commitNextPlan(nil, content)
}

func commitNextPlan(previous *agent.Plan, content agent.PlanContent) (*agent.Plan, error) {
	committed, err := agent.CommitNextPlan(previous, content)
	if err != nil {
		return nil, err
	}
	return &committed, nil
}
