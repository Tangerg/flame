package persistence

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Tangerg/scope/core/chat"

	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	sqlitestore "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// SessionStores is the SQLite-backed adapter for the session lifecycle's
// snapshot and atomic write-set ports. Each operation applies the complete
// application-decided mutation inside one storage transaction.
type SessionStores struct {
	sessions            *sqlitestore.SessionStore
	transcript          *sqlitestore.TranscriptStore
	interrupts          *InterruptStore
	runs                *sqlitestore.RunStore
	executorCheckpoints *ExecutorCheckpointStore
	history             *runsapp.ConversationHistory
	plan                planProjection
	approvalRules       sessionStateCleaner
	permissionModes     sessionStateCleaner
	toolResults         *sqlitestore.ToolResultStore
	childRunStarts      childRunStartReservationCleaner
	goals               goalStore
	tx                  Transactor
}

// SessionStoresConfig is the durable collaborator set for SessionStores.
type SessionStoresConfig struct {
	Sessions            *sqlitestore.SessionStore
	Transcript          *sqlitestore.TranscriptStore
	Interrupts          *InterruptStore
	Runs                *sqlitestore.RunStore
	ExecutorCheckpoints *ExecutorCheckpointStore
	History             *runsapp.ConversationHistory
	Plan                planProjection
	ApprovalRules       sessionStateCleaner
	PermissionModes     sessionStateCleaner
	ToolResults         *sqlitestore.ToolResultStore
	ChildRunStarts      childRunStartReservationCleaner
	Goals               goalStore
	Tx                  Transactor
}

// Transactor runs a complete write-set inside one durable transaction.
type Transactor func(context.Context, func(context.Context) error) error

// planProjection is the session-scoped Plan this write-set has to move with
// the session through its whole lifecycle: read for an archive, replaced by a
// restore, seeded into a fork, republished at a rollback boundary, dropped with
// a delete. Save applies an application-decided aggregate transition with CAS;
// this adapter assigns neither revision nor update time.
type planProjection interface {
	List(ctx context.Context, sessionID string) ([]plan.Step, error)
	State(ctx context.Context, sessionID string) (plan.Current, error)
	Save(ctx context.Context, sessionID string, replacement plan.Replacement) error
	DeleteSession(ctx context.Context, sessionID string) error
}

// sessionStateCleaner is the lifecycle port shared by projections whose rows
// cannot outlive or leak across a complete Session replacement.
type sessionStateCleaner interface {
	DeleteSession(ctx context.Context, sessionID string) error
}

// childRunStartReservationCleaner is the Session-lifecycle slice of the
// adapter-owned callback ledger. The Application supplies the exact Session
// write-set; this persistence adapter removes its invisible technical rows
// alongside the public state they cannot outlive.
type childRunStartReservationCleaner interface {
	DeleteSession(ctx context.Context, sessionID string) error
}

type goalStore interface {
	Get(ctx context.Context, sessionID string) (goal.Current, error)
	Clear(ctx context.Context, sessionID string) error
	RecordRun(ctx context.Context, record goal.RunRecord) error
}

