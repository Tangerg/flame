package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	framevalue "github.com/Tangerg/frame/value"
)

// Session heads and terminal Run boundaries hold references so a fork can retain
// precisely the reviewed content even after an unrelated Session advances.
type PlanStore struct{ db *database }

func newPlanStore(db *database) *PlanStore {
	return &PlanStore{db: db}
}
func (p *PlanStore) List(ctx context.Context, sessionID identity.SessionID) ([]plan.Step, error) {
	state, err := p.State(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return state.Steps(), nil
}

// State returns an explicit unwritten Current when no Plan row exists.
func (p *PlanStore) State(ctx context.Context, sessionID identity.SessionID) (plan.Current, error) {
	return scopedValue(ctx, p.db.read, func(ctx context.Context) (plan.Current, error) {
		if err := validateResource("read Session Plan", sessionID); err != nil {
			return plan.Current{}, err
		}
		var (
			stepsJSON, sourceJSON string
			digest                framevalue.Digest
			revision              uint64
			updatedNs             int64
		)
		err := p.db.connection(ctx).QueryRowContext(ctx, `SELECT doc.steps, doc.digest, source,revision, updated_at FROM session_plans JOIN plan_documents doc ON doc.digest = document WHERE session_id = ?`, sessionID).Scan(&stepsJSON, &digest, &sourceJSON, &revision, &updatedNs)
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
		state, err := plan.Restore(plan.Snapshot{Source: source, Steps: steps, Revision: revision, UpdatedAt: time.Unix(0, updatedNs).UTC()})
		if err != nil {
			return plan.Current{}, fmt.Errorf("sqlite: restore Plan: %w", err)
		}
		current, err := plan.CurrentOf(state)
		if err != nil {
			return plan.Current{}, fmt.Errorf("sqlite: own Plan state: %w", err)
		}
		return current, nil
	})
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
	if framevalue.Hash([]byte(stepsJSON)).String() != digest {
		return nil, errors.New("sqlite: immutable Plan document digest mismatch")
	}
	return steps, nil
}

// Save persists one Domain-decided replacement iff its expected version is
// still current. It assigns neither time nor revision.
func (p *PlanStore) Save(ctx context.Context, sessionID identity.SessionID, change plan.Replacement) error {
	if err := validateResource("save Session Plan", sessionID); err != nil {
		return err
	}
	return p.db.write(ctx, func(ctx context.Context) error {
		digest, err := p.storeDocument(ctx, change.State().Steps())
		if err != nil {
			return err
		}
		return p.advance(ctx, sessionID, digest, change)
	})
}

func (p *PlanStore) storeDocument(ctx context.Context, steps []plan.Step) (framevalue.Digest, error) {
	data, err := encodeStoredJSON(plan.ProposalOf(steps).Steps)
	if err != nil {
		return framevalue.Digest{}, fmt.Errorf("sqlite: encode Plan: %w", err)
	}
	digest := framevalue.Hash(data)
	if _, err := p.db.connection(ctx).ExecContext(ctx, `INSERT INTO plan_documents(digest, steps) VALUES (?, ?) ON CONFLICT(digest) DO NOTHING`, digest.String(), string(data)); err != nil {
		return framevalue.Digest{}, fmt.Errorf("sqlite: store immutable Plan: %w", err)
	}
	return digest, nil
}
func (p *PlanStore) advance(ctx context.Context, sessionID identity.SessionID, digest framevalue.Digest, change plan.Replacement) error {
	return p.db.write(ctx, func(ctx context.Context) error {
		expected := change.ExpectedVersion()
		replacement := change.State()
		source, encodeErr := encodeStoredJSON(replacement.Document().Source)
		if encodeErr != nil {
			return encodeErr
		}
		var result sql.Result
		var err error
		if expected.IsUnwritten() {
			result, err = p.db.connection(ctx).ExecContext(ctx, `INSERT INTO session_plans(session_id, document, source,revision, updated_at)
			 VALUES (?, ?, ?, ?,?) ON CONFLICT(session_id) DO NOTHING`, sessionID, digest.String(), string(source), replacement.Revision(), replacement.UpdatedAt().UnixNano())
		} else {
			expectedRevision, committed := expected.Revision()
			if !committed {
				return fmt.Errorf("sqlite: Plan expected version lost committed revision: %w", plan.ErrInvalid)
			}
			result, err = p.db.connection(ctx).ExecContext(ctx, `UPDATE session_plans SET document = ?,source=?, revision = ?, updated_at = ?
			 WHERE session_id = ? AND revision = ?`, digest.String(), string(source), replacement.Revision(), replacement.UpdatedAt().UnixNano(), sessionID, expectedRevision)
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
	})
}

// CaptureBoundary freezes the session's current Plan at one terminal Run.
func (p *PlanStore) CaptureBoundary(ctx context.Context, sessionID identity.SessionID, runID identity.RunID) error {
	return p.db.write(ctx, func(ctx context.Context) error {
		if err := validateResource("capture Plan boundary", sessionID); err != nil {
			return err
		}
		if err := validateResource("capture Plan boundary", runID); err != nil {
			return err
		}
		if _, err := p.db.connection(ctx).ExecContext(ctx, `INSERT INTO plan_boundaries(run_id,document,source)
		 SELECT ?,p.document,COALESCE(p.source,'{}') FROM (SELECT ? AS session_id) s LEFT JOIN session_plans p ON p.session_id=s.session_id`, runID, sessionID); err != nil {
			return fmt.Errorf("sqlite: capture Plan boundary for Run %q: %w", runID, err)
		}
		return nil
	})
}

// Boundary returns the Plan captured by runID. recorded=false means the Run
// never captured a boundary; it does not mean an empty Plan.
func (p *PlanStore) Boundary(ctx context.Context, runID identity.RunID) (plan.Document, bool, error) {
	return scopedPair(ctx, p.db.read, func(ctx context.Context) (plan.Document, bool, error) {
		if err := validateResource("read Plan boundary", runID); err != nil {
			return plan.Document{}, false, err
		}
		var sourceJSON string
		var stepsJSON, digest sql.NullString
		err := p.db.connection(ctx).QueryRowContext(ctx, `SELECT doc.steps, b.document, b.source
			FROM plan_boundaries b LEFT JOIN plan_documents doc ON doc.digest = b.document WHERE b.run_id = ?`,
			runID).Scan(&stepsJSON, &digest, &sourceJSON)
		if errors.Is(err, sql.ErrNoRows) {
			return plan.Document{}, false, nil
		}
		if err != nil {
			return plan.Document{}, false, fmt.Errorf("sqlite: read Plan boundary for Run %q: %w", runID, err)
		}
		document, err := decodePlanBoundary(stepsJSON, digest, sourceJSON)
		return document, err == nil, err
	})
}

func (p *PlanStore) ListBoundaries(ctx context.Context, sessionID identity.SessionID) ([]sessions.RunPlanBoundary, error) {
	return scopedValue(ctx, p.db.read, func(ctx context.Context) ([]sessions.RunPlanBoundary, error) {
		if err := validateResource("read Plan boundaries", sessionID); err != nil {
			return nil, err
		}
		rows, err := p.db.connection(ctx).QueryContext(ctx, `SELECT b.run_id, doc.steps, b.document, b.source
			FROM plan_boundaries b JOIN runs r ON r.run_id = b.run_id
			LEFT JOIN plan_documents doc ON doc.digest = b.document
			WHERE r.session_id = ? ORDER BY b.run_id`, sessionID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: read Plan boundaries: %w", err)
		}
		defer func() { _ = rows.Close() }()
		var boundaries []sessions.RunPlanBoundary
		for rows.Next() {
			var id identity.RunID
			var source string
			var steps, digest sql.NullString
			if err := rows.Scan(&id, &steps, &digest, &source); err != nil {
				return nil, fmt.Errorf("sqlite: scan Plan boundary: %w", err)
			}
			document, err := decodePlanBoundary(steps, digest, source)
			if err != nil {
				return nil, fmt.Errorf("sqlite: read Plan boundary for Run %q: %w", id, err)
			}
			boundaries = append(boundaries, sessions.RunPlanBoundary{RunID: id, Document: document})
		}
		return boundaries, rows.Err()
	})
}

func decodePlanBoundary(stepsJSON, digest sql.NullString, sourceJSON string) (plan.Document, error) {
	if !digest.Valid {
		return plan.Document{Steps: []plan.Step{}}, nil
	}
	if !stepsJSON.Valid {
		return plan.Document{}, errors.New("sqlite: Plan boundary lost its immutable document")
	}
	steps, err := decodePlanSteps(stepsJSON.String, digest.String)
	if err != nil {
		return plan.Document{}, err
	}
	var source extension.ContributionSource
	if err := decodeStoredJSON([]byte(sourceJSON), &source); err != nil {
		return plan.Document{}, err
	}
	if source != (extension.ContributionSource{}) {
		if err := source.Validate(); err != nil {
			return plan.Document{}, err
		}
	}
	return plan.Document{Steps: steps, Source: source}, nil
}

func (p *PlanStore) RestoreBoundaries(ctx context.Context, boundaries []sessions.RunPlanBoundary) error {
	if len(boundaries) == 0 {
		return nil
	}
	return p.db.write(ctx, func(ctx context.Context) error {
		for _, boundary := range boundaries {
			digest, err := p.storeDocument(ctx, boundary.Document.Steps)
			if err != nil {
				return err
			}
			source, err := encodeStoredJSON(boundary.Document.Source)
			if err != nil {
				return fmt.Errorf("sqlite: encode Plan boundary source: %w", err)
			}
			if _, err := p.db.connection(ctx).ExecContext(ctx, `INSERT INTO plan_boundaries(run_id, document, source) VALUES (?, ?, ?)`,
				boundary.RunID, digest.String(), string(source)); err != nil {
				return fmt.Errorf("sqlite: store Plan boundary for Run %q: %w", boundary.RunID, err)
			}
		}
		return nil
	})
}
func (p *PlanStore) DeleteSession(ctx context.Context, sessionID identity.SessionID) error {
	return p.db.write(ctx, func(ctx context.Context) error {
		if err := validateResource("delete Session Plan", sessionID); err != nil {
			return err
		}
		if _, err := p.db.connection(ctx).ExecContext(ctx, `DELETE FROM session_plans WHERE session_id = ?`, sessionID); err != nil {
			return fmt.Errorf("sqlite: delete session Plan: %w", err)
		}
		return nil
	})
}
