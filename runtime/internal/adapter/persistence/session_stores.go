package persistence

import (
	"context"
	"fmt"

	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

// SessionStores is the SQLite-backed adapter for the session lifecycle's
// snapshot and atomic write-set ports. Each operation applies the complete
// application-decided mutation inside one storage transaction.
type SessionStores struct {
	commandAcceptances  *commandAcceptanceStore
	resourceRetirements *ResourceRetirementStore
	sessions            *SessionStore
	transcript          *TranscriptStore
	interrupts          *InterruptStore
	runs                *RunStore
	executorCheckpoints *ExecutorCheckpointStore
	history             *runsapp.ConversationHistory
	plan                planProjection
	approvalRules       sessionStateCleaner
	planModes           sessionStateCleaner
	toolResults         *ToolResultStore
	childRunStarts      childRunStartReservationCleaner
	goals               goalStore
	tx                  Transactor
	readSnapshot        Transactor
}

// SessionStoresConfig is the durable collaborator set for SessionStores.
type SessionStoresConfig struct {
	CommandAcceptances  *commandAcceptanceStore
	ResourceRetirements *ResourceRetirementStore
	Sessions            *SessionStore
	Transcript          *TranscriptStore
	Interrupts          *InterruptStore
	Runs                *RunStore
	ExecutorCheckpoints *ExecutorCheckpointStore
	History             *runsapp.ConversationHistory
	Plan                planProjection
	ApprovalRules       sessionStateCleaner
	ApprovalModes       sessionStateCleaner
	ToolResults         *ToolResultStore
	ChildRunStarts      childRunStartReservationCleaner
	Goals               goalStore
	Tx                  Transactor
	ReadSnapshot        Transactor
}

// Transactor settles the supplied read or write boundary as one transaction.
type Transactor func(context.Context, func(context.Context) error) error

// planProjection is the session-scoped Plan this write-set has to move with
// the session through its whole lifecycle: read for an archive, replaced by a
// restore, seeded into a fork, republished at a rollback boundary, dropped with
// a delete. Save applies an application-decided aggregate transition with CAS;
// this adapter assigns neither revision nor update time.
type planProjection interface {
	State(ctx context.Context, sessionID string) (plan.Current, error)
	ListBoundaries(ctx context.Context, sessionID string) ([]sessions.RunPlanBoundary, error)
	RestoreBoundaries(ctx context.Context, boundaries []sessions.RunPlanBoundary) error
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

// goalStore also captures a terminal Goal-owned root Run inside the write-set
// that ended the Run.
type goalStore interface {
	Get(ctx context.Context, sessionID string) (goal.Current, error)
	Clear(ctx context.Context, sessionID string) error
	CaptureGoalRun(ctx context.Context, value rundomain.Run) error
}

// NewSessionStores returns the SQLite adapter for session snapshots and
// write-sets. Every store is required: a delete, fork, rollback or restore is
// one complete mutation, and a missing collaborator would silently narrow it
// rather than fail.
func NewSessionStores(cfg SessionStoresConfig) (*SessionStores, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "session store", value: cfg.Sessions},
		{name: "command acceptance store", value: cfg.CommandAcceptances},
		{name: "session resource retirements", value: cfg.ResourceRetirements},
		{name: "transcript store", value: cfg.Transcript},
		{name: "interrupt store", value: cfg.Interrupts},
		{name: "Run store", value: cfg.Runs},
		{name: "executor checkpoint store", value: cfg.ExecutorCheckpoints},
		{name: "conversation history", value: cfg.History},
		{name: "Plan projection", value: cfg.Plan},
		{name: "approval rule store", value: cfg.ApprovalRules},
		{name: "permission mode store", value: cfg.ApprovalModes},
		{name: "Tool result store", value: cfg.ToolResults},
		{name: "child Run start reservation store", value: cfg.ChildRunStarts},
		{name: "Goal store", value: cfg.Goals},
		{name: "transactor", value: cfg.Tx},
		{name: "read snapshot", value: cfg.ReadSnapshot},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("persistence: session %s is required", required.name)
		}
	}
	return &SessionStores{
		commandAcceptances:  cfg.CommandAcceptances,
		resourceRetirements: cfg.ResourceRetirements,
		sessions:            cfg.Sessions,
		transcript:          cfg.Transcript,
		interrupts:          cfg.Interrupts,
		runs:                cfg.Runs,
		executorCheckpoints: cfg.ExecutorCheckpoints,
		history:             cfg.History,
		plan:                cfg.Plan,
		approvalRules:       cfg.ApprovalRules,
		planModes:           cfg.ApprovalModes,
		toolResults:         cfg.ToolResults,
		childRunStarts:      cfg.ChildRunStarts,
		goals:               cfg.Goals,
		tx:                  cfg.Tx,
		readSnapshot:        cfg.ReadSnapshot,
	}, nil
}

