package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ErrCheckpointUnavailable reports that a file rollback can't restore the working
// tree — the checkpoint store is disabled or the target
// run has no snapshot.
var (
	ErrCheckpointUnavailable = errors.New("sessions: checkpoint unavailable")
	// ErrCheckpointRestoreIncomplete marks a restore that may already have
	// changed part of the working tree. The durable mutation intent must remain
	// pending so boot recovery can re-drive the operation.
	ErrCheckpointRestoreIncomplete = errors.New("sessions: checkpoint restore may be incomplete")
)

const mutationCleanupTimeout = 5 * time.Second

// RestoreScope is the closed family of resources one rollback can rewind.
type RestoreScope string

const (
	RestoreHistory RestoreScope = "history"
	RestoreFiles   RestoreScope = "files"
	RestoreBoth    RestoreScope = "both"
)

// Valid reports whether r names one supported rollback resource set.
func (r RestoreScope) Valid() bool {
	return r == RestoreHistory || r == RestoreFiles || r == RestoreBoth
}

// RestoresFiles reports whether r includes the working tree.
func (r RestoreScope) RestoresFiles() bool {
	return r == RestoreFiles || r == RestoreBoth
}

// RestoresHistory reports whether r includes durable Session history.
func (r RestoreScope) RestoresHistory() bool {
	return r == RestoreHistory || r == RestoreBoth
}

// RollbackSpec is the rollback intent: which Run to keep to and what the
// rollback rewinds. Every file restore is recoverable; RestoreBoth coordinates
// the working tree and durable history through the operation log described in
// §8.5.
type RollbackSpec struct {
	SessionID string
	ToRunID   string
	Scope     RestoreScope
}

func (r RollbackSpec) validate() error {
	if !r.Scope.Valid() {
		return fmt.Errorf("sessions: unknown restore scope %q", r.Scope)
	}
	if r.Scope.RestoresFiles() && r.ToRunID == "" {
		return errors.New("sessions: file restore requires target Run")
	}
	return nil
}

type DroppedRun struct {
	Run       run.Run
	UserInput []transcript.ContentBlock
}

type RollbackResult struct {
	Session View
	Dropped []DroppedRun
}

// Rollback executes a session rollback as one guarded operation: it claims
// the single-writer mutation slot (rejecting a rollback under an in-flight run
// as [ErrSessionBusy]) and, for a file restore, the working-tree mutation slot
// too, then resolves the boundary under those guards, stops detached processes
// that could outlive the discarded state, restores files before durable Session
// state, and applies the history truncation. It returns the resolved Session
// view with the mutation result so callers do not re-read a newer revision.
//
// The guards live with the use case: a file restore's `git reset --hard`
// writes a working tree a sibling session sharing the cwd would race, and that
// sibling's tool writes never take the checkpoint lock, so the mutation must see
// any in-flight run on the tree, not just this session's.
func (c *Coordinator) Rollback(ctx context.Context, spec RollbackSpec) (RollbackResult, error) {
	if err := spec.validate(); err != nil {
		return RollbackResult{}, err
	}
	restoreFiles := spec.Scope.RestoresFiles()
	restoreHistory := spec.Scope.RestoresHistory()
	currentSession, err := c.Get(ctx, spec.SessionID)
	if err != nil {
		return RollbackResult{}, err
	}
	result := RollbackResult{}

	sessionMutation, err := c.ClaimSessionMutation(spec.SessionID)
	if err != nil {
		return result, err
	}
	defer sessionMutation.Release()

	var cwd string
	if restoreFiles {
		cwd = currentSession.Workspace().Path()
		workingTreeMutation, claimed := c.ClaimWorkingTreeMutation(cwd)
		if !claimed {
			return result, fmt.Errorf("%w: working tree %q has a run admission in flight", ErrSessionBusy, cwd)
		}
		defer workingTreeMutation.Release()
	}

	resolvedBoundary, err := c.resolveRollbackBoundary(ctx, spec.SessionID, spec.ToRunID)
	if err != nil {
		return result, err
	}
	if restoreHistory {
		result.Dropped = resolvedBoundary.droppedRuns
	}
	if restoreFiles {
		if err := c.quiesceRollbackWorkspace(spec.SessionID, cwd); err != nil {
			return result, err
		}
	}
	// Every file restore is logged before Git touches the working tree. A reset
	// updates multiple paths and can fail after changing only some of them, so
	// even files-only rollback needs boot recovery. RestoreHistory distinguishes
	// that operation from the cross-resource files+history variant.
	mutationRecorded, err := c.recordRollbackMutation(ctx, spec, cwd)
	if err != nil {
		return result, err
	}

	// Errors before reset begins leave the tree unchanged, so their intent can be
	// cleared. ErrCheckpointRestoreIncomplete is different: reset may have
	// changed only part of the tree, and its intent must survive for recovery.
	if restoreRollbackFilesErr := c.restoreRollbackFiles(ctx, spec, cwd, mutationRecorded); restoreRollbackFilesErr != nil {
		if restoreFiles && errors.Is(restoreRollbackFilesErr, ErrCheckpointRestoreIncomplete) {
			c.transientState.ForgetWorkspace(cwd)
		}
		return result, restoreRollbackFilesErr
	}
	if restoreFiles {
		if err := c.retireRestoredWorkspace(spec.SessionID, cwd); err != nil {
			return result, err
		}
	}

	// The tree is restored now; a durable failure here leaves the intent logged so
	// boot recovery completes the truncation (the tree + history would otherwise
	// disagree).
	if restoreHistory && len(resolvedBoundary.timeline.Dropped) > 0 {
		if applyRollbackErr := c.applyRollback(ctx, spec.SessionID, resolvedBoundary.timeline); applyRollbackErr != nil {
			return result, applyRollbackErr
		}
	}

	if mutationRecorded {
		if completeMutationDetachedErr := c.completeMutationDetached(ctx, spec.SessionID); completeMutationDetachedErr != nil {
			return result, completeMutationDetachedErr
		}
	}
	result.Session, err = c.view(currentSession, ActivityIdle)
	if err != nil {
		return result, err
	}
	return result, nil
}

