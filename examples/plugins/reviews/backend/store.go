package reviews

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

// Store owns this backend's business transitions and immutable invocation
// receipts. The review and its receipt commit together in the private database.
type Store struct{ db *sql.DB }

const MaxReceipts = 4096

func validateInvocationID(id string) error {
	if id == "" || len(id) > 256 || !utf8.ValidString(id) {
		return errors.New("reviews: bounded invocation identity is required")
	}
	return nil
}

func OpenStore(ctx context.Context, directory string) (_ *Store, err error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("reviews: data directory must be absolute")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(directory, "reviews.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	defer func() {
		if err != nil {
			err = errors.Join(err, db.Close())
		}
	}()
	_, err = db.ExecContext(ctx, fmt.Sprintf(`
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = FULL;
CREATE TABLE IF NOT EXISTS reviews (
 id TEXT PRIMARY KEY CHECK (id = '%s'),
 title TEXT NOT NULL CHECK (length(title) > 0),
 status TEXT NOT NULL CHECK (status IN ('%s', '%s')),
 revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND %d)
);
CREATE TABLE IF NOT EXISTS invocations (
 id TEXT PRIMARY KEY,
 arguments BLOB NOT NULL,
 result BLOB NOT NULL
);
INSERT OR IGNORE INTO reviews VALUES ('%s', 'Inspect the plugin ownership boundary', '%s', 1);
`, ExampleReviewID, Open, Resolved, MaxRevision, ExampleReviewID, Open))
	if err != nil {
		return nil, fmt.Errorf("reviews: open private database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) List(ctx context.Context) ([]Snapshot, error) {
	var snapshot Snapshot
	if err := s.db.QueryRowContext(ctx, `SELECT id, title, status, revision FROM reviews`).Scan(&snapshot.ID, &snapshot.Title, &snapshot.Status, &snapshot.Revision); err != nil {
		return nil, err
	}
	review, err := Restore(snapshot)
	if err != nil {
		return nil, err
	}
	return []Snapshot{review.Snapshot()}, nil
}

func (s *Store) Update(ctx context.Context, invocationID string, input Update) (_ UpdateResult, err error) {
	if err := validateInvocationID(invocationID); err != nil {
		return UpdateResult{}, err
	}
	if input.Validate() != nil {
		return UpdateResult{Type: Invalid}, nil
	}
	arguments, err := json.Marshal(input)
	if err != nil {
		return UpdateResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpdateResult{}, err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, rollbackErr)
		}
	}()
	var previousArguments, encoded []byte
	err = tx.QueryRowContext(ctx, `SELECT arguments, result FROM invocations WHERE id = ?`, invocationID).Scan(&previousArguments, &encoded)
	if err == nil {
		if string(previousArguments) != string(arguments) {
			return UpdateResult{Type: IdentityConflict}, nil
		}
		var result UpdateResult
		if err := json.Unmarshal(encoded, &result, json.RejectUnknownMembers(true)); err != nil {
			return UpdateResult{}, err
		}
		if err := result.Validate(); err != nil {
			return UpdateResult{}, err
		}
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return UpdateResult{}, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM invocations`).Scan(&count); err != nil {
		return UpdateResult{}, err
	}
	if count >= MaxReceipts {
		return UpdateResult{Type: Capacity}, nil
	}
	result, err := applyUpdate(ctx, tx, input)
	if err != nil {
		return UpdateResult{}, err
	}
	encoded, err = json.Marshal(result)
	if err != nil {
		return UpdateResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO invocations VALUES (?, ?, ?)`, invocationID, arguments, encoded); err != nil {
		return UpdateResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return UpdateResult{}, fmt.Errorf("reviews: commit update: %w", err)
	}
	return result, nil
}

func applyUpdate(ctx context.Context, tx *sql.Tx, input Update) (UpdateResult, error) {
	var snapshot Snapshot
	err := tx.QueryRowContext(ctx, `SELECT id, title, status, revision FROM reviews WHERE id = ?`, input.ID).Scan(&snapshot.ID, &snapshot.Title, &snapshot.Status, &snapshot.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return UpdateResult{Type: NotFound}, nil
	}
	if err != nil {
		return UpdateResult{}, err
	}
	review, err := Restore(snapshot)
	if err != nil {
		return UpdateResult{}, err
	}
	next, err := review.Change(input.ExpectedRevision, input.Status)
	if errors.Is(err, ErrConflict) {
		return UpdateResult{Type: Conflict, Review: &snapshot}, nil
	}
	if errors.Is(err, ErrRevisionExhausted) {
		return UpdateResult{Type: RevisionExhausted}, nil
	}
	if err != nil {
		return UpdateResult{}, err
	}
	snapshot = next.Snapshot()
	if _, err := tx.ExecContext(ctx, `UPDATE reviews SET status = ?, revision = ? WHERE id = ?`, snapshot.Status, snapshot.Revision, snapshot.ID); err != nil {
		return UpdateResult{}, err
	}
	return UpdateResult{Type: Updated, Review: &snapshot}, nil
}
