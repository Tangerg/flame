package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

type WorkspaceMutationStore struct {
	db       *database
	receipts *idempotencyStore
}

func (s *WorkspaceMutationStore) Record(ctx context.Context, m sessions.WorkspaceMutation) (bool, error) {
	created, err := scopedValue(ctx, s.db.independent, func(tx context.Context) (bool, error) {
		if err := validateWorkspaceMutation(m); err != nil {
			return false, err
		}
		pending, err := s.ListPending(tx)
		if err != nil {
			return false, err
		}
		for _, existing := range pending {
			conflicts, err := workspaceMutationsOverlap(existing.SessionID, existing.CWD, m.SessionID, m.CWD)
			if err != nil {
				return false, err
			}
			if !conflicts {
				continue
			}
			if existing == m {
				return false, nil
			}
			return false, sessions.ErrWorkspaceMutationPending
		}
		if m.Receipt.Key != "" {
			_, claimed, err := s.receipts.Claim(tx, operation.ReplayClaim{
				Key: m.Receipt.Key, Fingerprint: m.Receipt.Fingerprint,
			})
			if err != nil {
				return false, err
			}
			if !claimed {
				return false, operation.ErrInProgress
			}
		}
		result, err := s.db.connection(tx).ExecContext(tx, `INSERT INTO pending_workspace_mutations(session_id, cwd, to_run_id, restore_history, receipt_key)
			 SELECT sessions.id, sessions.workspace_path, runs.run_id, ?, NULLIF(?, '')
			   FROM sessions JOIN runs ON runs.run_id = ? AND runs.session_id = sessions.id
			  WHERE sessions.id = ? AND sessions.workspace_path = ?`, m.RestoreHistory, m.Receipt.Key, m.ToRunID, m.SessionID, m.CWD)
		if err != nil {
			return false, err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if changed != 1 {
			return false, errors.New("workspace mutation has no matching Run boundary")
		}
		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("sqlite: record workspace mutation: %w", err)
	}
	return created, nil
}

func (s *WorkspaceMutationStore) Complete(ctx context.Context, m sessions.WorkspaceMutation, write func(context.Context) ([]byte, error)) error {
	if err := validateWorkspaceMutation(m); err != nil {
		return err
	}
	err := s.db.independent(ctx, func(tx context.Context) error {
		result, err := s.db.connection(tx).ExecContext(tx, `DELETE FROM pending_workspace_mutations
		  WHERE session_id = ? AND cwd = ? AND to_run_id = ? AND restore_history = ?
		    AND receipt_key IS NULLIF(?, '')
		    AND (receipt_key IS NULL OR EXISTS (
		        SELECT 1 FROM idempotency_records WHERE key = receipt_key AND kind = ? AND fingerprint = ?
		    ))`, m.SessionID, m.CWD, m.ToRunID, m.RestoreHistory, m.Receipt.Key, idempotencyPendingResponse, m.Receipt.Fingerprint)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return errors.New("workspace mutation lost its original intent")
		}
		payload, err := write(tx)
		if err != nil || m.Receipt.Key == "" {
			return err
		}
		if len(payload) == 0 {
			return errors.New("keyed workspace mutation omitted its outcome")
		}
		return s.receipts.Complete(tx, operation.ReplayRecord{
			Key: m.Receipt.Key, Fingerprint: m.Receipt.Fingerprint, Payload: payload,
		})
	})
	if err != nil {
		return fmt.Errorf("sqlite: complete workspace mutation: %w", err)
	}
	return nil
}

func (s *WorkspaceMutationStore) SessionPending(ctx context.Context, sessionID string) (bool, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) (bool, error) {
		var pending bool
		err := s.db.connection(ctx).QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM pending_workspace_mutations WHERE session_id = ?)`, sessionID).Scan(&pending)
		if err != nil {
			return false, fmt.Errorf("sqlite: read pending workspace mutation: %w", err)
		}
		return pending, nil
	})
}

func (s *WorkspaceMutationStore) ListPending(ctx context.Context) ([]sessions.WorkspaceMutation, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) ([]sessions.WorkspaceMutation, error) {
		rows, err := s.db.connection(ctx).QueryContext(ctx, `SELECT m.session_id, m.cwd, m.to_run_id, m.restore_history,
			   COALESCE(m.receipt_key, ''), COALESCE(r.fingerprint, '')
			  FROM pending_workspace_mutations m
			  LEFT JOIN idempotency_records r ON r.key = m.receipt_key AND r.kind = ?
			  ORDER BY m.session_id`, idempotencyPendingResponse)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list pending workspace mutations: %w", err)
		}
		defer func() { _ = rows.Close() }()
		var pending []sessions.WorkspaceMutation
		for rows.Next() {
			var m sessions.WorkspaceMutation
			if err := rows.Scan(&m.SessionID, &m.CWD, &m.ToRunID, &m.RestoreHistory, &m.Receipt.Key, &m.Receipt.Fingerprint); err != nil {
				return nil, fmt.Errorf("sqlite: scan pending workspace mutation: %w", err)
			}
			if err := validateWorkspaceMutation(m); err != nil {
				return nil, err
			}
			for _, existing := range pending {
				conflicts, err := workspaceMutationsOverlap(existing.SessionID, existing.CWD, m.SessionID, m.CWD)
				if err != nil {
					return nil, err
				}
				if conflicts {
					return nil, errors.New("sqlite: retained workspace mutations have overlapping ownership")
				}
			}
			pending = append(pending, m)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: iterate pending workspace mutations: %w", err)
		}
		return pending, nil
	})
}

func validateWorkspaceMutation(m sessions.WorkspaceMutation) error {
	if err := validateSessionResource("workspace mutation", m.SessionID); err != nil {
		return err
	}
	if err := validateRunResource("workspace mutation", m.ToRunID); err != nil {
		return err
	}
	if m.CWD == "" {
		return errors.New("sqlite: workspace mutation has no working tree")
	}
	if (m.Receipt.Key == "") != (m.Receipt.Fingerprint == "") {
		return errors.New("sqlite: workspace mutation has an incomplete receipt reservation")
	}
	return nil
}

func workspaceMutationsOverlap(leftSessionID, leftPath, rightSessionID, rightPath string) (bool, error) {
	if leftSessionID == rightSessionID {
		return true, nil
	}
	if leftPath == "" || rightPath == "" {
		return false, nil
	}
	return filesystem.PathsOverlap(leftPath, rightPath)
}
