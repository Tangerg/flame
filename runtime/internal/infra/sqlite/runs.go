package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	sqlite3 "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

// Coarse admission states stored in runs.state. The partial unique index
// idx_runs_session_active keys on non-terminal root rows, so a Session holds at
// most one non-terminal Run tree while any number of its descendant rows may be
// active. The fine [run.Outcome] is stored separately in runs.outcome.
type runState string

const (
	runStateRunning  runState = "running"
	runStateWaiting  runState = "waiting"
	runStateTerminal runState = "terminal"
)

func parseRunState(raw string) (runState, error) {
	state := runState(raw)
	switch state {
	case runStateRunning, runStateWaiting, runStateTerminal:
		return state, nil
	default:
		return "", fmt.Errorf("sqlite: unknown Run state %q", raw)
	}
}

func (r runState) databaseValue() string { return string(r) }

// RunStore is the SQLite-backed Run table: one row per root or child Run,
// holding its durable projection. Its immutable lineage columns identify the
// tree without reconstructing it from transcript Items. A partial unique index
// guarantees at most one non-terminal root per Session across restarts; child
// rows share that root's admission.
//
// One table, one owner: the accrued facts are written only by the lifecycle
// transition that makes them true, so "where is this Run" and "how did it end"
// cannot disagree. A Run's open interrupts are the one part kept elsewhere: the
// interrupts table owns them and reads compose them.
type RunStore struct {
	db *sql.DB
}

// NewRunStore binds the Run table to db. db must have been opened via [Open] so
// the current schema was installed.
func NewRunStore(db *sql.DB) *RunStore {
	return &RunStore{db: db}
}

// Admit records draft as the session's active (running) Run. It returns
// [rundomain.ErrSessionBusy] when the partial unique index rejects the INSERT —
// the session already has a non-terminal Run — or when an unfinished workspace
// rollback owns this Session or its working tree, and
// [rundomain.ErrIdentityConflict] when the Run ID is already taken, since the
// caller may supply one.
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
	// This is the capability set's only writer, here and in Restore. Suspend,
	// resume, and finish deliberately do not name the column: the value cannot change
	// after admission, and the way to guarantee that is to have nothing able to
	// change it.
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		pendingWorkspaceMutation, err := r.pendingWorkspaceMutation(ctx, draft.SessionID)
		if err != nil {
			return err
		}
		if pendingWorkspaceMutation {
			return rundomain.ErrSessionBusy
		}
		if lineage.IsChild() {
			if err := r.validateChildPlacement(
				ctx,
				"admit",
				draft.RunID,
				draft.SessionID,
				draft.ParentRunID,
				draft.RootRunID,
				true,
			); err != nil {
				return err
			}
		}
		_, err = conn(ctx, r.db).ExecContext(ctx,
			`INSERT INTO runs(
			   run_id, session_id, spawned_by_item_id, parent_run_id, root_run_id,
			   state, active_segment_id, provider, model, reasoning_effort, goal_incarnation_id,
			   capabilities, message_mark, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			admitted.ID(), admitted.SessionID(),
			lineage.SpawnedByItemID, lineage.ParentRunID, lineage.RootRunID,
			runStateRunning.databaseValue(), admitted.ActiveSegmentID(),
			admitted.ModelSelection().Provider(), admitted.ModelSelection().Model(), admitted.ModelSelection().ReasoningEffort(),
			admitted.GoalIncarnationID(),
			capabilities,
			nil, now, now)
		// Two constraints can reject this INSERT and they mean opposite things: the
		// primary key says the id is spoken for, the partial index says the Session
		// already owns another root tree.
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

// pendingWorkspaceMutation reports whether a recoverable destructive mutation
// still owns either this Session or the canonical working tree recorded on it.
// The check shares the admission transaction, so completing an intent and
// admitting the next Run have one serial order across Runtime processes.
func (r *RunStore) pendingWorkspaceMutation(ctx context.Context, sessionID string) (bool, error) {
	var pending bool
	if err := conn(ctx, r.db).QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1
			  FROM pending_workspace_mutations AS pending
			 WHERE pending.session_id = ?
			    OR pending.cwd = (
				SELECT workspace_path FROM sessions WHERE id = ?
			    )
		)`,
		sessionID,
		sessionID,
	).Scan(&pending); err != nil {
		return false, fmt.Errorf("sqlite: inspect pending workspace mutation: %w", err)
	}
	return pending, nil
}