// NewSessionStores returns the SQLite adapter for session snapshots and
// write-sets. Every store is required: a delete, fork, rollback or restore is
// one complete mutation, and a missing collaborator would silently narrow it
// rather than fail.
func NewSessionStores(cfg SessionStoresConfig) (*SessionStores, error) {
	for _, dependency := range []struct {
		name  string
		value any
	}{
		{name: "session store", value: cfg.Sessions},
		{name: "transcript store", value: cfg.Transcript},
		{name: "interrupt store", value: cfg.Interrupts},
		{name: "Run store", value: cfg.Runs},
		{name: "executor checkpoint store", value: cfg.ExecutorCheckpoints},
		{name: "conversation history", value: cfg.History},
		{name: "Plan projection", value: cfg.Plan},
		{name: "approval rule store", value: cfg.ApprovalRules},
		{name: "permission mode store", value: cfg.PermissionModes},
		{name: "Tool result store", value: cfg.ToolResults},
		{name: "child Run start reservation store", value: cfg.ChildRunStarts},
		{name: "Goal store", value: cfg.Goals},
		{name: "transactor", value: cfg.Tx},
	} {
		if missingSessionStore(dependency.value) {
			return nil, fmt.Errorf("persistence: session %s is required", dependency.name)
		}
	}
	return &SessionStores{
		sessions:            cfg.Sessions,
		transcript:          cfg.Transcript,
		interrupts:          cfg.Interrupts,
		runs:                cfg.Runs,
		executorCheckpoints: cfg.ExecutorCheckpoints,
		history:             cfg.History,
		plan:                cfg.Plan,
		approvalRules:       cfg.ApprovalRules,
		permissionModes:     cfg.PermissionModes,
		toolResults:         cfg.ToolResults,
		childRunStarts:      cfg.ChildRunStarts,
		goals:               cfg.Goals,
		tx:                  cfg.Tx,
	}, nil
}

