package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/qimu/integrity"
	"github.com/Tangerg/qimu/sqlite"
)

// Session heads and terminal Run boundaries hold references so a fork can retain
// precisely the reviewed content even after an unrelated Session advances.
type PlanStore struct{ db *sql.DB }

func newPlanStore(db *sql.DB) *PlanStore { return &PlanStore{db: db} }

func (p *PlanStore) List(ctx context.Context, sessionID string) ([]plan.Step, error) {
	state, err := p.State(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return state.Steps(), nil
}

// State returns an explicit unwritten Current when no Plan row exists.
func (p *PlanStore) State(ctx context.Context, sessionID string) (plan.Current, error) {
	if err := validateSessionResource("read Session Plan", sessionID); err != nil {
		return plan.Current{}, err
	}
	var (
		stepsJSON, sourceJSON string
		digest                integrity.Digest
		revision              uint64
		updatedNs             int64
	)
	err := sqlite.Connection(ctx, p.db).QueryRowContext(ctx,
		`SELECT doc.steps, doc.digest, source,revision, updated_at FROM session_plans JOIN plan_documents doc ON doc.digest = document WHERE session_id = ?`, sessionID,
	).Scan(&stepsJSON, &digest, &sourceJSON, &revision, &updatedNs)
	if errors.Is(err, sql.ErrNoRows) {
		return plan.Current{}, nil
	}
	if err != nil {
		return plan.Current{}, fmt.Errorf("sqlite: read Plan: %w", err)
	}
	steps, err := decodePlanSteps(stepsJSON, digest.String())
	if err != nil {
		return plan.Current{}, err
	}
	var source extension.ContributionSource
	if err := decodeStoredJSON([]byte(sourceJSON), &source); err != nil {
		return plan.Current{}, err
	}
	state, err := plan.Restore(plan.Snapshot{Source: source,
		Steps: steps, Revision: revision, UpdatedAt: time.Unix(0, updatedNs).UTC(),
	})
	if err != nil {
		return plan.Current{}, fmt.Errorf("sqlite: restore Plan: %w", err)
	}
	current, err := plan.CurrentOf(state)
	if err != nil {
		return plan.Current{}, fmt.Errorf("sqlite: own Plan state: %w", err)
	}
	return current, nil
}

func decodePlanSteps(stepsJSON, digest string) ([]plan.Step, error) {
	var rows []planworkflow.Step
	if err := decodeStoredJSON([]byte(stepsJSON), &rows); err != nil {
		return nil, fmt.Errorf("sqlite: decode Plan: %w", err)
	}
	if rows == nil {
		return nil, errors.New("sqlite: stored Plan steps must be an array")
	}
	steps := plan.StepsOf(planworkflow.Proposal{Steps: rows})
	if err := plan.ValidateSteps(steps); err != nil {
		return nil, fmt.Errorf("sqlite: validate Plan: %w", err)
	}
	if integrity.Hash([]byte(stepsJSON)).String() != digest {
		return nil, errors.New("sqlite: immutable Plan document digest mismatch")
	}
	return steps, nil
}

// Save persists one Domain-decided replacement iff its expected version is
// still current. It assigns neither time nor revision.
func (p *PlanStore) Save(ctx context.Context, sessionID string, change plan.Replacement) error {
	if err := validateSessionResource("save Session Plan", sessionID); err != nil {
		return err
	}
	replacement := change.State()
	steps := replacement.Steps()
	data, err := encodeStoredJSON(plan.ProposalOf(steps).Steps)
	if err != nil {
		return fmt.Errorf("sqlite: encode Plan: %w", err)
	}
	digest := integrity.Hash(data)
	return sqlite.RunInTx(ctx, p.db, func(ctx context.Context) error {
		if _, err := sqlite.Connection(ctx, p.db).ExecContext(ctx,
			`INSERT INTO plan_documents(digest, steps) VALUES (?, ?) ON CONFLICT(digest) DO NOTHING`,
			digest.String(), string(data)); err != nil {
			return fmt.Errorf("sqlite: store immutable Plan: %w", err)
		}
		return p.advance(ctx, sessionID, digest, change)
	})
}

func (p *PlanStore) advance(ctx context.Context, sessionID string, digest integrity.Digest, change plan.Replacement) error {
	expected := change.ExpectedVersion()
	replacement := change.State()
	source, encodeErr := encodeStoredJSON(replacement.Document().Source)
	if encodeErr != nil {
		return encodeErr
	}
	var result sql.Result
	var err error
	if expected.IsUnwritten() {
		result, err = sqlite.Connection(ctx, p.db).ExecContext(ctx,
			`INSERT INTO session_plans(session_id, document, source,revision, updated_at)
			 VALUES (?, ?, ?, ?,?) ON CONFLICT(session_id) DO NOTHING`,
			sessionID, digest.String(), string(source), replacement.Revision(), replacement.UpdatedAt().UnixNano(),
		)
	} else {
		expectedRevision, committed := expected.Revision()
		if !committed {
			return fmt.Errorf("sqlite: Plan expected version lost committed revision: %w", plan.ErrInvalid)
		}
		result, err = sqlite.Connection(ctx, p.db).ExecContext(ctx,
			`UPDATE session_plans SET document = ?,source=?, revision = ?, updated_at = ?
			 WHERE session_id = ? AND revision = ?`,
			digest.String(), string(source), replacement.Revision(), replacement.UpdatedAt().UnixNano(), sessionID, expectedRevision,
		)
	}
	if err != nil {
		return fmt.Errorf("sqlite: save Plan: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect Plan save: %w", err)
	}
	if written != 1 {
		return fmt.Errorf("sqlite: expected Plan %s: %w", expected, plan.ErrRevisionConflict)
	}
	return nil
}

// CaptureBoundary freezes the session's current Plan at one terminal Run.
func (p *PlanStore) CaptureBoundary(ctx context.Context, sessionID, runID string) error {
	if err := validateSessionResource("capture Plan boundary", sessionID); err != nil {
		return err
	}
	if err := validateRunResource("capture Plan boundary", runID); err != nil {
		return err
	}
	if _, err := sqlite.Connection(ctx, p.db).ExecContext(ctx,
		`INSERT INTO plan_boundaries(run_id,document,source)
		 SELECT ?,p.document,COALESCE(p.source,'{}') FROM (SELECT ? AS session_id) s LEFT JOIN session_plans p ON p.session_id=s.session_id`,
		runID, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: capture Plan boundary for Run %q: %w", runID, err)
	}
	return nil
}

// Boundary returns the Plan captured by runID. recorded=false means the Run
// never captured a boundary; it does not mean an empty Plan.
func (p *PlanStore) Boundary(ctx context.Context, runID string) (plan.Document, bool, error) {
	if err := validateRunResource("read Plan boundary", runID); err != nil {
		return plan.Document{}, false, err
	}
	var sourceJSON string
	var stepsJSON, digest sql.NullString
	err := sqlite.Connection(ctx, p.db).QueryRowContext(ctx,
		`SELECT doc.steps, doc.digest,b.source FROM plan_boundaries b LEFT JOIN plan_documents doc ON doc.digest = b.document WHERE run_id = ?`, runID,
	).Scan(&stepsJSON, &digest, &sourceJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return plan.Document{}, false, nil
	}
	if err != nil {
		return plan.Document{}, false, fmt.Errorf("sqlite: read Plan boundary for Run %q: %w", runID, err)
	}
	if !digest.Valid {
		return plan.Document{Steps: []plan.Step{}}, true, nil
	}
	steps, err := decodePlanSteps(stepsJSON.String, digest.String)
	if err != nil {
		return plan.Document{}, false, err
	}
	var source extension.ContributionSource
	if err := decodeStoredJSON([]byte(sourceJSON), &source); err != nil {
		return plan.Document{}, false, err
	}
	return plan.Document{Steps: steps, Source: source}, true, nil
}

func (p *PlanStore) DeleteSession(ctx context.Context, sessionID string) error {
	if err := validateSessionResource("delete Session Plan", sessionID); err != nil {
		return err
	}
	if _, err := sqlite.Connection(ctx, p.db).ExecContext(ctx,
		`DELETE FROM session_plans WHERE session_id = ?`, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: delete session Plan: %w", err)
	}
	return nil
}