// validateChildPlacement proves immutable Run-to-Run topology before inserting
// a child. The spawning Item is validated by the application write-set that
// owns Item creation and child admission/restore together.
func (r *RunStore) validateChildPlacement(
	ctx context.Context,
	operation string,
	runID string,
	sessionID string,
	parentRunID string,
	rootRunID string,
	requireOpen bool,
) error {
	var (
		parentSession string
		parentRoot    string
		parentState   string
		rootSession   string
		rootParent    string
		rootState     string
	)
	err := conn(ctx, r.db).QueryRowContext(ctx,
		`SELECT parent.session_id, parent.root_run_id, parent.state,
		        root.session_id, root.parent_run_id, root.state
		   FROM runs AS parent
		   JOIN runs AS root ON root.run_id = ?
		  WHERE parent.run_id = ?`,
		rootRunID,
		parentRunID,
	).Scan(
		&parentSession,
		&parentRoot,
		&parentState,
		&rootSession,
		&rootParent,
		&rootState,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf(
			"sqlite: %s child run %q: parent %q or root %q does not exist",
			operation,
			runID,
			parentRunID,
			rootRunID,
		)
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
	if parentTreeRoot == "" {
		parentTreeRoot = parentRunID
	}
	switch {
	case parentSession != sessionID:
		return fmt.Errorf(
			"sqlite: %s child run %q: parent %q belongs to session %q, want %q",
			operation,
			runID,
			parentRunID,
			parentSession,
			sessionID,
		)
	case rootSession != sessionID:
		return fmt.Errorf(
			"sqlite: %s child run %q: root %q belongs to session %q, want %q",
			operation,
			runID,
			rootRunID,
			rootSession,
			sessionID,
		)
	case rootParent != "":
		return fmt.Errorf(
			"sqlite: %s child run %q: root %q is itself a child",
			operation,
			runID,
			rootRunID,
		)
	case parentTreeRoot != rootRunID:
		return fmt.Errorf(
			"sqlite: %s child run %q: parent %q belongs to root %q, want %q",
			operation,
			runID,
			parentRunID,
			parentTreeRoot,
			rootRunID,
		)
	case requireOpen && parentRunState == runStateTerminal:
		return fmt.Errorf(
			"sqlite: %s child run %q: parent %q is terminal",
			operation,
			runID,
			parentRunID,
		)
	case requireOpen && rootRunState == runStateTerminal:
		return fmt.Errorf(
			"sqlite: %s child run %q: root %q is terminal",
			operation,
			runID,
			rootRunID,
		)
	}
	return nil
}

// Suspend writes the domain-decided park of the exact active Segment, recording
// what the Run had consumed up to the park. Every tree member is fenced by
// segmentID on a running row; the root additionally supplies commitID so the
// complete barrier can be reconciled after an ambiguous transaction result.
func (r *RunStore) Suspend(
	ctx context.Context,
	value rundomain.Run,
	segmentID string,
	commitID runtimeidentity.CommitID,
) error {
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
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		if err := r.requireTransition(ctx, "suspend", value); err != nil {
			return err
		}
		// The segment identity is cleared in the same statement that parks the Run:
		// a Run waiting on a person has no segment to attach to.
		res, err := conn(ctx, r.db).ExecContext(ctx,
			`UPDATE runs SET state = ?, active_segment_id = '', commit_segment_id = ?, commit_id = ?,
		        steps = ?, active_duration_ns = ?, usage = ?, context_tokens = ?, updated_at = ?
		 WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ?`,
			coarseState(value.State()).databaseValue(), commitSegmentID, commitIDValue,
			metrics.steps, metrics.durationNs, metrics.usage, value.ContextTokens(), value.UpdatedAt().UTC().UnixNano(),
			value.SessionID(), value.ID(), runStateRunning.databaseValue(), segmentID)
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

// Resume writes the exact parked Run's decided continuation. Unlike cleanup
// transitions it is strict: a row that is no longer the parked Run the
// replacement was derived from means the continuation opening does not own the
// durable Run and must roll back.
func (r *RunStore) Resume(ctx context.Context, replacement rundomain.Replacement) error {
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sqlite: resume run: %w", err)
	}
	expected, next := replacement.Expected(), replacement.State()
	metrics, err := runMetricsRow(next.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: resume run %q: %w", next.ID(), err)
	}
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		if err := r.requireExpected(ctx, "resume", expected); err != nil {
			return err
		}
		res, err := conn(ctx, r.db).ExecContext(ctx,
			`UPDATE runs SET state = ?, active_segment_id = ?, commit_segment_id = '', commit_id = '',
		        steps = ?, active_duration_ns = ?, usage = ?, context_tokens = ?, updated_at = ?
		 WHERE session_id = ? AND run_id = ? AND state = ? AND updated_at = ?`,
			coarseState(next.State()).databaseValue(), next.ActiveSegmentID(),
			metrics.steps, metrics.durationNs, metrics.usage, next.ContextTokens(), next.UpdatedAt().UTC().UnixNano(),
			expected.SessionID(), expected.ID(), coarseState(expected.State()).databaseValue(),
			expected.UpdatedAt().UTC().UnixNano())
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

// RequireActiveSegment proves that an event transaction still belongs to the
// exact running Segment that produced it. Callers execute this read through the
// transaction-bound connection before any projection write; a replacement,
// park, or terminal transition therefore rejects the complete stale write-set.
func (r *RunStore) RequireActiveSegment(ctx context.Context, sessionID, runID, segmentID string) error {
	if err := validateRunCoordinates("require active Run Segment", sessionID, runID, segmentID); err != nil {
		return err
	}
	var state, activeSegmentID string
	err := conn(ctx, r.db).QueryRowContext(ctx,
		`SELECT state, active_segment_id
		   FROM runs
		  WHERE session_id = ? AND run_id = ?`,
		sessionID,
		runID,
	).Scan(&state, &activeSegmentID)
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
	if storedState != runStateRunning || activeSegmentID != segmentID {
		return fmt.Errorf(
			"sqlite: Run %q is %s in Segment %q, want running Segment %q",
			runID,
			state,
			activeSegmentID,
			segmentID,
		)
	}
	return nil
}

// UpdateProgress writes the Run the domain advanced at one model-call boundary:
// its cumulative accounting, latest prompt footprint and update time. The write
// is fenced to the running Segment the Run names, so a stale continuation cannot
// overwrite a newer Run; it never moves lifecycle state.
func (r *RunStore) UpdateProgress(ctx context.Context, progressed rundomain.Run) error {
	if err := validateRunCoordinates("update Run progress", progressed.SessionID(), progressed.ID(), progressed.ActiveSegmentID()); err != nil {
		return err
	}
	encoded, err := runMetricsRow(progressed.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: update Run progress for %q: %w", progressed.ID(), err)
	}
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		if err := r.requireTransition(ctx, "update progress", progressed); err != nil {
			return err
		}
		result, err := conn(ctx, r.db).ExecContext(ctx,
			`UPDATE runs SET steps = ?, active_duration_ns = ?, usage = ?, context_tokens = ?, updated_at = ?
		 WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ?`,
			encoded.steps,
			encoded.durationNs,
			encoded.usage,
			progressed.ContextTokens(),
			progressed.UpdatedAt().UTC().UnixNano(),
			progressed.SessionID(),
			progressed.ID(),
			runStateRunning.databaseValue(),
			progressed.ActiveSegmentID(),
		)
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

// Terminalize ends the exact non-terminal Run snapshot that replacement names,
// recording the outcome the application reached and the result that explains it.
func (r *RunStore) Terminalize(ctx context.Context, replacement rundomain.Replacement) error {
	return r.replace(ctx, "terminalize", replacement)
}

// TerminalizeEvent ends one exact active Segment and stamps the immutable
// Application EventCommit write-set identity into the Run row. The stamp shares
// the caller's transaction with every projection in that EventCommit.
func (r *RunStore) TerminalizeEvent(
	ctx context.Context,
	value rundomain.Run,
	segmentID string,
	commitID runtimeidentity.CommitID,
) error {
	marker, err := newRunCommitMarker(value.SessionID(), value.ID(), segmentID, commitID)
	if err != nil {
		return err
	}
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		if err := r.requireTransition(ctx, "terminalize", value); err != nil {
			return err
		}
		return r.finish(ctx, "terminalize", value, marker,
			`state = ? AND active_segment_id = ?`, runStateRunning.databaseValue(), segmentID)
	})
}

// RebaseMessageMark writes the watermark of an Application-decided
// Run.WithMessageMark replacement, fenced on the terminal row it was derived
// from. Compaction does not change when the Run happened or any of its
// lifecycle facts, so updated_at deliberately remains untouched.
func (r *RunStore) RebaseMessageMark(ctx context.Context, change rundomain.Replacement) error {
	if err := change.Validate(); err != nil {
		return fmt.Errorf("sqlite: rebase Run message watermark: %w", err)
	}
	expected := change.Expected()
	count, known := change.State().MessageMark().Count()
	if !known {
		return errors.New("sqlite: rebase Run message watermark: the rebased watermark is unknown")
	}
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		if err := r.requireExpected(ctx, "rebase message watermark", expected); err != nil {
			return err
		}
		result, err := conn(ctx, r.db).ExecContext(ctx,
			`UPDATE runs SET message_mark = ?
		 WHERE session_id = ? AND run_id = ? AND state = ? AND message_mark IS ?`,
			count, expected.SessionID(), expected.ID(), runStateTerminal.databaseValue(), messageMarkValue(expected.MessageMark()),
		)
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

// RecoverLost ends the exact non-terminal Run snapshot whose executor state is
// no longer resumable. Unlike Terminalize, this recovery transition is legal
// from either Running or Waiting, because it describes a Run nobody is driving
// rather than one the executor finished.
func (r *RunStore) RecoverLost(ctx context.Context, replacement rundomain.Replacement) error {
	return r.replace(ctx, "recover lost", replacement)
}

// replace writes a Replacement's decided terminal state. Its legality was
// settled when the Replacement was derived; the write fences the exact
// aggregate it was derived from.
func (r *RunStore) replace(ctx context.Context, op string, replacement rundomain.Replacement) error {
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sqlite: %s run: %w", op, err)
	}
	expected := replacement.Expected()
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		if err := r.requireExpected(ctx, op, expected); err != nil {
			return err
		}
		return r.finish(ctx, op, replacement.State(), nil,
			`state = ? AND active_segment_id = ? AND updated_at = ?`,
			coarseState(expected.State()).databaseValue(), expected.ActiveSegmentID(), expected.UpdatedAt().UTC().UnixNano())
	})
}