func missingSessionStore(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var (
	_ sessions.SnapshotReader         = (*SessionStores)(nil)
	_ sessions.MaterialSnapshotReader = (*SessionStores)(nil)
	_ sessions.WriteSets              = (*SessionStores)(nil)
)

func (s *SessionStores) ReadMaterialSnapshot(ctx context.Context, sessionID string) (sessions.MaterialSnapshot, error) {
	var snapshot sessions.MaterialSnapshot
	err := s.tx(ctx, func(ctx context.Context) error {
		ses, err := s.sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		items, err := s.transcript.List(ctx, sessionID)
		if err != nil {
			return err
		}
		runs, err := s.runs.ListRuns(ctx, sessionID)
		if err != nil {
			return err
		}
		interrupts, err := s.interrupts.List(ctx, sessionID)
		if err != nil {
			return err
		}
		state, err := s.plan.State(ctx, sessionID)
		if err != nil {
			return err
		}
		current, err := s.goals.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		var currentGoal *goal.Goal
		if stored, found := current.Goal(); found {
			stored = stored.Clone()
			currentGoal = &stored
		}
		snapshot = sessions.MaterialSnapshot{
			Session: ses, Items: items, Runs: runs, Interrupts: interrupts, Plan: state,
			Goal: currentGoal,
		}
		return nil
	})
	return snapshot, err
}

func (s *SessionStores) ReadSnapshot(ctx context.Context, sessionID string) (sessions.Snapshot, error) {
	var snapshot sessions.Snapshot
	err := s.tx(ctx, func(ctx context.Context) error {
		ses, err := s.sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		messages, err := s.history.Read(ctx, sessionID)
		if err != nil {
			return err
		}
		items, err := s.transcript.List(ctx, sessionID)
		if err != nil {
			return err
		}
		runs, err := s.runs.ListRuns(ctx, sessionID)
		if err != nil {
			return err
		}
		toolResults, err := s.toolResults.List(ctx, sessionID)
		if err != nil {
			return err
		}
		steps, err := s.plan.List(ctx, sessionID)
		if err != nil {
			return err
		}
		snapshot = sessions.Snapshot{
			Session: ses, Messages: messages, Items: items, Runs: runs,
			ToolResults: toolResults, Plan: steps,
		}
		return nil
	})
	return snapshot, err
}

// ApplyFork persists the Domain-derived child Session and the complete visible
// history/Plan boundary in one transaction.
func (s *SessionStores) ApplyFork(ctx context.Context, fork sessions.ForkPlan) (session.Session, error) {
	if err := fork.Validate(); err != nil {
		return session.Session{}, fmt.Errorf("persistence: invalid fork plan: %w", err)
	}
	snapshot := fork.Snapshot()
	child := fork.Child()
	parentID := fork.ParentID()
	planReplacement := fork.PlanReplacement()
	err := s.tx(ctx, func(ctx context.Context) error {
		if _, err := s.sessions.Get(ctx, parentID); err != nil {
			return err
		}
		if err := s.sessions.Insert(ctx, child); err != nil {
			return err
		}
		if err := s.history.Seed(ctx, child.ID(), snapshot.Messages); err != nil {
			return err
		}
		// A branch that copies the conversation copies the plan it was following. Only
		// a non-empty list is written: a fresh child with no row already reads as a
		// session with no list, and writing one would publish an empty list as news.
		if err := s.savePlanReplacement(ctx, child.ID(), planReplacement); err != nil {
			return err
		}
		if err := s.restoreRuns(ctx, snapshot.Runs); err != nil {
			return err
		}
		if err := s.appendTranscriptItems(ctx, snapshot.Items); err != nil {
			return err
		}
		return s.restoreToolResults(ctx, snapshot.ToolResults)
	})
	if err != nil {
		return session.Session{}, err
	}
	return child, nil
}

// ApplyRollback persists one resolved rollback plan atomically.
func (s *SessionStores) ApplyRollback(ctx context.Context, rollback sessions.RollbackPlan) error {
	if err := rollback.Validate(); err != nil {
		return fmt.Errorf("persistence: invalid rollback plan: %w", err)
	}
	sessionID := rollback.SessionID()
	dropRunIDs := rollback.DropRunIDs()
	checkpointRootIDs := rollback.CheckpointRootIDs()
	return s.tx(ctx, func(ctx context.Context) error {
		if err := s.republishRollbackState(ctx, rollback); err != nil {
			return err
		}
		if err := s.deleteRolledBackRuns(ctx, sessionID, dropRunIDs); err != nil {
			return err
		}
		if len(checkpointRootIDs) > 0 {
			if err := s.executorCheckpoints.DeleteCheckpoints(ctx, sessionID, checkpointRootIDs); err != nil {
				return err
			}
		}
		if err := s.deleteChildRunStarts(ctx, sessionID); err != nil {
			return err
		}
		return nil
	})
}

func (s *SessionStores) republishRollbackState(ctx context.Context, rollback sessions.RollbackPlan) error {
	// The application has already decided whether this boundary was recorded and,
	// if so, computed its new aggregate revision. The adapter only applies it.
	sessionID := rollback.SessionID()
	if err := s.savePlanReplacement(ctx, sessionID, rollback.PlanReplacement()); err != nil {
		return err
	}
	if err := s.goals.Clear(ctx, sessionID); err != nil {
		return err
	}
	if mark, known := rollback.TruncationMark(); known {
		return s.history.Truncate(ctx, sessionID, mark)
	}
	return nil
}

func (s *SessionStores) deleteRolledBackRuns(ctx context.Context, sessionID string, runIDs []string) error {
	for _, runID := range runIDs {
		if err := s.transcript.DeleteRun(ctx, sessionID, runID); err != nil {
			return err
		}
		// A rolled-back Run ceases to exist and therefore releases the Session's
		// admission slot; no state remains to terminalize.
		if err := s.runs.Delete(ctx, sessionID, runID); err != nil {
			return err
		}
		if err := s.interrupts.Delete(ctx, sessionID, runID); err != nil {
			return err
		}
	}
	return nil
}

// ApplyRestore replaces every durable projection for a restored session in one
// transaction.
func (s *SessionStores) ApplyRestore(ctx context.Context, restore sessions.RestorePlan) error {
	if err := restore.Validate(); err != nil {
		return fmt.Errorf("persistence: invalid restore plan: %w", err)
	}
	sessionReplacement := restore.SessionReplacement()
	snapshot := restore.Snapshot()
	planReplacement := restore.PlanReplacement()
	return s.tx(ctx, func(ctx context.Context) error {
		restoredSession := sessionReplacement.State()
		sessionID := restoredSession.ID()
		if err := s.saveSessionReplacement(ctx, sessionReplacement); err != nil {
			return err
		}
		// Keep the live Plan row when a replacement was prepared: deleting it would
		// reset the session's revision space before the CAS update.
		if planReplacement == nil {
			if err := s.clearSessionOwnedState(ctx, sessionID); err != nil {
				return err
			}
		} else if err := s.clearSessionOwnedStateExceptPlan(ctx, sessionID); err != nil {
			return err
		}
		if err := s.restorePlanAndHistory(ctx, sessionID, snapshot.Messages, planReplacement); err != nil {
			return err
		}
		if err := s.restoreRuns(ctx, snapshot.Runs); err != nil {
			return err
		}
		if err := s.appendTranscriptItems(ctx, snapshot.Items); err != nil {
			return err
		}
		return s.restoreToolResults(ctx, snapshot.ToolResults)
	})
}

func (s *SessionStores) saveSessionReplacement(
	ctx context.Context,
	replacement session.Replacement,
) error {
	if replacement.ExpectedRevision() == 0 {
		return s.sessions.Insert(ctx, replacement.State())
	}
	return s.sessions.Save(ctx, replacement)
}

func (s *SessionStores) restorePlanAndHistory(
	ctx context.Context,
	sessionID string,
	messages []chat.Message,
	planReplacement *plan.Replacement,
) error {
	if err := s.savePlanReplacement(ctx, sessionID, planReplacement); err != nil {
		return err
	}
	return s.history.Seed(ctx, sessionID, messages)
}

func (s *SessionStores) savePlanReplacement(ctx context.Context, sessionID string, replacement *plan.Replacement) error {
	if replacement == nil {
		return nil
	}
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("persistence: invalid Plan replacement: %w", err)
	}
	return s.plan.Save(ctx, sessionID, *replacement)
}