var (
	_ sessions.SnapshotReader         = (*SessionStores)(nil)
	_ sessions.MaterialSnapshotReader = (*SessionStores)(nil)
	_ sessions.WriteSets              = (*SessionStores)(nil)
)

func (s *SessionStores) ReadMaterialSnapshot(ctx context.Context, sessionID string) (sessions.MaterialSnapshot, error) {
	var snapshot sessions.MaterialSnapshot
	err := s.readSnapshot(ctx, func(ctx context.Context) error {
		ses, err := s.sessions.Get(ctx, sessionID)
		if err != nil {
			return err
		}
		items, err := s.transcript.List(ctx, sessionID)
		if err != nil {
			return err
		}
		steerInputs, err := s.commandAcceptances.SessionSteerInputs(ctx, sessionID)
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
			SteerInputs: steerInputs,
			Session:     ses, Items: items, Runs: runs, Interrupts: interrupts, Plan: state,
			Goal: currentGoal,
		}
		return nil
	})
	return snapshot, err
}

func (s *SessionStores) ReadSnapshot(ctx context.Context, sessionID string) (sessions.Snapshot, error) {
	var snapshot sessions.Snapshot
	err := s.readSnapshot(ctx, func(ctx context.Context) error {
		history, err := s.readHistory(ctx, sessionID)
		if err != nil {
			return err
		}
		planHead, err := s.plan.State(ctx, sessionID)
		if err != nil {
			return err
		}
		snapshot.HistorySnapshot = history
		if p, ok := planHead.State(); ok {
			doc := p.Document()
			snapshot.Plan, snapshot.PlanSource = doc.Steps, doc.Source
		}
		return nil
	})
	return snapshot, err
}

func (s *SessionStores) ReadForkMaterial(ctx context.Context, sessionID string) (sessions.ForkMaterial, error) {
	var material sessions.ForkMaterial
	err := s.readSnapshot(ctx, func(ctx context.Context) error {
		history, err := s.readHistory(ctx, sessionID)
		if err != nil {
			return err
		}
		boundaries, err := s.plan.ListBoundaries(ctx, sessionID)
		if err != nil {
			return err
		}
		material = sessions.ForkMaterial{HistorySnapshot: history, PlanBoundaries: boundaries}
		return nil
	})
	return material, err
}

func (s *SessionStores) readHistory(ctx context.Context, sessionID string) (sessions.HistorySnapshot, error) {
	ses, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return sessions.HistorySnapshot{}, err
	}
	messages, err := s.history.Read(ctx, sessionID)
	if err != nil {
		return sessions.HistorySnapshot{}, err
	}
	items, err := s.transcript.ListStored(ctx, sessionID)
	if err != nil {
		return sessions.HistorySnapshot{}, err
	}
	runs, err := s.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return sessions.HistorySnapshot{}, err
	}
	toolResults, err := s.toolResults.List(ctx, sessionID)
	if err != nil {
		return sessions.HistorySnapshot{}, err
	}
	return sessions.HistorySnapshot{
		Session: ses, Messages: messages, Items: items, Runs: runs, ToolResults: toolResults,
	}, nil
}

// ApplyFork persists the Domain-derived child Session and the complete visible
// history/Plan boundary in one transaction.
func (s *SessionStores) ApplyFork(ctx context.Context, fork sessions.ForkPlan) (session.Session, error) {
	snapshot := fork.History()
	child := snapshot.Session
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
		if err := s.savePlanReplacement(ctx, child.ID(), planReplacement); err != nil {
			return err
		}
		if err := s.restoreRuns(ctx, snapshot.Runs); err != nil {
			return err
		}
		if err := s.plan.RestoreBoundaries(ctx, fork.PlanBoundaries()); err != nil {
			return err
		}
		return s.restoreTranscript(ctx, snapshot.Items, snapshot.ToolResults)
	})
	if err != nil {
		return session.Session{}, err
	}
	return child, nil
}

// ApplyRollback persists one resolved rollback plan atomically.
func (s *SessionStores) ApplyRollback(ctx context.Context, rollback sessions.RollbackPlan) error {
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
		if err := s.interrupts.Delete(ctx, runID); err != nil {
			return err
		}
	}
	return nil
}

// ApplyRestore replaces every durable projection for a restored session in one
// transaction.
func (s *SessionStores) ApplyRestore(ctx context.Context, restore sessions.RestorePlan) error {
	sessionReplacement := restore.SessionReplacement()
	snapshot := restore.History()
	planReplacement := restore.PlanReplacement()
	return s.tx(ctx, func(ctx context.Context) error {
		restoredSession := sessionReplacement.State()
		sessionID := restoredSession.ID()
		if err := s.requireAvailableResources(ctx, sessionID); err != nil {
			return err
		}
		if err := s.saveSessionReplacement(ctx, sessionReplacement); err != nil {
			return err
		}
		// An absent archived Plan carries no authority to erase the destination's current version.
		if err := s.clearSessionOwnedStateExceptPlan(ctx, sessionID); err != nil {
			return err
		}
		if err := s.restorePlanAndHistory(ctx, sessionID, snapshot.Messages, planReplacement); err != nil {
			return err
		}
		if err := s.restoreRuns(ctx, snapshot.Runs); err != nil {
			return err
		}
		if err := s.restoreTranscript(ctx, snapshot.Items, snapshot.ToolResults); err != nil {
			return err
		}
		if retirement, replaced := restore.Retirement(); replaced {
			return s.resourceRetirements.record(ctx, retirement)
		}
		return nil
	})
}