type resolvedRollbackBoundary struct {
	timeline    transcript.Boundary
	droppedRuns []DroppedRun
}

func (c *Coordinator) resolveRollbackBoundary(
	ctx context.Context,
	sessionID string,
	toRunID string,
) (resolvedRollbackBoundary, error) {
	items, err := c.transcript.List(ctx, sessionID)
	if err != nil {
		return resolvedRollbackBoundary{}, err
	}
	runs, err := listSessionRuns(ctx, c.runs, sessionID)
	if err != nil {
		return resolvedRollbackBoundary{}, err
	}
	boundary, err := transcript.TimelineFromRuns(runs).BoundaryAt(toRunID, true)
	if err != nil {
		return resolvedRollbackBoundary{}, err
	}
	return resolvedRollbackBoundary{
		timeline:    boundary,
		droppedRuns: projectDroppedRuns(boundary, runs, transcript.OpeningUserMessagesByRun(items)),
	}, nil
}

func (c *Coordinator) quiesceRollbackWorkspace(sessionID, cwd string) error {
	// A Session can own shells below its isolated copy, while sibling Sessions
	// can own shells below the real working tree being reset. Retire both
	// ownership scopes before Git changes any path.
	if err := c.transientState.QuiesceSession(sessionID); err != nil {
		return fmt.Errorf("sessions: quiesce process-local Session state before file rollback: %w", err)
	}
	if err := c.transientState.QuiesceWorkspace(cwd); err != nil {
		return fmt.Errorf("sessions: quiesce working tree before file rollback: %w", err)
	}
	return nil
}

func (c *Coordinator) recordRollbackMutation(ctx context.Context, spec RollbackSpec, cwd string) (bool, error) {
	if !spec.Scope.RestoresFiles() || c.mutations == nil {
		return false, nil
	}
	err := c.mutations.Record(ctx, WorkspaceMutation{
		SessionID: spec.SessionID, CWD: cwd, ToRunID: spec.ToRunID,
		RestoreHistory: spec.Scope.RestoresHistory(),
	})
	return err == nil, err
}

func (c *Coordinator) retireRestoredWorkspace(sessionID, cwd string) error {
	// The restored tree invalidates every file-read stamp below this shared
	// workspace and every isolated copy derived from its pre-rollback history.
	c.transientState.ForgetWorkspace(cwd)
	if c.sandbox == nil {
		return nil
	}
	if err := c.sandbox.Discard(sessionID); err != nil {
		return fmt.Errorf("sessions: discard sandbox copy after file rollback: %w", err)
	}
	return nil
}