// Write transactions may consume the Pending set before replacing its Runs.
// Decode the complete Run facts without requiring that transient relation;
// the enclosing application transaction owns the hand-off's integrity.
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

func (r *RunStore) requireTransition(ctx context.Context, operation string, state rundomain.Run) error {
	current, found, err := r.readRun(ctx, operation, state.ID(), scanRunForRecovery)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("sqlite: %s run %q: Run was not found", operation, state.ID())
	}
	if _, err := rundomain.Replace(current, func(rundomain.Run) (rundomain.Run, error) { return state, nil }); err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", operation, state.ID(), err)
	}
	return nil
}

// finish ends a non-terminal Run, writing the terminal state, its reason, and the
// facts that explain it in ONE statement — a row can never claim a terminal
// state without the result behind it, nor hold a result while still running.
// The terminal state was decided by the aggregate's transition before it got
// here and is not re-derived. fence names the committed source row: a
// Replacement fences the exact aggregate it was derived from, and an event
// commit fences its active Segment. Either way the UPDATE is a CAS, so a row
// that moved under the transaction fails instead of being overwritten.
func (r *RunStore) finish(
	ctx context.Context,
	op string,
	value rundomain.Run,
	marker *runCommitMarker,
	fence string,
	fenceArgs ...any,
) error {
	metrics, err := runMetricsRow(value.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, value.ID(), err)
	}
	failure, hasFailure := value.Failure()
	var failureRef *rundomain.Failure
	if hasFailure {
		failureRef = &failure
	}
	encodedEffects, err := encodeUnresolvedEffects(value.UnresolvedEffects())
	if err != nil {
		return err
	}
	encodedFailure, err := encodeRunFailure(failureRef)
	if err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, value.ID(), err)
	}
	outcome, _ := value.Outcome()
	commitSegmentID, commitID := marker.databaseValues()
	query :=
		`UPDATE runs SET
		   state = ?, active_segment_id = '', commit_segment_id = ?, commit_id = ?,
		   outcome = ?, detail = ?, steps = ?, active_duration_ns = ?,
		   usage = ?, context_tokens = ?, problem = ?, unresolved_effects = ?, message_mark = ?, finished_at = ?, updated_at = ?
		 WHERE session_id = ? AND run_id = ? AND ` + fence
	args := []any{
		coarseState(value.State()).databaseValue(), commitSegmentID, commitID,
		string(outcome), value.Detail(), metrics.steps, metrics.durationNs,
		metrics.usage, value.ContextTokens(), encodedFailure, encodedEffects,
		messageMarkValue(value.MessageMark()), value.FinishedAt().UTC().UnixNano(),
		value.UpdatedAt().UTC().UnixNano(), value.SessionID(), value.ID(),
	}
	args = append(args, fenceArgs...)
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		res, err := conn(ctx, r.db).ExecContext(ctx, query, args...)
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
		// A root Run's end is also a boundary of the session's Plan, and this CAS is
		// the only place a Run can reach terminal — so the boundary is stamped here
		// rather than by each caller that ends a Run, which is how "no terminal root
		// without a recorded boundary" holds by construction. A child ends inside its
		// root's tree, which is never cut before the root ends. Restore is deliberately
		// NOT a boundary: an imported Run finished in another runtime, and stamping the
		// importing session's live list would invent a value that Run never had.
		if value.Lineage().IsChild() {
			return nil
		}
		return NewPlanStore(r.db).CaptureBoundary(ctx, value.SessionID(), value.ID())
	})
}