func (s *SessionStores) restoreRuns(ctx context.Context, restored []rundomain.Run) error {
	for _, run := range restored {
		if err := s.runs.Restore(ctx, run); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionStores) restoreToolResults(ctx context.Context, blobs []toolresult.Blob) error {
	for _, blob := range blobs {
		if err := s.toolResults.Restore(ctx, blob); err != nil {
			return err
		}
	}
	return nil
}

// ApplyDelete removes all durable state for the addressed session.
func (s *SessionStores) ApplyDelete(ctx context.Context, deletion sessions.DeletePlan) error {
	if err := deletion.Validate(); err != nil {
		return fmt.Errorf("persistence: invalid delete plan: %w", err)
	}
	sessionID := deletion.SessionID()
	return s.tx(ctx, func(ctx context.Context) error {
		return s.deleteSession(ctx, sessionID)
	})
}

func (s *SessionStores) deleteSession(ctx context.Context, sessionID string) error {
	if err := s.clearSessionOwnedState(ctx, sessionID); err != nil {
		return err
	}
	return s.sessions.Delete(ctx, sessionID)
}

func (s *SessionStores) clearSessionOwnedState(ctx context.Context, sessionID string) error {
	if err := s.clearSessionOwnedStateExceptPlan(ctx, sessionID); err != nil {
		return err
	}
	return s.plan.DeleteSession(ctx, sessionID)
}

func (s *SessionStores) clearSessionOwnedStateExceptPlan(ctx context.Context, sessionID string) error {
	if err := s.transcript.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.history.Clear(ctx, sessionID); err != nil {
		return err
	}
	if err := s.deleteInterrupts(ctx, sessionID); err != nil {
		return err
	}
	if err := s.executorCheckpoints.DeleteSessionCheckpoints(ctx, sessionID); err != nil {
		return err
	}
	if err := s.runs.DeleteForSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.approvalRules.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.permissionModes.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.goals.Clear(ctx, sessionID); err != nil {
		return err
	}
	if err := s.toolResults.DropSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.deleteChildRunStarts(ctx, sessionID); err != nil {
		return err
	}
	return nil
}

// ApplyTerminal persists the terminal record for an abandoned parked run and
// clears its executor checkpoint atomically.
func (s *SessionStores) ApplyTerminal(ctx context.Context, terminal sessions.TerminalPlan) error {
	if err := terminal.Validate(); err != nil {
		return fmt.Errorf("persistence: invalid terminal plan: %w", err)
	}
	root, _ := terminal.RootRun()
	items := terminal.Items()
	messages := terminal.Messages()
	runs := terminal.Runs()
	goalRun := terminal.GoalRun()
	return s.tx(ctx, func(ctx context.Context) error {
		if err := s.appendTranscriptItems(ctx, items); err != nil {
			return err
		}
		if len(messages) != 0 {
			if err := s.history.Append(ctx, root.SessionID(), messages...); err != nil {
				return fmt.Errorf("persistence: append terminal conversation messages: %w", err)
			}
		}
		if err := s.clearParkedRunState(
			ctx,
			root,
			terminal.CheckpointRootID(),
			terminal.ConsumesClaimedResume(),
		); err != nil {
			return err
		}
		if err := s.terminalizeParkedRuns(ctx, runs); err != nil {
			return err
		}
		return s.recordGoalTerminalRun(ctx, root.ID(), goalRun)
	})
}

func (s *SessionStores) appendTranscriptItems(ctx context.Context, items []transcript.Item) error {
	for _, item := range items {
		if err := s.transcript.AppendItem(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionStores) clearParkedRunState(
	ctx context.Context,
	root rundomain.Run,
	checkpointRootID string,
	resumeClaimed bool,
) error {
	if checkpointRootID != "" {
		if err := s.executorCheckpoints.DeleteCheckpoints(ctx, root.SessionID(), []string{checkpointRootID}); err != nil {
			return err
		}
	}
	// Delete the interrupt before the terminal write: while it exists the Run is
	// parked on it, and a Run cannot be both finished and waiting.
	if resumeClaimed {
		if err := s.interrupts.DeleteResumeClaim(
			ctx,
			root.SessionID(),
			root.ID(),
			checkpointRootID,
		); err != nil {
			return err
		}
	} else if err := s.interrupts.Delete(ctx, root.SessionID(), root.ID()); err != nil {
		return err
	}
	return s.deleteChildRunStarts(ctx, root.SessionID())
}

func (s *SessionStores) deleteChildRunStarts(ctx context.Context, sessionID string) error {
	if err := s.childRunStarts.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("persistence: delete child Run start reservations for Session %q: %w", sessionID, err)
	}
	return nil
}

func (s *SessionStores) terminalizeParkedRuns(ctx context.Context, runs []rundomain.Replacement) error {
	for _, replacement := range runs {
		run := replacement.State()
		outcome, terminal := run.Outcome()
		if !terminal {
			return fmt.Errorf("persistence: terminal Run %q outcome is required", run.ID())
		}
		switch outcome {
		case rundomain.OutcomeCanceled:
			if err := s.runs.Terminalize(ctx, replacement); err != nil {
				return err
			}
		case rundomain.OutcomeLost:
			if err := s.runs.RecoverLost(ctx, replacement); err != nil {
				return err
			}
		default:
			return fmt.Errorf("persistence: unsupported parked terminal outcome %s", outcome)
		}
	}
	return nil
}

func (s *SessionStores) recordGoalTerminalRun(ctx context.Context, rootRunID string, record *goal.RunRecord) error {
	if record == nil {
		return nil
	}
	if err := s.goals.RecordRun(ctx, *record); err != nil {
		return fmt.Errorf("persistence: record Goal Run for Run %q: %w", rootRunID, err)
	}
	return nil
}

func (s *SessionStores) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := s.interrupts.List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, interrupt := range pending {
		if err := s.interrupts.Delete(ctx, sessionID, interrupt.RootRunID); err != nil {
			return err
		}
	}
	return nil
}