func (c *Coordinator) restoreRollbackFiles(
	ctx context.Context,
	spec RollbackSpec,
	cwd string,
	mutationRecorded bool,
) error {
	if !spec.Scope.RestoresFiles() {
		return nil
	}
	err := c.restore(ctx, spec.SessionID, cwd, spec.ToRunID)
	if err == nil || !mutationRecorded || errors.Is(err, ErrCheckpointRestoreIncomplete) {
		return err
	}
	cleanupErr := c.completeMutationDetached(ctx, spec.SessionID)
	if cleanupErr == nil {
		return err
	}
	return errors.Join(err, fmt.Errorf("sessions: clear failed rollback intent: %w", cleanupErr))
}

func projectDroppedRuns(boundary transcript.Boundary, runs []run.Run, inputs map[string][]transcript.ContentBlock) []DroppedRun {
	byID := make(map[string]run.Run, len(runs))
	for _, run := range runs {
		byID[run.ID()] = run
	}
	out := make([]DroppedRun, 0, len(boundary.Dropped))
	for _, node := range boundary.Dropped {
		out = append(out, DroppedRun{Run: byID[node.ID], UserInput: inputs[node.ID]})
	}
	return out
}

// RecoverWorkspaceMutations re-drives every file rollback a crash left
// unfinished (§8.5): for each logged intent it re-restores the working tree
// (reentrant), conditionally re-applies the durable truncation (idempotent — an
// already-committed cut recomputes an empty boundary), then clears the intent.
// It runs at boot before the server serves, so no run contends for the session
// and the admission guards the live path needs are unnecessary. A failed
// recovery aborts startup (returned loud) rather than serving a session whose
// tree and history disagree.
func (c *Coordinator) RecoverWorkspaceMutations(ctx context.Context) error {
	if c.mutations == nil {
		return nil
	}
	pending, err := c.mutations.ListPending(ctx)
	if err != nil {
		return err
	}
	for _, m := range pending {
		if err := c.recoverRollback(ctx, m); err != nil {
			return fmt.Errorf("recover rollback for session %q: %w", m.SessionID, err)
		}
	}
	return nil
}

func (c *Coordinator) recoverRollback(ctx context.Context, m WorkspaceMutation) error {
	var boundary transcript.Boundary
	if m.RestoreHistory {
		runs, err := listSessionRuns(ctx, c.runs, m.SessionID)
		if err != nil {
			return err
		}
		boundary, err = transcript.TimelineFromRuns(runs).BoundaryAt(m.ToRunID, true)
		if err != nil {
			return err
		}
	}
	if err := c.quiesceRollbackWorkspace(m.SessionID, m.CWD); err != nil {
		return err
	}
	if err := c.restore(ctx, m.SessionID, m.CWD, m.ToRunID); err != nil {
		if errors.Is(err, ErrCheckpointRestoreIncomplete) {
			c.transientState.ForgetWorkspace(m.CWD)
		}
		return err
	}
	if err := c.retireRestoredWorkspace(m.SessionID, m.CWD); err != nil {
		return err
	}
	if m.RestoreHistory && len(boundary.Dropped) > 0 {
		if err := c.applyRollback(ctx, m.SessionID, boundary); err != nil {
			return err
		}
	}
	return c.completeMutation(ctx, m.SessionID)
}

func (c *Coordinator) completeMutation(ctx context.Context, sessionID string) error {
	if c.mutations == nil {
		return nil
	}
	return c.mutations.Complete(ctx, sessionID)
}

func (c *Coordinator) completeMutationDetached(ctx context.Context, sessionID string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationCleanupTimeout)
	defer cancel()
	return c.completeMutation(cleanupCtx, sessionID)
}

// restore drives the checkpoint store, mapping a nil store (file checkpoints
// disabled) onto [ErrCheckpointUnavailable] so a build without checkpoints
// rejects file restore rather than nil-panicking.
func (c *Coordinator) restore(ctx context.Context, sessionID, cwd, runID string) error {
	if c.checkpoints == nil {
		return ErrCheckpointUnavailable
	}
	return c.checkpoints.Restore(ctx, sessionID, cwd, runID)
}
