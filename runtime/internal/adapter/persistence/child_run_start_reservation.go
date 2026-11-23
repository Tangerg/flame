package persistence

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

var (
	errInvalidChildRunStartReservation  = errors.New("sqlite: invalid child Run start reservation")
	errChildRunStartReservationConflict = errors.New("sqlite: child Run start reservation conflict")
)

const reservedState = "reserved"

func parseStoredChildRunStart(raw string) (outcome runs.ChildRunStartOutcome, concluded bool, err error) {
	if raw == reservedState {
		return "", false, nil
	}
	outcome = runs.ChildRunStartOutcome(raw)
	if !outcome.Valid() {
		return "", false, errChildRunStartReservationConflict
	}
	return outcome, true, nil
}

type ChildRunStartReservationRecord struct {
	MemberID  string
	SessionID identity.SessionID
	Payload   []byte
	CreatedAt time.Time
}

func (c ChildRunStartReservationRecord) same(other ChildRunStartReservationRecord) bool {
	return c.MemberID == other.MemberID && c.SessionID == other.SessionID && c.CreatedAt.Equal(other.CreatedAt) && bytes.Equal(c.Payload, other.Payload)
}

func (c ChildRunStartReservationRecord) validate() error {
	if err := runs.ValidateMember(c.MemberID); err != nil {
		return fmt.Errorf("%w: %w", errInvalidChildRunStartReservation, err)
	}
	if err := c.SessionID.Validate(); err != nil {
		return fmt.Errorf("%w: %w", errInvalidChildRunStartReservation, err)
	}
	if len(c.Payload) == 0 {
		return fmt.Errorf("%w: payload", errInvalidChildRunStartReservation)
	}
	if c.CreatedAt.IsZero() {
		return fmt.Errorf("%w: creation time", errInvalidChildRunStartReservation)
	}
	if err := instant.Validate(c.CreatedAt); err != nil {
		return fmt.Errorf("%w: %w", errInvalidChildRunStartReservation, err)
	}
	return nil
}

// Rows survive conclusion so publications stay idempotent without guessing from
// absence.
type ChildRunStartReservationStore struct{ db *database }

func newChildRunStartReservationStore(db *database) *ChildRunStartReservationStore {
	return &ChildRunStartReservationStore{db: db}
}

func (c *ChildRunStartReservationStore) Reserve(ctx context.Context, record ChildRunStartReservationRecord) error {
	return c.db.write(ctx, func(ctx context.Context) error {
		if err := record.validate(); err != nil {
			return err
		}
		result, err := c.db.connection(ctx).ExecContext(ctx, `
		INSERT INTO child_run_start_reservations(member_id, session_id, payload, created_at, state)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(member_id) DO NOTHING`, record.MemberID, record.SessionID, record.Payload, record.CreatedAt.UTC().UnixNano(), reservedState)
		if err != nil {
			return fmt.Errorf("sqlite: reserve child Run start: %w", err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect child Run start reservation: %w", err)
		}
		if inserted == 1 {
			return nil
		}
		existing, _, _, err := c.load(ctx, record.MemberID)
		if err != nil {
			return err
		}
		if !existing.same(record) {
			return errChildRunStartReservationConflict
		}
		return nil
	})
}

func (c *ChildRunStartReservationStore) Concluded(ctx context.Context, record ChildRunStartReservationRecord, outcome runs.ChildRunStartOutcome) (bool, error) {
	if err := record.validate(); err != nil {
		return false, err
	}
	if !outcome.Valid() {
		return false, errors.New("sqlite: child Run start outcome is invalid")
	}
	existing, stored, concluded, err := c.load(ctx, record.MemberID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !existing.same(record) {
		return false, errChildRunStartReservationConflict
	}
	return concluded && stored == outcome, nil
}

func (c *ChildRunStartReservationStore) Conclude(ctx context.Context, record ChildRunStartReservationRecord, outcome runs.ChildRunStartOutcome) (bool, error) {
	return scopedValue(ctx, c.db.write, func(ctx context.Context) (bool, error) {
		if err := record.validate(); err != nil {
			return false, err
		}
		if !outcome.Valid() {
			return false, errors.New("sqlite: child Run start outcome is invalid")
		}
		existing, stored, concluded, err := c.load(ctx, record.MemberID)
		if errors.Is(err, sql.ErrNoRows) {
			return false, errChildRunStartReservationConflict
		}
		if err != nil {
			return false, err
		}
		if !existing.same(record) {
			return false, errChildRunStartReservationConflict
		}
		if concluded {
			if stored == outcome {
				return false, nil
			}
			return false, errChildRunStartReservationConflict
		}
		result, err := c.db.connection(ctx).ExecContext(ctx, `
		UPDATE child_run_start_reservations
		SET state = ?
		WHERE member_id = ? AND state = ?`, outcome.String(), record.MemberID, reservedState)
		if err != nil {
			return false, fmt.Errorf("sqlite: conclude child Run start reservation: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return false, fmt.Errorf("sqlite: inspect child Run start conclusion: %w", err)
		}
		if changed != 1 {
			return false, errChildRunStartReservationConflict
		}
		return true, nil
	})
}

func (c *ChildRunStartReservationStore) DeleteSession(ctx context.Context, sessionID identity.SessionID) error {
	return c.db.write(ctx, func(ctx context.Context) error {
		if err := sessionID.Validate(); err != nil {
			return fmt.Errorf("%w: %w", errInvalidChildRunStartReservation, err)
		}
		if _, err := c.db.connection(ctx).ExecContext(ctx, `DELETE FROM child_run_start_reservations WHERE session_id = ?`, sessionID); err != nil {
			return fmt.Errorf("sqlite: delete Session child Run start reservations: %w", err)
		}
		return nil
	})
}

func (c *ChildRunStartReservationStore) load(ctx context.Context, memberID string) (ChildRunStartReservationRecord, runs.ChildRunStartOutcome, bool, error) {
	return scopedTriple(ctx, c.db.read, func(ctx context.Context) (ChildRunStartReservationRecord, runs.ChildRunStartOutcome, bool, error) {
		var record ChildRunStartReservationRecord
		var createdAt int64
		var state string
		err := c.db.connection(ctx).QueryRowContext(ctx, `
		SELECT member_id, session_id, payload, created_at, state
		FROM child_run_start_reservations
		WHERE member_id = ?`, memberID).Scan(&record.MemberID, &record.SessionID, &record.Payload, &createdAt, &state)
		if err != nil {
			return ChildRunStartReservationRecord{}, "", false, err
		}
		record.CreatedAt = time.Unix(0, createdAt).UTC()
		if err := record.validate(); err != nil {
			return ChildRunStartReservationRecord{}, "", false, err
		}
		outcome, concluded, err := parseStoredChildRunStart(state)
		if err != nil {
			return ChildRunStartReservationRecord{}, "", false, err
		}
		return record, outcome, concluded, nil
	})
}