// Restore inserts a complete terminal Run row for a session being imported or
// restored. It is not an admission: an imported Run has already finished, so it
// never claims the session's non-terminal slot and never passes through the
// state machine. A non-terminal Run is refused — restoring one would hand the
// session's admission slot to an executor that is not running.
func (r *RunStore) Restore(ctx context.Context, value rundomain.Run) error {
	if !value.State().IsTerminal() {
		return fmt.Errorf("sqlite: restore run %q: state is %s, want terminal", value.ID(), value.State())
	}
	lineage := value.Lineage()
	if lineage.IsChild() {
		if err := r.validateChildPlacement(
			ctx,
			"restore",
			value.ID(),
			value.SessionID(),
			lineage.ParentRunID,
			lineage.RootRunID,
			false,
		); err != nil {
			return err
		}
	}
	metrics, err := runMetricsRow(value.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	failure, hasFailure := value.Failure()
	var failureRef *rundomain.Failure
	if hasFailure {
		failureRef = &failure
	}
	encodedEffects, err := encodeUnresolvedEffects(value.UnresolvedEffects())
	if err != nil {
		return err
	}
	encodedFailure, err := encodeRunFailure(failureRef)
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	capabilities, err := encodeRunCapabilities(value.Capabilities())
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	outcome, _ := value.Outcome()
	selection := value.ModelSelection()
	_, err = conn(ctx, r.db).ExecContext(ctx,
		`INSERT INTO runs(
		   run_id, session_id, spawned_by_item_id, parent_run_id, root_run_id,
		   state, outcome, provider, model, reasoning_effort, goal_incarnation_id,
		   detail, steps, active_duration_ns, usage, context_tokens, problem, unresolved_effects,
		   capabilities, message_mark, created_at, finished_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID(), value.SessionID(),
		lineage.SpawnedByItemID, lineage.ParentRunID, lineage.RootRunID,
		coarseState(value.State()).databaseValue(), string(outcome),
		selection.Provider(), selection.Model(), selection.ReasoningEffort(),
		value.GoalIncarnationID(),
		value.Detail(), metrics.steps, metrics.durationNs, metrics.usage, value.ContextTokens(), encodedFailure, encodedEffects,
		capabilities, messageMarkValue(value.MessageMark()),
		value.CreatedAt().UTC().UnixNano(), value.FinishedAt().UTC().UnixNano(), value.UpdatedAt().UTC().UnixNano())
	if isPrimaryKeyViolation(err) {
		// A Run id belongs to one Session for its whole lifetime. An import that
		// would re-parent an existing Run is refused rather than silently taking it
		// over, which is what an upsert here would do.
		return fmt.Errorf("%w: run %q already exists", rundomain.ErrIdentityConflict, value.ID())
	}
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	return nil
}

// coarseState is the column value a Run in state s is stored under. It routes
// through the domain's lifecycle position so a row written by Suspend and a query
// filtering on [rundomain.StatusWaiting] cannot disagree about which value that
// is — the partial unique index keys on non-terminal, so every terminal State
// collapses to the one 'terminal' value (the fine reason lives in runs.outcome).
func coarseState(s rundomain.State) runState {
	return stateColumn(s.Status())
}

// stateColumn is the durable spelling of a lifecycle position. It stays an
// explicit table rather than [rundomain.Status.String]: these three strings are
// on disk and inside the partial unique index's predicate, so a Go rename must not
// be able to rewrite them.
func stateColumn(status rundomain.Status) runState {
	switch status {
	case rundomain.StatusWaiting:
		return runStateWaiting
	case rundomain.StatusFinished:
		return runStateTerminal
	default:
		return runStateRunning
	}
}

// Delete drops one Run's row. The rollback boundary uses it: a Run being dropped
// wholesale frees the session's admission slot by ceasing to exist, so there is
// nothing left to terminalize.
func (r *RunStore) Delete(ctx context.Context, sessionID, runID string) error {
	if err := validateSessionResource("delete Run", sessionID); err != nil {
		return err
	}
	if err := validateRunResource("delete Run", runID); err != nil {
		return err
	}
	if _, err := conn(ctx, r.db).ExecContext(ctx,
		`DELETE FROM runs WHERE run_id = ? AND session_id = ?`, runID, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: delete run: %w", err)
	}
	return nil
}

// DeleteForSession drops every Run row of a session whose durable state is being
// removed or replaced wholesale — the session-delete cascade, the import/restore
// replace, and the child-Run subtree purge. Freeing the admission slot by deletion
// (not terminalization) keeps the runs table from accumulating dead rows for
// sessions that no longer exist. Joins the caller's transaction via the context.
func (r *RunStore) DeleteForSession(ctx context.Context, sessionID string) error {
	if err := validateSessionResource("delete Session Runs", sessionID); err != nil {
		return err
	}
	_, err := conn(ctx, r.db).ExecContext(ctx,
		`DELETE FROM runs WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: delete runs for session: %w", err)
	}
	return nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE-index failure — for
// this table, the partial-unique-index rejection that means the session already
// holds a non-terminal run. A primary-key collision has its OWN extended code and
// does NOT appear here, which is what lets an id clash be told apart from a busy
// session. modernc.org/sqlite surfaces both as a typed *sqlite.Error carrying the
// extended result code.
func isUniqueViolation(err error) bool {
	se, ok := errors.AsType[*sqlite3.Error](err)
	return ok && se.Code() == sqlite3lib.SQLITE_CONSTRAINT_UNIQUE
}

// isPrimaryKeyViolation reports whether err is a SQLite PRIMARY KEY collision —
// here, a run id that already belongs to a Run.
func isPrimaryKeyViolation(err error) bool {
	se, ok := errors.AsType[*sqlite3.Error](err)
	return ok && se.Code() == sqlite3lib.SQLITE_CONSTRAINT_PRIMARYKEY
}
