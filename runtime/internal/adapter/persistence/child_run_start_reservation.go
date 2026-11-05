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

// ChildRunStartReservationRecord is SQLite's opaque technical record. Runtime
// application semantics live in the segment adapter; storage only enforces
// exact identity and compare-before-delete behavior.
type ChildRunStartReservationRecord struct {
	MemberID  string
	SessionID string
	Payload   []byte
	CreatedAt time.Time
}

func (c ChildRunStartReservationRecord) same(other ChildRunStartReservationRecord) bool {
	return c.MemberID == other.MemberID && c.SessionID == other.SessionID && c.CreatedAt.Equal(other.CreatedAt) && bytes.Equal(c.Payload, other.Payload)
}
func (c ChildRunStartReservationRecord) validate() error {
	if err := runs.ValidateMember(c.MemberID); err != nil {
		return fmt.Errorf("%w: %v", errInvalidChildRunStartReservation, err)
	}
	if err := identity.ValidateSession(c.SessionID); err != nil {
		return fmt.Errorf("%w: %v", errInvalidChildRunStartReservation, err)
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

// ChildRunStartReservationStore persists opaque child-start reservations and
// their conclusive state. Rows intentionally survive conclusion so exact
// publications remain idempotent without guessing from absence.
type ChildRunStartReservationStore struct{ db *database }

func newChildRunStartReservationStore(db *database) *ChildRunStartReservationStore {
	return &ChildRunStartReservationStore{db: db}
}

// Reserve inserts one record or accepts an exact replay. Reusing MemberID with
// different content is a durable identity conflict.
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

// Concluded confirms the exact durable outcome without advancing a reservation.
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

// Conclude advances an exact reservation from reserved to one outcome. changed
// is true only for the first conclusion. An exact replay of the same outcome
// returns false; absence, different content, or a different prior outcome is a
// conflict.
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

// DeleteSession removes every callback receipt owned by one Session. Child
// reservations are meaningful only while that Session's root execution tree is
// live; root terminalization, rollback/restore, and Session deletion call this
// inside their existing write-set so no concluded or reserve-before-abort row
// outlives its owner.
func (c *ChildRunStartReservationStore) DeleteSession(ctx context.Context, sessionID string) error {
	return c.db.write(ctx, func(ctx context.Context) error {
		if err := identity.ValidateSession(sessionID); err != nil {
			return fmt.Errorf("%w: %v", errInvalidChildRunStartReservation, err)
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
