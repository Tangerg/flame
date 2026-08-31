package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
// the session already has a non-terminal Run — and
// [rundomain.ErrIdentityConflict] when the Run ID is already taken, since the
// caller may supply one.
func (r *RunStore) Admit(ctx context.Context, draft rundomain.Draft) error {
	admitted, err := rundomain.Admit(draft)
	if err != nil {
		return fmt.Errorf("sqlite: admit run %q: %w", draft.RunID, err)
	}
	lineage := admitted.Lineage()
	capabilities, err := encodeRunCapabilities(runCapabilitiesForStorage(admitted))
	if err != nil {
		return fmt.Errorf("sqlite: admit run %q: %w", draft.RunID, err)
	}
	now := admitted.CreatedAt().UnixNano()
	maxTotalTokens, maxSteps, maxBudgetUSD := runLimitColumnValues(admitted.Limits())
	// This is the capability set's only writer, here and in Restore. Suspend,
	// resume, and finish deliberately do not name the column: the value cannot change
	// after admission, and the way to guarantee that is to have nothing able to
	// change it.
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
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
		_, err := conn(ctx, r.db).ExecContext(ctx,
			`INSERT INTO runs(
			   run_id, session_id, spawned_by_item_id, parent_run_id, root_run_id,
			   state, active_segment_id, provider, model, reasoning_effort, goal_incarnation_id, max_total_tokens, max_steps, max_budget_usd,
			   capabilities, message_mark, started_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			admitted.ID(), admitted.SessionID(),
			lineage.SpawnedByItemID, lineage.ParentRunID, lineage.RootRunID,
			runStateRunning.databaseValue(), admitted.ActiveSegmentID(),
			admitted.ModelSelection().Provider(), admitted.ModelSelection().Model(), admitted.ModelSelection().ReasoningEffort(),
			admitted.GoalIncarnationID(),
			maxTotalTokens, maxSteps, maxBudgetUSD, capabilities,
			rundomain.UnknownMessageMark, now, now)
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

// Suspend persists the exact aggregate transition from Running to Waiting,
// recording what the Run had consumed up to the park. A missing row,
// repeated transition, mismatched identity, or any other source state is an
// ownership error and never succeeds silently.
func (r *RunStore) Suspend(ctx context.Context, value rundomain.Run) error {
	return r.suspend(ctx, value, runCommitIdentity{})
}

// SuspendBarrier parks one exact active Segment and stamps the root-owned tree
// barrier identity in the same transition. Child Runs use Suspend without a
// marker; the root marker proves the complete multi-Run transaction.
func (r *RunStore) SuspendBarrier(
	ctx context.Context,
	value rundomain.Run,
	segmentID string,
	commitID runtimeidentity.CommitID,
) error {
	if err := validateRunCommitIdentity(value.SessionID(), value.ID(), segmentID, commitID); err != nil {
		return err
	}
	return r.suspend(ctx, value, runCommitIdentity{segmentID: segmentID, commitID: commitID})
}

func (r *RunStore) suspend(
	ctx context.Context,
	value rundomain.Run,
	commit runCommitIdentity,
) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("sqlite: suspend run %q: %w", value.ID(), err)
	}
	if value.State() != rundomain.Waiting {
		return fmt.Errorf("sqlite: suspend run %q: state is %s, want waiting", value.ID(), value.State())
	}
	metrics, err := runMetricsRow(value.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: suspend run %q: %w", value.ID(), err)
	}
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		current, found, err := r.runForTransition(ctx, value.ID())
		if err != nil {
			return err
		}
		if !found || current.SessionID() != value.SessionID() {
			return errors.New("sqlite: suspend run: active run not found")
		}
		if !commit.commitID.IsZero() && current.ActiveSegmentID() != commit.segmentID {
			return fmt.Errorf(
				"sqlite: suspend run: active Segment is %q, want %q",
				current.ActiveSegmentID(), commit.segmentID,
			)
		}
		next, err := current.AdvanceProgress(
			value.Metrics(), value.ContextTokens(), value.UpdatedAt(),
		)
		if err != nil {
			return fmt.Errorf("sqlite: suspend run: advance aggregate metrics: %w", err)
		}
		next, err = next.Suspend(value.UpdatedAt())
		if err != nil {
			return fmt.Errorf("sqlite: suspend run: %w", err)
		}
		if !next.Equal(value) {
			return errors.New("sqlite: suspend run: proposed Run differs from the aggregate transition")
		}
		// The segment identity is cleared in the same statement that parks the Run:
		// a Run waiting on a person has no segment to attach to.
		res, err := conn(ctx, r.db).ExecContext(ctx,
			`UPDATE runs SET state = ?, active_segment_id = '', commit_segment_id = ?, commit_id = ?,
			        steps = ?, active_duration_ns = ?, usage = ?, context_tokens = ?, updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ?`,
			coarseState(next.State()).databaseValue(), commit.segmentID, commit.commitID.String(),
			metrics.steps, metrics.durationNs, metrics.usage, next.ContextTokens(), runUpdatedAt(value),
			value.SessionID(), value.ID(), coarseState(current.State()).databaseValue())
		if err != nil {
			return fmt.Errorf("sqlite: suspend run: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: suspend run: read affected rows: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("sqlite: suspend run: state changed concurrently (was %s)", current.State())
		}
		return nil
	})
}

// Resume continues the exact parked Run (Waiting → Running). Unlike cleanup
// transitions it is strict: a missing/mismatched/already-running row means the
// continuation opening does not own the durable Run and must roll back.
func (r *RunStore) Resume(
	ctx context.Context,
	sessionID string,
	draft rundomain.ResumeDraft,
	resumedAt time.Time,
) error {
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		current, found, err := r.runForTransition(ctx, draft.RunID)
		if err != nil {
			return err
		}
		if !found || current.SessionID() != sessionID {
			return errors.New("sqlite: resume run: active run not found")
		}
		next, err := current.Resume(draft.SegmentID, resumedAt)
		if err != nil {
			return fmt.Errorf("sqlite: resume run: %w", err)
		}
		// The accrual is untouched: a continuation inherits what the park committed,
		// and the segment now opening has consumed nothing yet. What does move is the
		// segment identity, which the park cleared and this one replaces.
		res, err := conn(ctx, r.db).ExecContext(ctx,
			`UPDATE runs SET state = ?, active_segment_id = ?, commit_segment_id = '', commit_id = '', updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ?`,
			coarseState(next.State()).databaseValue(), next.ActiveSegmentID(), next.UpdatedAt().UnixNano(),
			sessionID, draft.RunID, coarseState(current.State()).databaseValue())
		if err != nil {
			return fmt.Errorf("sqlite: resume run: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: resume run: read affected rows: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("sqlite: resume run: state changed concurrently (was %s)", current.State())
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

// UpdateProgress records cumulative accounting and the latest prompt footprint
// observed at one model-call boundary while fencing both facts to the exact
// active segment. It never moves lifecycle state and rejects stale or regressing
// cumulative accounting.
func (r *RunStore) UpdateProgress(
	ctx context.Context,
	sessionID string,
	runID string,
	segmentID string,
	metrics rundomain.Metrics,
	contextTokens int64,
	updatedAt time.Time,
) error {
	if err := validateRunCoordinates("update Run progress", sessionID, runID, segmentID); err != nil {
		return err
	}
	if updatedAt.IsZero() {
		return errors.New("sqlite: update Run progress requires an update time")
	}
	if err := metrics.Validate(); err != nil {
		return fmt.Errorf("sqlite: update Run progress for %q: %w", runID, err)
	}
	if contextTokens < 0 {
		return fmt.Errorf("sqlite: update Run progress for %q: context tokens must not be negative", runID)
	}
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		current, found, err := r.Run(ctx, runID)
		if err != nil {
			return err
		}
		if !found || current.SessionID() != sessionID {
			return fmt.Errorf("sqlite: update Run progress: running Run %q was not found in session %q", runID, sessionID)
		}
		if current.State() != rundomain.Running || current.ActiveSegmentID() != segmentID {
			return fmt.Errorf(
				"sqlite: update Run progress: Run %q is %s in segment %q, want running segment %q",
				runID,
				current.State(),
				current.ActiveSegmentID(),
				segmentID,
			)
		}
		next, err := current.AdvanceProgress(metrics, contextTokens, updatedAt)
		if err != nil {
			return fmt.Errorf("sqlite: update Run progress for %q: %w", runID, err)
		}
		encoded, err := runMetricsRow(next.Metrics())
		if err != nil {
			return fmt.Errorf("sqlite: update Run progress for %q: %w", runID, err)
		}
		result, err := conn(ctx, r.db).ExecContext(ctx,
			`UPDATE runs SET steps = ?, active_duration_ns = ?, usage = ?, context_tokens = ?, updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ?`,
			encoded.steps,
			encoded.durationNs,
			encoded.usage,
			next.ContextTokens(),
			updatedAt.UTC().UnixNano(),
			sessionID,
			runID,
			runStateRunning.databaseValue(),
			segmentID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: update Run progress for %q: %w", runID, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect Run progress update for %q: %w", runID, err)
		}
		if changed != 1 {
			return fmt.Errorf("sqlite: update Run progress for %q lost its active-segment fence", runID)
		}
		return nil
	})
}

// Terminalize ends the exact non-terminal Run that run identifies, recording the
// outcome the executor reached and the result that explains it.
func (r *RunStore) Terminalize(ctx context.Context, value rundomain.Run) error {
	return r.terminalize(ctx, value, runCommitIdentity{})
}

// RecordRunCommit stamps one exact active Segment's latest immutable
// Application write-set identity into the Run row. Callers invoke it only at
// the end of the command transaction, after every projection has succeeded.
func (r *RunStore) RecordRunCommit(
	ctx context.Context,
	sessionID string,
	runID string,
	segmentID string,
	commitID runtimeidentity.CommitID,
) error {
	if err := validateRunCommitIdentity(sessionID, runID, segmentID, commitID); err != nil {
		return err
	}
	result, err := conn(ctx, r.db).ExecContext(ctx,
		`UPDATE runs SET commit_segment_id = ?, commit_id = ?
		  WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ?`,
		segmentID,
		commitID.String(),
		sessionID,
		runID,
		runStateRunning.databaseValue(),
		segmentID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: record Run commit %q: %w", commitID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect Run commit %q marker: %w", commitID, err)
	}
	if changed != 1 {
		return fmt.Errorf("sqlite: Run commit %q lost its active-segment fence", commitID)
	}
	return nil
}

// RecordWaitingRunCommit stamps a command that transforms an already-waiting
// tree without opening a new Segment. The empty Segment is deliberate: the
// unique command identity and Waiting root own this boundary.
func (r *RunStore) RecordWaitingRunCommit(
	ctx context.Context,
	sessionID string,
	runID string,
	commitID runtimeidentity.CommitID,
) error {
	if err := validateWaitingRunCommitIdentity(sessionID, runID, commitID); err != nil {
		return err
	}
	result, err := conn(ctx, r.db).ExecContext(ctx,
		`UPDATE runs SET commit_segment_id = '', commit_id = ?
		  WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ''`,
		commitID.String(),
		sessionID,
		runID,
		runStateWaiting.databaseValue(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: record waiting Run commit %q: %w", commitID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect waiting Run commit %q marker: %w", commitID, err)
	}
	if changed != 1 {
		return fmt.Errorf("sqlite: waiting Run commit %q lost its state fence", commitID)
	}
	return nil
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
	if err := validateRunCommitIdentity(value.SessionID(), value.ID(), segmentID, commitID); err != nil {
		return err
	}
	return r.terminalize(ctx, value, runCommitIdentity{segmentID: segmentID, commitID: commitID})
}

type runCommitIdentity struct {
	segmentID string
	commitID  runtimeidentity.CommitID
}

func (r *RunStore) terminalize(
	ctx context.Context,
	value rundomain.Run,
	identity runCommitIdentity,
) error {
	return r.finish(ctx, "terminalize", value, identity, func(current rundomain.Run) (rundomain.Run, error) {
		outcome, terminal := value.Outcome()
		if !terminal {
			return rundomain.Run{}, errors.New("outcome is required")
		}
		failure, failed := value.Failure()
		var failureRef *rundomain.Failure
		if failed {
			failureRef = &failure
		}
		return current.Terminate(rundomain.Termination{
			Outcome: outcome, Detail: value.Detail(), Failure: failureRef,
			FinishedAt: value.FinishedAt(), MessageMark: value.MessageMark(),
		})
	})
}

// RunCommitCommitted proves that this exact immutable Application Run write-set crossed
// the durable boundary. It does not infer success from the coarse Run state:
// another Segment, restored/resumed Run, or later write attempt has a different
// or absent marker. Running markers require the same active Segment; waiting
// barriers and terminal boundaries retain the Segment that produced them.
// A command that starts and ends while already Waiting uses an empty Segment.
func (r *RunStore) RunCommitCommitted(
	ctx context.Context,
	sessionID string,
	runID string,
	segmentID string,
	commitID runtimeidentity.CommitID,
) (bool, error) {
	if segmentID == "" {
		if err := validateWaitingRunCommitIdentity(sessionID, runID, commitID); err != nil {
			return false, err
		}
	} else if err := validateRunCommitIdentity(sessionID, runID, segmentID, commitID); err != nil {
		return false, err
	}
	var found int
	err := conn(ctx, r.db).QueryRowContext(ctx,
		`SELECT count(*)
		   FROM runs
		  WHERE session_id = ? AND run_id = ?
		    AND commit_segment_id = ? AND commit_id = ?
		    AND ((state = ? AND active_segment_id = ?) OR state IN (?, ?))`,
		sessionID,
		runID,
		segmentID,
		commitID.String(),
		runStateRunning.databaseValue(),
		segmentID,
		runStateWaiting.databaseValue(),
		runStateTerminal.databaseValue(),
	).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("sqlite: verify Run commit %q: %w", commitID, err)
	}
	return found == 1, nil
}

func validateRunCommitIdentity(sessionID, runID, segmentID string, commitID runtimeidentity.CommitID) error {
	if err := validateRunCoordinates("verify Run commit", sessionID, runID, segmentID); err != nil {
		return err
	}
	if err := commitID.Validate(); err != nil {
		return fmt.Errorf("sqlite: verify Run commit: %w", err)
	}
	return nil
}

func validateRunCoordinates(operation, sessionID, runID, segmentID string) error {
	if err := validateSessionResource(operation, sessionID); err != nil {
		return err
	}
	if err := validateRunResource(operation, runID); err != nil {
		return err
	}
	if err := validateSegmentResource(operation, segmentID); err != nil {
		return err
	}
	return nil
}

func validateWaitingRunCommitIdentity(sessionID, runID string, commitID runtimeidentity.CommitID) error {
	if err := validateSessionResource("verify waiting Run commit", sessionID); err != nil {
		return err
	}
	if err := validateRunResource("verify waiting Run commit", runID); err != nil {
		return err
	}
	if err := commitID.Validate(); err != nil {
		return fmt.Errorf("sqlite: verify waiting Run commit: %w", err)
	}
	return nil
}

// RebaseMessageMark applies an exact Application-decided coordinate rewrite to
// one terminal Run. Compaction does not change when the Run happened or any of
// its lifecycle facts, so updated_at deliberately remains untouched.
func (r *RunStore) RebaseMessageMark(ctx context.Context, expected, replacement rundomain.Run) error {
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("sqlite: rebase Run message watermark: expected Run: %w", err)
	}
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sqlite: rebase Run message watermark: replacement Run: %w", err)
	}
	if !expected.State().IsTerminal() || !replacement.State().IsTerminal() {
		return errors.New("sqlite: rebase Run message watermark: terminal Run is required")
	}
	if expected.ID() != replacement.ID() || expected.SessionID() != replacement.SessionID() {
		return errors.New("sqlite: rebase Run message watermark changes identity")
	}
	derived, err := expected.WithMessageMark(replacement.MessageMark())
	if err != nil {
		return fmt.Errorf("sqlite: rebase Run message watermark: %w", err)
	}
	if !derived.Equal(replacement) {
		return errors.New("sqlite: rebase Run message watermark changes non-watermark facts")
	}
	result, err := conn(ctx, r.db).ExecContext(ctx,
		`UPDATE runs SET message_mark = ?
		 WHERE session_id = ? AND run_id = ? AND state = ? AND message_mark = ?`,
		replacement.MessageMark(), expected.SessionID(), expected.ID(), runStateTerminal.databaseValue(), expected.MessageMark(),
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
}

// RecoverLost ends the exact non-terminal Run whose executor state is no longer
// resumable. Unlike Terminalize, this recovery transition is legal from either
// Running or Waiting, because it describes a Run nobody is driving rather
// than one the executor finished.
func (r *RunStore) RecoverLost(ctx context.Context, value rundomain.Run) error {
	return r.finish(ctx, "recover lost", value, runCommitIdentity{}, func(current rundomain.Run) (rundomain.Run, error) {
		failure, failed := value.Failure()
		if !failed {
			return rundomain.Run{}, errors.New("lost failure is required")
		}
		return current.RecoverLost(failure, value.FinishedAt(), value.MessageMark())
	})
}

// finish ends a non-terminal Run, writing the terminal state, its reason, and the
// facts that explain it in ONE statement — a row can never claim a terminal
// state without the result behind it, nor hold a result while still running.
// transition invokes the aggregate's rule for this kind of ending; the UPDATE
// is a CAS on the committed source state, so a row that moved under the
// transaction fails instead of being overwritten.
func (r *RunStore) finish(
	ctx context.Context,
	op string,
	value rundomain.Run,
	commit runCommitIdentity,
	transition func(rundomain.Run) (rundomain.Run, error),
) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, value.ID(), err)
	}
	metrics, err := runMetricsRow(value.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, value.ID(), err)
	}
	failure, hasFailure := value.Failure()
	var failureRef *rundomain.Failure
	if hasFailure {
		failureRef = &failure
	}
	encodedFailure, err := encodeRunFailure(failureRef)
	if err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, value.ID(), err)
	}
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		current, found, err := r.runForTransition(ctx, value.ID())
		if err != nil {
			return err
		}
		if !found || current.SessionID() != value.SessionID() {
			return fmt.Errorf("sqlite: %s run: active run not found", op)
		}
		if !commit.commitID.IsZero() && current.ActiveSegmentID() != commit.segmentID {
			return fmt.Errorf(
				"sqlite: %s run: active Segment is %q, want %q",
				op,
				current.ActiveSegmentID(),
				commit.segmentID,
			)
		}
		current, err = current.AdvanceProgress(
			value.Metrics(), value.ContextTokens(), value.FinishedAt(),
		)
		if err != nil {
			return fmt.Errorf("sqlite: %s run: advance aggregate metrics: %w", op, err)
		}
		next, err := transition(current)
		if err != nil {
			return fmt.Errorf("sqlite: %s run: %w", op, err)
		}
		if !next.Equal(value) {
			return fmt.Errorf("sqlite: %s run: proposed Run differs from the aggregate transition", op)
		}
		outcome, _ := value.Outcome()
		query :=
			`UPDATE runs SET
			   state = ?, active_segment_id = '', commit_segment_id = ?, commit_id = ?,
			   outcome = ?, detail = ?, steps = ?, active_duration_ns = ?,
			   usage = ?, context_tokens = ?, problem = ?, message_mark = ?, finished_at = ?, updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ?`
		args := []any{
			coarseState(next.State()).databaseValue(), commit.segmentID, commit.commitID.String(),
			outcome.String(), value.Detail(), metrics.steps, metrics.durationNs,
			metrics.usage, next.ContextTokens(), encodedFailure,
			value.MessageMark(), value.FinishedAt().UTC().UnixNano(),
			runUpdatedAt(value), value.SessionID(), value.ID(), coarseState(current.State()).databaseValue(),
		}
		if !commit.commitID.IsZero() {
			query += ` AND active_segment_id = ?`
			args = append(args, commit.segmentID)
		}
		res, err := conn(ctx, r.db).ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("sqlite: %s run: %w", op, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: %s run: read affected rows: %w", op, err)
		}
		if n == 0 {
			return fmt.Errorf("sqlite: %s run: state changed concurrently (was %s)", op, current.State())
		}
		// The Run's end is also a boundary of the session's Plan, and this CAS is
		// the only place a Run can reach terminal — so the boundary is stamped here
		// rather than by each caller that ends a Run, which is how "no terminal Run
		// without a recorded boundary" holds by construction. Restore is deliberately
		// NOT a boundary: an imported Run finished in another runtime, and stamping the
		// importing session's live list would invent a value that Run never had.
		return NewPlanStore(r.db).CaptureBoundary(ctx, value.SessionID(), value.ID())
	})
}

// Restore inserts a complete terminal Run row for a session being imported or
// restored. It is not an admission: an imported Run has already finished, so it
// never claims the session's non-terminal slot and never passes through the
// state machine. A non-terminal Run is refused — restoring one would hand the
// session's admission slot to an executor that is not running.
func (r *RunStore) Restore(ctx context.Context, value rundomain.Run) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
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
	encodedFailure, err := encodeRunFailure(failureRef)
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	capabilities, err := encodeRunCapabilities(runCapabilitiesForStorage(value))
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	outcome, _ := value.Outcome()
	selection := value.ModelSelection()
	limits := value.Limits()
	maxTotalTokens, maxSteps, maxBudgetUSD := runLimitColumnValues(limits)
	_, err = conn(ctx, r.db).ExecContext(ctx,
		`INSERT INTO runs(
		   run_id, session_id, spawned_by_item_id, parent_run_id, root_run_id,
		   state, outcome, provider, model, reasoning_effort, goal_incarnation_id,
		   detail, steps, active_duration_ns, usage, context_tokens, problem,
		   max_total_tokens, max_steps, max_budget_usd,
		   capabilities, message_mark, started_at, finished_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID(), value.SessionID(),
		lineage.SpawnedByItemID, lineage.ParentRunID, lineage.RootRunID,
		coarseState(value.State()).databaseValue(), outcome.String(),
		selection.Provider(), selection.Model(), selection.ReasoningEffort(),
		value.GoalIncarnationID(),
		value.Detail(), metrics.steps, metrics.durationNs, metrics.usage, value.ContextTokens(), encodedFailure,
		maxTotalTokens, maxSteps, maxBudgetUSD, capabilities, value.MessageMark(),
		value.CreatedAt().UTC().UnixNano(), value.FinishedAt().UTC().UnixNano(), runUpdatedAt(value))
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

// runCapabilitiesForStorage keeps the root row as the single durable author.
// Child aggregates inherit the materialized value in memory, but persisting a
// second copy would let one Run tree carry contradictory capability sets.
func runCapabilitiesForStorage(value rundomain.Run) rundomain.Capabilities {
	if value.Lineage().IsChild() {
		return rundomain.Capabilities{}
	}
	return value.Capabilities()
}

// runForTransition reads the aggregate that a write is about to advance. It
// tolerates a temporarily absent Pending row because write-sets may delete that
// row before terminalizing the Run in the same transaction; the proposed
// aggregate transition remains the authority for whether the write is legal.
func (r *RunStore) runForTransition(ctx context.Context, runID string) (rundomain.Run, bool, error) {
	if err := validateRunResource("read Run for transition", runID); err != nil {
		return rundomain.Run{}, false, err
	}
	row := conn(ctx, r.db).QueryRowContext(ctx,
		`SELECT `+runColumns+`
		 FROM runs AS r
		 `+runReadJoins+`
		 WHERE r.run_id = ?`, runID)
	value, err := scanRunForRecovery(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return rundomain.Run{}, false, nil
	case err != nil:
		return rundomain.Run{}, false, fmt.Errorf("sqlite: read Run %q for transition: %w", runID, err)
	}
	return value, true, nil
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

// runUpdatedAt is the row's touch time. A caller that recorded one is honored so
// a restored Run keeps the timestamp it was exported with; otherwise the write
// stamps itself.
func runUpdatedAt(run rundomain.Run) int64 {
	if run.UpdatedAt().IsZero() {
		return time.Now().UTC().UnixNano()
	}
	return run.UpdatedAt().UTC().UnixNano()
}
