package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/qimu"
)

type Attempt struct {
	ID              string               `json:"id"`
	Epoch           uint64               `json:"epoch"`
	Source          qimu.UnitRef         `json:"source"`
	Binding         qimu.BindingSnapshot `json:"binding"`
	Realm           string               `json:"realm"`
	Profile         qimu.Profile         `json:"profile"`
	Parent          string               `json:"parent,omitempty"`
	Scope           qimu.ScopeID         `json:"scope,omitempty"`
	ScopeGeneration uint64               `json:"scopeGeneration,omitempty"`
	Generation      uint64               `json:"generation,omitempty"`
}

func (a Attempt) Validate() error {
	if !validID(a.ID) || !validID(a.Realm) || a.Epoch == 0 || a.Profile.Validate() != nil {
		return errors.New("sqlite: invalid execution attempt")
	}
	if a.Parent != "" && !validID(a.Parent) {
		return errors.New("sqlite: invalid parent attempt")
	}
	if a.Profile == qimu.BrowserJS && (a.Scope.Validate() != nil || a.ScopeGeneration == 0 || a.Generation == 0) {
		return errors.New("sqlite: browser attempt requires its exact scope and generation")
	}
	if err := a.Source.Validate(); err != nil {
		return err
	}
	if err := a.Binding.Validate(); err != nil {
		return err
	}
	if !slices.ContainsFunc(a.Binding.Units, func(unit qimu.UnitBinding) bool { return unit.Source == a.Source }) {
		return errors.New("sqlite: attempt source is outside its binding")
	}
	return nil
}

// ExitProof is accepted only from a trusted driver after it verifies its actual
// owner's evidence. Timeout, disconnect, a new epoch or an old PID is not proof.
type ExitProof struct {
	AttemptID string `json:"attemptId"`
	Realm     string `json:"realm"`
	ExitID    string `json:"exitId"`
	Kind      string `json:"kind"`
	Evidence  []byte `json:"evidence"`
}

type AttemptRecord struct {
	Attempt  Attempt    `json:"attempt"`
	Released bool       `json:"released"`
	Proof    *ExitProof `json:"proof,omitempty"`
}

// RecordAttempt must complete before the runner or Realm can create anything.
// Reusing its identity never authorizes a second creation or a wider binding.
func (s *Store) RecordAttempt(ctx context.Context, attempt Attempt) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	if attempt.Epoch != s.epoch {
		return qimu.ErrFenced
	}
	encoded, err := json.Marshal(attempt)
	if err != nil {
		return err
	}
	if len(encoded) > s.maxRequestBytes {
		return errors.New("sqlite: attempt exceeds its byte limit")
	}
	return s.admission.Under(ctx, func() error {
		err := s.transaction(ctx, func(tx Executor) error {
			var previous []byte
			err := tx.QueryRowContext(ctx, `SELECT request FROM qimu_attempts WHERE id=?`, attempt.ID).Scan(&previous)
			if err == nil {
				if !bytes.Equal(previous, encoded) {
					return ErrConflict
				}
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if attempt.Parent != "" {
				var state string
				if err := tx.QueryRowContext(ctx, `SELECT state FROM qimu_attempts WHERE id=?`, attempt.Parent).Scan(&state); err != nil {
					return err
				}
				if state != "approved" {
					return qimu.ErrClosed
				}
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO qimu_attempts(id,epoch,request,request_digest,state) VALUES(?,?,?,?,'approved')`, attempt.ID, attempt.Epoch, encoded, qimu.Hash(encoded))
			return err
		})
		if err != nil && !errors.Is(err, ErrConflict) && !errors.Is(err, qimu.ErrClosed) {
			s.admission.RequireRecovery(err)
		}
		return err
	})
}

func (s *Store) Attempts(ctx context.Context) (records []AttemptRecord, err error) {
	err = s.transaction(ctx, func(tx Executor) error {
		rows, err := tx.QueryContext(ctx, `SELECT request,request_digest,state,proof FROM qimu_attempts ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var request, proof []byte
			var digest qimu.Digest
			var state string
			if err := rows.Scan(&request, &digest, &state, &proof); err != nil {
				return err
			}
			if qimu.Hash(request) != digest {
				return errors.New("sqlite: execution request integrity failure")
			}
			var record AttemptRecord
			if err := jsonv2.Unmarshal(request, &record.Attempt, jsonv2.RejectUnknownMembers(true)); err != nil {
				return err
			}
			if err := record.Attempt.Validate(); err != nil {
				return err
			}
			record.Released = state == "released"
			if record.Released {
				if err := jsonv2.Unmarshal(proof, &record.Proof, jsonv2.RejectUnknownMembers(true)); err != nil {
					return err
				}
				if record.Proof == nil || record.Proof.AttemptID != record.Attempt.ID || record.Proof.Realm != record.Attempt.Realm {
					return errors.New("sqlite: invalid stored exit proof")
				}
			}
			records = append(records, record)
		}
		return rows.Err()
	})
	return records, err
}

// ExecutionDependencies is deliberately separate from business Held queries.
// A Ready idle instance may be retired first; its protection remains here until
// actual exit is confirmed, including attempts with an unknown creation result.
func (s *Store) ExecutionDependencies(ctx context.Context) ([]qimu.Dependency, error) {
	records, err := s.Attempts(ctx)
	if err != nil {
		return nil, err
	}
	var dependencies []qimu.Dependency
	for _, record := range records {
		if !record.Released {
			dependencies = append(dependencies, record.Attempt.Binding.Dependencies()...)
		}
	}
	uses, err := s.Uses(ctx)
	if err != nil {
		return nil, err
	}
	for _, record := range uses {
		if !record.Released {
			dependencies = append(dependencies, record.Approval.Binding.Dependencies()...)
		}
	}
	return qimu.CompactDependencies(dependencies), nil
}

func (s *Store) ReleaseAttempt(ctx context.Context, id string, proof ExitProof) error {
	if !validID(id) || proof.AttemptID != id || !validID(proof.Realm) || !validID(proof.ExitID) || !validID(proof.Kind) || len(proof.Evidence) == 0 || len(proof.Evidence) > s.maxRequestBytes {
		return errors.New("sqlite: exact exit evidence is required")
	}
	encoded, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	return s.admission.Maintenance(ctx, func() error {
		err := s.transaction(ctx, func(tx Executor) error {
			var request, previous []byte
			var state string
			if err := tx.QueryRowContext(ctx, `SELECT request,state,proof FROM qimu_attempts WHERE id=?`, id).Scan(&request, &state, &previous); err != nil {
				return err
			}
			var attempt Attempt
			if err := jsonv2.Unmarshal(request, &attempt); err != nil {
				return err
			}
			if attempt.Realm != proof.Realm {
				return ErrConflict
			}
			if state == "released" {
				if !bytes.Equal(previous, encoded) {
					return ErrConflict
				}
				return nil
			}
			_, err := tx.ExecContext(ctx, `UPDATE qimu_attempts SET state='released',proof=? WHERE id=? AND state='approved'`, encoded, id)
			return err
		})
		if err != nil && !errors.Is(err, ErrConflict) && !errors.Is(err, sql.ErrNoRows) {
			s.admission.RequireRecovery(fmt.Errorf("persist exit proof: %w", err))
		}
		return err
	})
}

// Read invokes a trusted query in a fresh current transaction. It is useful for
// recovery and diagnostics; mutations must use an Owner's submission protocol.
func (s *Store) Read(ctx context.Context, query func(Executor) error) error {
	if query == nil {
		return errors.New("sqlite: query is required")
	}
	return s.transaction(ctx, query)
}
