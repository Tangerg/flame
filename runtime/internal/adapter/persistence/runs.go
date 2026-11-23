package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	sqlite3 "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"
)

func parseRunState(raw string) (rundomain.State, error) {
	state := rundomain.State(raw)
	if !state.Valid() {
		return "", fmt.Errorf("sqlite: unknown Run state %q", raw)
	}
	return state, nil
}

type RunStore struct{ db *database }

func newRunStore(db *database) *RunStore {
	return &RunStore{db: db}
}

// Two constraints can reject this INSERT with opposite meanings: the primary key
// says the id is taken, the partial index says the Session already owns another
// root tree. Besides Restore this is the capabilities column's only writer, which
// keeps it frozen by construction.
func (r *RunStore) Admit(ctx context.Context, draft rundomain.Draft) error {
	admitted, err := rundomain.Admit(draft)
	if err != nil {
		return fmt.Errorf("sqlite: admit run %q: %w", draft.RunID, err)
	}
	lineage := admitted.Lineage()
	capabilities, err := encodeRunCapabilities(admitted.Capabilities())
	if err != nil {
		return fmt.Errorf("sqlite: admit run %q: %w", draft.RunID, err)
	}
	now := admitted.CreatedAt().UnixNano()
	return r.db.write(ctx, func(ctx context.Context) error {
		if err := r.requireWorkspaceAdmission(ctx, draft.SessionID); err != nil {
			return err
		}
		if lineage.IsChild() {
			if err := r.validateChildPlacement(ctx, admitChild, draft.RunID, draft.SessionID, draft.ParentRunID, draft.RootRunID); err != nil {
				return err
			}
		}
		_, err = r.db.connection(ctx).ExecContext(ctx, `INSERT INTO runs(
			   run_id, session_id, spawned_by_item_id, parent_run_id, root_run_id,
			   state, active_segment_id, provider, model, reasoning_effort, goal_incarnation_id,
			   capabilities, message_mark, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, admitted.ID(), admitted.SessionID(), lineage.SpawnedByItemID(), lineage.ParentRunID(), lineage.RootRunID(), string(rundomain.Running), admitted.ActiveSegmentID(), admitted.ModelSelection().Provider(), admitted.ModelSelection().Model(), admitted.ModelSelection().ReasoningEffort(), admitted.GoalIncarnationID().String(), capabilities, nil, now, now)
		switch {
		case isPrimaryKeyViolation(err):
			return fmt.Errorf("%w: run %q already exists", rundomain.ErrIdentityConflict, draft.RunID)
		case isUniqueViolation(err):
			return rundomain.ErrSessionBusy
		case err != nil:
			return fmt.Errorf("sqlite: admit run %q: %w", draft.RunID, err)
		}
		return nil
	})
}

func (r *RunStore) requireWorkspaceAdmission(ctx context.Context, sessionID identity.SessionID) error {
	return r.db.read(ctx, func(ctx context.Context) error {
		connection := r.db.connection(ctx)
		if pending, err := sessionResourceRetirementPending(ctx, connection, sessionID); err != nil || pending {
			if err != nil {
				return err
			}
			return rundomain.ErrSessionBusy
		}
		var cwd string
		if err := connection.QueryRowContext(ctx, `SELECT workspace_path FROM sessions WHERE id = ?`, sessionID).Scan(&cwd); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("sqlite: read Run workspace: %w", err)
		}
		rows, err := connection.QueryContext(ctx, `SELECT session_id, cwd FROM pending_workspace_mutations`)
		if err != nil {
			return fmt.Errorf("sqlite: inspect pending workspace mutations: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var pendingSessionID identity.SessionID
			var pendingCWD string
			if err := rows.Scan(&pendingSessionID, &pendingCWD); err != nil {
				return fmt.Errorf("sqlite: read pending workspace mutation: %w", err)
			}
			conflicts, err := workspaceMutationsOverlap(pendingSessionID, pendingCWD, sessionID, cwd)
			if err != nil {
				return fmt.Errorf("sqlite: compare pending workspace mutation: %w", err)
			}
			if conflicts {
				return rundomain.ErrSessionBusy
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("sqlite: iterate pending workspace mutations: %w", err)
		}
		return nil
	})
}

type childPlacement string

const (
	admitChild   childPlacement = "admit"
	restoreChild childPlacement = "restore"
)

func (p childPlacement) requiresOpenTree() bool { return p == admitChild }

func (r *RunStore) validateChildPlacement(ctx context.Context, operation childPlacement, runID identity.RunID, sessionID identity.SessionID, parentRunID identity.RunID, rootRunID identity.RunID) error {
	return r.db.read(ctx, func(ctx context.Context) error {
		var (
			parentSession identity.SessionID
			parentRoot    identity.RunID
			parentState   string
			rootSession   identity.SessionID
			rootParent    string
			rootState     string
		)
		err := r.db.connection(ctx).QueryRowContext(ctx, `SELECT parent.session_id, parent.root_run_id, parent.state,
		        root.session_id, root.parent_run_id, root.state
		   FROM runs AS parent
		   JOIN runs AS root ON root.run_id = ?
		  WHERE parent.run_id = ?`, rootRunID, parentRunID).Scan(&parentSession, &parentRoot, &parentState, &rootSession, &rootParent, &rootState)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("sqlite: %s child run %q: parent %q or root %q does not exist", operation, runID, parentRunID, rootRunID)
		}
		if err != nil {
			return fmt.Errorf("sqlite: %s child run %q: validate tree: %w", operation, runID, err)
		}
		parentRunState, err := parseRunState(parentState)
		if err != nil {
			return fmt.Errorf("sqlite: %s child run %q: parent: %w", operation, runID, err)
		}
		rootRunState, err := parseRunState(rootState)
		if err != nil {
			return fmt.Errorf("sqlite: %s child run %q: root: %w", operation, runID, err)
		}
		parentTreeRoot := parentRoot
		if parentTreeRoot.IsZero() {
			parentTreeRoot = parentRunID
		}
		switch {
		case parentSession != sessionID:
			return fmt.Errorf("sqlite: %s child run %q: parent %q belongs to session %q, want %q", operation, runID, parentRunID, parentSession, sessionID)
		case rootSession != sessionID:
			return fmt.Errorf("sqlite: %s child run %q: root %q belongs to session %q, want %q", operation, runID, rootRunID, rootSession, sessionID)
		case rootParent != "":
			return fmt.Errorf("sqlite: %s child run %q: root %q is itself a child", operation, runID, rootRunID)
		case parentTreeRoot != rootRunID:
			return fmt.Errorf("sqlite: %s child run %q: parent %q belongs to root %q, want %q", operation, runID, parentRunID, parentTreeRoot, rootRunID)
		case operation.requiresOpenTree() && parentRunState == rundomain.Finished:
			return fmt.Errorf("sqlite: %s child run %q: parent %q is terminal", operation, runID, parentRunID)
		case operation.requiresOpenTree() && rootRunState == rundomain.Finished:
			return fmt.Errorf("sqlite: %s child run %q: root %q is terminal", operation, runID, rootRunID)
		}
		return nil
	})
}

func (r *RunStore) Suspend(ctx context.Context, value rundomain.Run, segmentID identity.SegmentID, commitID runs.CommitID) error {
	if err := validateRunCoordinates("suspend Run", value.SessionID(), value.ID(), segmentID); err != nil {
		return err
	}
	var marker *runCommitMarker
	if !commitID.IsZero() {
		var err error
		marker, err = newRunCommitMarker(value.SessionID(), value.ID(), segmentID, commitID)
		if err != nil {
			return err
		}
	}
	metrics, err := runMetricsRow(value.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: suspend run %q: %w", value.ID(), err)
	}
	commitSegmentID, commitIDValue := marker.databaseValues()
	return r.db.write(ctx, func(ctx context.Context) error {
		if err := r.requireAdvance(ctx, "suspend", value); err != nil {
			return err
		}
		res, err := r.db.connection(ctx).ExecContext(ctx, `UPDATE runs SET state = ?, active_segment_id = '', commit_segment_id = ?, commit_id = ?,
		        steps = ?, active_duration_ns = ?, usage = ?, context_tokens = ?, updated_at = ?
		 WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ?`, string(value.State()), commitSegmentID, commitIDValue, metrics.steps, metrics.durationNs, metrics.usage, value.ContextTokens(), value.UpdatedAt().UTC().UnixNano(), value.SessionID(), value.ID(), string(rundomain.Running), segmentID)
		if err != nil {
			return fmt.Errorf("sqlite: suspend run %q: %w", value.ID(), err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: suspend run %q: read affected rows: %w", value.ID(), err)
		}
		if n == 0 {
			return fmt.Errorf("sqlite: suspend run %q: Segment %q is no longer the running Segment", value.ID(), segmentID)
		}
		return nil
	})
}

func (r *RunStore) Resume(ctx context.Context, replacement rundomain.Replacement) error {
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sqlite: resume run: %w", err)
	}
	expected, next := replacement.Expected(), replacement.State()
	metrics, err := runMetricsRow(next.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: resume run %q: %w", next.ID(), err)
	}
	return r.db.write(ctx, func(ctx context.Context) error {
		if err := r.requireWorkspaceAdmission(ctx, expected.SessionID()); err != nil {
			return err
		}
		if err := r.requireExpected(ctx, "resume", expected); err != nil {
			return err
		}
		res, err := r.db.connection(ctx).ExecContext(ctx, `UPDATE runs SET state = ?, active_segment_id = ?, commit_segment_id = '', commit_id = '',
		        steps = ?, active_duration_ns = ?, usage = ?, context_tokens = ?, updated_at = ?
		 WHERE session_id = ? AND run_id = ? AND state = ? AND updated_at = ?`, string(next.State()), next.ActiveSegmentID(), metrics.steps, metrics.durationNs, metrics.usage, next.ContextTokens(), next.UpdatedAt().UTC().UnixNano(), expected.SessionID(), expected.ID(), string(expected.State()), expected.UpdatedAt().UTC().UnixNano())
		if err != nil {
			return fmt.Errorf("sqlite: resume run %q: %w", next.ID(), err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: resume run %q: read affected rows: %w", next.ID(), err)
		}
		if n == 0 {
			return fmt.Errorf("sqlite: resume run: Run %q is not the parked Run the continuation resumes", next.ID())
		}
		return nil
	})
}

func (r *RunStore) RequireActiveSegment(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, segmentID identity.SegmentID) error {
	return r.db.read(ctx, func(ctx context.Context) error {
		if err := validateRunCoordinates("require active Run Segment", sessionID, runID, segmentID); err != nil {
			return err
		}
		var state, activeSegmentID string
		err := r.db.connection(ctx).QueryRowContext(ctx, `SELECT state, active_segment_id
		   FROM runs
		  WHERE session_id = ? AND run_id = ?`, sessionID, runID).Scan(&state, &activeSegmentID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("sqlite: Run %q was not found in session %q", runID, sessionID)
		}
		if err != nil {
			return fmt.Errorf("sqlite: read active Segment for Run %q: %w", runID, err)
		}
		storedState, err := parseRunState(state)
		if err != nil {
			return fmt.Errorf("sqlite: read active Segment for Run %q: %w", runID, err)
		}
		if storedState != rundomain.Running || activeSegmentID != segmentID.String() {
			return fmt.Errorf("sqlite: Run %q is %s in Segment %q, want running Segment %q", runID, state, activeSegmentID, segmentID)
		}
		return nil
	})
}

func (r *RunStore) UpdateProgress(ctx context.Context, progressed rundomain.Run) error {
	if err := validateRunCoordinates("update Run progress", progressed.SessionID(), progressed.ID(), progressed.ActiveSegmentID()); err != nil {
		return err
	}
	encoded, err := runMetricsRow(progressed.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: update Run progress for %q: %w", progressed.ID(), err)
	}
	return r.db.write(ctx, func(ctx context.Context) error {
		if err := r.requireAdvance(ctx, "update progress", progressed); err != nil {
			return err
		}
		result, err := r.db.connection(ctx).ExecContext(ctx, `UPDATE runs SET steps = ?, active_duration_ns = ?, usage = ?, context_tokens = ?, updated_at = ?
		 WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ?`, encoded.steps, encoded.durationNs, encoded.usage, progressed.ContextTokens(), progressed.UpdatedAt().UTC().UnixNano(), progressed.SessionID(), progressed.ID(), string(rundomain.Running), progressed.ActiveSegmentID())
		if err != nil {
			return fmt.Errorf("sqlite: update Run progress for %q: %w", progressed.ID(), err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect Run progress update for %q: %w", progressed.ID(), err)
		}
		if changed != 1 {
			return fmt.Errorf("sqlite: update Run progress for %q lost its active-segment fence", progressed.ID())
		}
		return nil
	})
}

const terminalizeOperation = "terminalize"

func (r *RunStore) Terminalize(ctx context.Context, replacement rundomain.Replacement, winningCancellationKey string) error {
	return r.replace(ctx, terminalizeOperation, replacement, winningCancellationKey)
}

func (r *RunStore) TerminalizeEvent(ctx context.Context, value rundomain.Run, segmentID identity.SegmentID, commitID runs.CommitID, winningCancellationKey string) error {
	marker, err := newRunCommitMarker(value.SessionID(), value.ID(), segmentID, commitID)
	if err != nil {
		return err
	}
	return r.db.write(ctx, func(ctx context.Context) error {
		if err := r.requireAdvance(ctx, terminalizeOperation, value); err != nil {
			return err
		}
		return r.finish(ctx, terminalizeOperation, value, marker, winningCancellationKey, `state = ? AND active_segment_id = ?`, string(rundomain.Running), segmentID)
	})
}

// updated_at deliberately stays untouched: compaction does not change when the Run
// happened.
func (r *RunStore) RebaseMessageMark(ctx context.Context, change rundomain.Replacement) error {
	if err := change.Validate(); err != nil {
		return fmt.Errorf("sqlite: rebase Run message watermark: %w", err)
	}
	expected := change.Expected()
	count, known := change.State().MessageMark().Count()
	if !known {
		return errors.New("sqlite: rebase Run message watermark: the rebased watermark is unknown")
	}
	return r.db.write(ctx, func(ctx context.Context) error {
		if err := r.requireExpected(ctx, "rebase message watermark", expected); err != nil {
			return err
		}
		result, err := r.db.connection(ctx).ExecContext(ctx, `UPDATE runs SET message_mark = ?
		 WHERE session_id = ? AND run_id = ? AND state = ? AND message_mark IS ?`, count, expected.SessionID(), expected.ID(), string(rundomain.Finished), messageMarkValue(expected.MessageMark()))
		if err != nil {
			return fmt.Errorf("sqlite: rebase Run %q message watermark: %w", expected.ID(), err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect Run %q message watermark rebase: %w", expected.ID(), err)
		}
		if changed != 1 {
			return fmt.Errorf("sqlite: rebase Run %q message watermark lost its expected-value fence", expected.ID())
		}
		return nil
	})
}

func (r *RunStore) RecoverLost(ctx context.Context, replacement rundomain.Replacement) error {
	return r.replace(ctx, "recover lost", replacement, "")
}

func (r *RunStore) replace(ctx context.Context, op string, replacement rundomain.Replacement, winningCancellationKey string) error {
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sqlite: %s run: %w", op, err)
	}
	expected := replacement.Expected()
	return r.db.write(ctx, func(ctx context.Context) error {
		if err := r.requireExpected(ctx, op, expected); err != nil {
			return err
		}
		return r.finish(ctx, op, replacement.State(), nil, winningCancellationKey, `state = ? AND active_segment_id = ? AND updated_at = ?`, string(expected.State()), expected.ActiveSegmentID(), expected.UpdatedAt().UTC().UnixNano())
	})
}

func (r *RunStore) requireExpected(ctx context.Context, operation string, expected rundomain.Run) error {
	current, found, err := r.readRun(ctx, operation, expected.ID(), scanRunForRecovery)
	if err != nil {
		return err
	}
	if !found || !current.Equal(expected) {
		return fmt.Errorf("sqlite: %s run %q: Run changed after the application prepared its replacement", operation, expected.ID())
	}
	return nil
}

func (r *RunStore) requireAdvance(ctx context.Context, operation string, state rundomain.Run) error {
	current, found, err := r.readRun(ctx, operation, state.ID(), scanRunForRecovery)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("sqlite: %s run %q: Run was not found", operation, state.ID())
	}
	if err := state.Metrics().ValidateAdvanceFrom(current.Metrics()); err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", operation, state.ID(), err)
	}
	if state.UpdatedAt().Before(current.UpdatedAt()) {
		return fmt.Errorf("sqlite: %s run %q: snapshot precedes the committed update", operation, state.ID())
	}
	return nil
}

// This CAS is the only place a Run reaches terminal, so the root's Plan boundary
// is stamped here; that is how no terminal root lacks a recorded boundary.
func (r *RunStore) finish(ctx context.Context, op string, value rundomain.Run, marker *runCommitMarker, winningCancellationKey string, fence string, fenceArgs ...any) error {
	if winningCancellationKey != "" && !value.Canceled() {
		return errors.New("sqlite: a winning cancellation key requires a canceled Run")
	}
	metrics, err := runMetricsRow(value.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, value.ID(), err)
	}
	encodedEffects, err := encodeUnresolvedEffects(value.UnresolvedEffects())
	if err != nil {
		return err
	}
	ending, _ := value.Ending()
	outcome, encodedEnding, err := encodeRunEnding(ending)
	if err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, value.ID(), err)
	}
	commitSegmentID, commitID := marker.databaseValues()
	query := `UPDATE runs SET
		   state = ?, active_segment_id = '', commit_segment_id = ?, commit_id = ?,
		   outcome = ?, ending = ?, steps = ?, active_duration_ns = ?,
		   usage = ?, context_tokens = ?, unresolved_effects = ?, message_mark = ?, finished_at = ?, updated_at = ?
		 WHERE session_id = ? AND run_id = ? AND ` + fence
	args := []any{string(value.State()), commitSegmentID, commitID, outcome, encodedEnding, metrics.steps, metrics.durationNs, metrics.usage, value.ContextTokens(), encodedEffects, messageMarkValue(value.MessageMark()), value.FinishedAt().UTC().UnixNano(), value.UpdatedAt().UTC().UnixNano(), value.SessionID(), value.ID()}
	args = append(args, fenceArgs...)
	return r.db.write(ctx, func(ctx context.Context) error {
		res, err := r.db.connection(ctx).ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("sqlite: %s run: %w", op, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: %s run: read affected rows: %w", op, err)
		}
		if n == 0 {
			return fmt.Errorf("sqlite: %s run %q: Run changed after the application prepared its terminal state", op, value.ID())
		}
		if err := newIdempotencyStore(r.db).settleRunCancellationRefusals(ctx, value, winningCancellationKey); err != nil {
			return err
		}
		if value.Lineage().IsChild() {
			return nil
		}
		return newPlanStore(r.db).CaptureBoundary(ctx, value.SessionID(), value.ID())
	})
}

// An upsert would silently re-parent an existing Run, so an import that would do
// so is refused.
func (r *RunStore) Restore(ctx context.Context, value rundomain.Run) error {
	return r.db.write(ctx, func(ctx context.Context) error {
		if !value.State().IsTerminal() {
			return fmt.Errorf("sqlite: restore run %q: state is %s, want terminal", value.ID(), value.State())
		}
		lineage := value.Lineage()
		if lineage.IsChild() {
			if err := r.validateChildPlacement(ctx, restoreChild, value.ID(), value.SessionID(), lineage.ParentRunID(), lineage.RootRunID()); err != nil {
				return err
			}
		}
		metrics, err := runMetricsRow(value.Metrics())
		if err != nil {
			return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
		}
		encodedEffects, err := encodeUnresolvedEffects(value.UnresolvedEffects())
		if err != nil {
			return err
		}
		ending, _ := value.Ending()
		outcome, encodedEnding, err := encodeRunEnding(ending)
		if err != nil {
			return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
		}
		capabilities, err := encodeRunCapabilities(value.Capabilities())
		if err != nil {
			return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
		}
		selection := value.ModelSelection()
		_, err = r.db.connection(ctx).ExecContext(ctx, `INSERT INTO runs(
		   run_id, session_id, spawned_by_item_id, parent_run_id, root_run_id,
		   state, outcome, provider, model, reasoning_effort, goal_incarnation_id,
		   ending, steps, active_duration_ns, usage, context_tokens, unresolved_effects,
		   capabilities, message_mark, created_at, finished_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID(), value.SessionID(), lineage.SpawnedByItemID(), lineage.ParentRunID(), lineage.RootRunID(), string(value.State()), outcome, selection.Provider(), selection.Model(), selection.ReasoningEffort(), value.GoalIncarnationID().String(), encodedEnding, metrics.steps, metrics.durationNs, metrics.usage, value.ContextTokens(), encodedEffects, capabilities, messageMarkValue(value.MessageMark()), value.CreatedAt().UTC().UnixNano(), value.FinishedAt().UTC().UnixNano(), value.UpdatedAt().UTC().UnixNano())
		if isPrimaryKeyViolation(err) {
			return fmt.Errorf("%w: run %q already exists", rundomain.ErrIdentityConflict, value.ID())
		}
		if err != nil {
			return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
		}
		return nil
	})
}

func (r *RunStore) Delete(ctx context.Context, sessionID identity.SessionID, runID identity.RunID) error {
	return r.db.write(ctx, func(ctx context.Context) error {
		if err := validateResource("delete Run", sessionID); err != nil {
			return err
		}
		if err := validateResource("delete Run", runID); err != nil {
			return err
		}
		if _, err := r.db.connection(ctx).ExecContext(ctx, `DELETE FROM runs WHERE run_id = ? AND session_id = ?`, runID, sessionID); err != nil {
			return fmt.Errorf("sqlite: delete run: %w", err)
		}
		return nil
	})
}

func (r *RunStore) DeleteForSession(ctx context.Context, sessionID identity.SessionID) error {
	return r.db.write(ctx, func(ctx context.Context) error {
		if err := validateResource("delete Session Runs", sessionID); err != nil {
			return err
		}
		_, err := r.db.connection(ctx).ExecContext(ctx, `DELETE FROM runs WHERE session_id = ?`, sessionID)
		if err != nil {
			return fmt.Errorf("sqlite: delete runs for session: %w", err)
		}
		return nil
	})
}

func isUniqueViolation(err error) bool {
	se, ok := errors.AsType[*sqlite3.Error](err)
	return ok && se.Code() == sqlite3lib.SQLITE_CONSTRAINT_UNIQUE
}

func isPrimaryKeyViolation(err error) bool {
	se, ok := errors.AsType[*sqlite3.Error](err)
	return ok && se.Code() == sqlite3lib.SQLITE_CONSTRAINT_PRIMARYKEY
}
