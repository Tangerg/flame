package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

func (s *SessionStores) AdmitWorktreeFork(ctx context.Context, intent sessions.WorktreeForkIntent, plan sessions.ForkPlan) error {
	history, err := encodeForkHistory(plan)
	if err != nil {
		return fmt.Errorf("sqlite: freeze fork history: %w", err)
	}
	return s.tx(ctx, func(tx context.Context) error {
		if err := s.requireForkParent(tx, plan); err != nil {
			return err
		}
		if _, claimed, err := s.replay.Claim(tx, intent.Receipt); err != nil {
			return err
		} else if !claimed {
			return operation.ErrInProgress
		}
		tree := intent.Checkout
		row, err := s.db.connection(tx).ExecContext(tx, `INSERT INTO pending_worktree_forks(
			checkout_id, checkout_path, source_path, repository, base, patch, writer_id, receipt_key, history
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(checkout_id) DO NOTHING`,
			tree.ID, tree.Path, tree.Source, tree.Repository, tree.Head, tree.Patch, intent.WriteID, intent.Receipt.Key, history)
		if err != nil {
			return fmt.Errorf("sqlite: admit worktree fork: %w", err)
		}
		inserted, err := row.RowsAffected()
		if err != nil {
			return err
		}
		if inserted == 0 {
			return session.ErrRevisionConflict
		}
		return nil
	})
}

const pendingForksQuery = `SELECT f.checkout_id, f.checkout_path, f.source_path, f.repository, f.base, f.patch,
	f.writer_id, f.receipt_key, r.fingerprint
	FROM pending_worktree_forks f JOIN idempotency_records r ON r.key = f.receipt_key`

func (s *SessionStores) PendingWorktreeFork(ctx context.Context, receipt operation.ReplayClaim) (sessions.WorktreeForkIntent, error) {
	return scopedValue(ctx, s.readSnapshot, func(ctx context.Context) (sessions.WorktreeForkIntent, error) {
		row := s.db.connection(ctx).QueryRowContext(ctx, pendingForksQuery+` WHERE f.receipt_key = ? AND r.fingerprint = ?`, receipt.Key, receipt.Fingerprint)
		intent, err := scanWorktreeFork(row)
		if errors.Is(err, sql.ErrNoRows) {
			return sessions.WorktreeForkIntent{}, operation.ErrClaimLost
		}
		return intent, err
	})
}

func (s *SessionStores) PendingWorktreeForks(ctx context.Context) ([]sessions.WorktreeForkIntent, error) {
	return scopedValue(ctx, s.readSnapshot, func(ctx context.Context) ([]sessions.WorktreeForkIntent, error) {
		rows, err := s.db.connection(ctx).QueryContext(ctx, pendingForksQuery+` ORDER BY f.checkout_id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var pending []sessions.WorktreeForkIntent
		for rows.Next() {
			intent, err := scanWorktreeFork(rows)
			if err != nil {
				return nil, err
			}
			pending = append(pending, intent)
		}
		return pending, rows.Err()
	})
}

func scanWorktreeFork(row scanRow) (sessions.WorktreeForkIntent, error) {
	var intent sessions.WorktreeForkIntent
	tree := &intent.Checkout
	err := row.Scan(&tree.ID, &tree.Path, &tree.Source, &tree.Repository, &tree.Head, &tree.Patch,
		&intent.WriteID, &intent.Receipt.Key, &intent.Receipt.Fingerprint)
	return intent, err
}

func (s *SessionStores) WorktreeForkPending(ctx context.Context, checkoutID string) (bool, error) {
	return scopedValue(ctx, s.readSnapshot, func(ctx context.Context) (bool, error) {
		var pending bool
		err := s.db.connection(ctx).QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM pending_worktree_forks WHERE checkout_id = ?)`, checkoutID).Scan(&pending)
		return pending, err
	})
}

func (s *SessionStores) PublishWorktreeFork(ctx context.Context, intent sessions.WorktreeForkIntent, refusal []byte, encode func(session.Session) ([]byte, error)) (*sessions.ForkPublication, error) {
	var publication *sessions.ForkPublication
	err := s.tx(ctx, func(tx context.Context) error {
		// The draft is consumed in the transaction that makes the child and
		// its receipt visible. The source Session can change after admission.
		var encoded []byte
		err := s.db.connection(tx).QueryRowContext(tx,
			`DELETE FROM pending_worktree_forks WHERE receipt_key = ? AND writer_id = ? RETURNING history`,
			intent.Receipt.Key, intent.WriteID).Scan(&encoded)
		if err != nil {
			return err
		}
		payload := refusal
		if len(refusal) == 0 {
			var draft forkHistory
			if err := decodeStoredJSON(encoded, &draft); err != nil {
				return fmt.Errorf("sqlite: decode admitted fork history: %w", err)
			}
			history, plan, err := draft.restore(intent.Checkout.Path)
			if err != nil {
				return err
			}
			if err := s.insertFork(tx, history, draft.PlanBoundaries, plan); err != nil {
				return err
			}
			payload, err = encode(history.Session)
			if err != nil {
				return err
			}
			publication = &sessions.ForkPublication{History: history, HasPlan: plan != nil}
		}
		_, err = s.replay.Complete(tx, operation.ReplayRecord{
			Key: intent.Receipt.Key, Fingerprint: intent.Receipt.Fingerprint, Payload: payload,
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: publish worktree fork: %w", err)
	}
	return publication, nil
}