func (s *SessionStores) saveSessionReplacement(
	ctx context.Context,
	replacement session.Replacement,
) error {
	if _, replacing := replacement.Expected(); !replacing {
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

// ApplyDelete removes all durable state for the addressed session.
func (s *SessionStores) ApplyDelete(ctx context.Context, deletion sessions.DeletePlan) error {
	sessionID := deletion.SessionID()
	return s.tx(ctx, func(ctx context.Context) error {
		if err := s.requireAvailableResources(ctx, sessionID); err != nil {
			return err
		}
		if err := s.deleteSession(ctx, sessionID); err != nil {
			return err
		}
		return s.resourceRetirements.record(ctx, deletion.Retirement())
	})
}

func (s *SessionStores) requireAvailableResources(ctx context.Context, sessionID string) error {
	pending, err := s.resourceRetirements.pending(ctx, sessionID)
	if err != nil {
		return err
	}
	if pending {
		return rundomain.ErrSessionBusy
	}
	return nil
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
	if err := s.planModes.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.goals.Clear(ctx, sessionID); err != nil {
		return err
	}
	if err := s.deleteChildRunStarts(ctx, sessionID); err != nil {
		return err
	}
	return nil
}

// ApplyTerminal persists the terminal record for an abandoned parked run and
// clears its executor checkpoint atomically.
func (s *SessionStores) ApplyTerminal(ctx context.Context, terminal sessions.TerminalPlan, winningCancellationKey string) error {
	root, _ := terminal.RootRun()
	items := terminal.Items()
	messages := terminal.Messages()
	runs := terminal.Runs()
	return s.tx(ctx, func(ctx context.Context) error {
		for _, item := range items {
			if err := s.transcript.ReplaceItem(ctx, item); err != nil {
				return err
			}
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
		); err != nil {
			return err
		}
		if err := s.terminalizeParkedRuns(ctx, runs, root.ID(), winningCancellationKey); err != nil {
			return err
		}
		return s.recordGoalTerminalRun(ctx, root)
	})
}

func (s *SessionStores) restoreTranscript(ctx context.Context, items []transcript.Item, bodies []transcript.ToolResultBlob) error {
	byID := make(map[transcript.ToolResultID]*transcript.ToolResultBlob, len(bodies))
	for index := range bodies {
		byID[bodies[index].ID] = &bodies[index]
	}
	for _, item := range items {
		call, _ := item.ToolCall()
		var body *transcript.ToolResultBlob
		if ref := call.Invocation.Offload; ref != nil {
			body = byID[ref.ID]
		}
		if err := s.transcript.writeItem(ctx, item, nil, true, body); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionStores) clearParkedRunState(
	ctx context.Context,
	root rundomain.Run,
	checkpointRootID string,
) error {
	if checkpointRootID != "" {
		if err := s.executorCheckpoints.DeleteCheckpoints(ctx, root.SessionID(), []string{checkpointRootID}); err != nil {
			return err
		}
	}
	// Delete the interrupt before the terminal write: while it exists the Run is
	// parked on it, and a Run cannot be both finished and waiting.
	if err := s.interrupts.Delete(ctx, root.ID()); err != nil {
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

func (s *SessionStores) terminalizeParkedRuns(ctx context.Context, runs []rundomain.Replacement, rootRunID, winningCancellationKey string) error {
	for _, replacement := range runs {
		run := replacement.State()
		outcome, terminal := run.Outcome()
		if !terminal {
			return fmt.Errorf("persistence: terminal Run %q outcome is required", run.ID())
		}
		switch outcome {
		case rundomain.OutcomeCanceled:
			key := ""
			if run.ID() == rootRunID {
				key = winningCancellationKey
			}
			if err := s.runs.Terminalize(ctx, replacement, key); err != nil {
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

func (s *SessionStores) recordGoalTerminalRun(ctx context.Context, root rundomain.Run) error {
	if root.GoalIncarnationID() == "" {
		return nil
	}
	if err := s.goals.CaptureGoalRun(ctx, root); err != nil {
		return fmt.Errorf("persistence: record Goal Run for Run %q: %w", root.ID(), err)
	}
	return nil
}

func (s *SessionStores) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := s.interrupts.List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, interrupt := range pending {
		if err := s.interrupts.Delete(ctx, interrupt.RootRunID); err != nil {
			return err
		}
	}
	return nil
}
