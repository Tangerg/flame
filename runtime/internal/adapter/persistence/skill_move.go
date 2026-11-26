package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skillauthoring"
)

type SkillMoveStore struct {
	db       *database
	receipts *idempotencyStore
	encode   func(error) ([]byte, error)
}

func (b *Bundle) SkillMoves(encode func(error) ([]byte, error)) *SkillMoveStore {
	return &SkillMoveStore{db: b.db, receipts: b.Idempotency, encode: encode}
}

func (s *SkillMoveStore) Prepare(ctx context.Context, move skillauthoring.LifecycleMove) error {
	return s.db.independent(ctx, func(tx context.Context) error {
		if move.Receipt.Key != "" {
			_, claimed, err := s.receipts.Claim(tx, move.Receipt)
			if err != nil {
				return err
			}
			if !claimed {
				return operation.ErrInProgress
			}
		}
		_, err := s.db.connection(tx).ExecContext(tx, `INSERT INTO pending_skill_move(id, name, destination, directory_id, document_revision, receipt_key)
			VALUES(1,?,?,?,?,NULLIF(?,''))`, move.Name, move.Destination, move.DirectoryID, move.DocumentRevision, move.Receipt.Key)
		return err
	})
}

func (s *SkillMoveStore) Pending(ctx context.Context) (skillauthoring.LifecycleMove, bool, error) {
	return scopedPair(ctx, s.db.read, func(tx context.Context) (skillauthoring.LifecycleMove, bool, error) {
		var move skillauthoring.LifecycleMove
		err := s.db.connection(tx).QueryRowContext(tx, `SELECT move.name, move.destination, move.directory_id, move.document_revision,
			COALESCE(move.receipt_key,''), COALESCE(receipt.fingerprint,'')
			FROM pending_skill_move move LEFT JOIN idempotency_records receipt
			ON receipt.key=move.receipt_key AND receipt.kind=? WHERE move.id=1`, idempotencyPendingResponse).
			Scan(&move.Name, &move.Destination, &move.DirectoryID, &move.DocumentRevision, &move.Receipt.Key, &move.Receipt.Fingerprint)
		if errors.Is(err, sql.ErrNoRows) {
			return skillauthoring.LifecycleMove{}, false, nil
		}
		if err != nil {
			return skillauthoring.LifecycleMove{}, false, err
		}
		if err := errors.Join(skills.ValidateName(move.Name), move.Destination.Validate()); err != nil {
			return skillauthoring.LifecycleMove{}, false, err
		}
		if move.Receipt.Key != "" && move.Receipt.Fingerprint == "" {
			return skillauthoring.LifecycleMove{}, false, operation.ErrClaimLost
		}
		return move, true, nil
	})
}

func (s *SkillMoveStore) Complete(ctx context.Context, move skillauthoring.LifecycleMove, cause error) error {
	return s.db.independent(ctx, func(tx context.Context) error {
		result, err := s.db.connection(tx).ExecContext(tx, `DELETE FROM pending_skill_move
			WHERE id=1 AND name=? AND destination=? AND directory_id=? AND document_revision=? AND receipt_key IS NULLIF(?, '')`,
			move.Name, move.Destination, move.DirectoryID, move.DocumentRevision, move.Receipt.Key)
		if err != nil {
			return err
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if deleted != 1 {
			return operation.ErrClaimLost
		}
		if move.Receipt.Key == "" {
			return nil
		}
		payload, err := s.encode(cause)
		if err != nil {
			return err
		}
		_, err = s.receipts.Complete(tx, operation.ReplayRecord{Key: move.Receipt.Key, Fingerprint: move.Receipt.Fingerprint, Payload: payload})
		return err
	})
}
