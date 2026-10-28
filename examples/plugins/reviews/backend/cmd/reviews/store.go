package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/scope/core/jsonschema"

	_ "modernc.org/sqlite"
)

// store owns this backend's business transitions and immutable invocation
// receipts. The review and its receipt commit together in the private database.
type store struct {
	db          *sql.DB
	inputSchema jsonschema.Schema
}

func validateInvocationID(id string) error {
	if id == "" || len(id) > 256 || !utf8.ValidString(id) {
		return errors.New("reviews: bounded invocation identity is required")
	}
	return nil
}

func openStore(ctx context.Context, directory string) (_ *store, err error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("reviews: data directory must be absolute")
	}
	inputSchema, err := jsonschema.For[reviews.Update]()
	if err != nil {
		return nil, fmt.Errorf("reviews: input schema: %w", err)
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
	if _, err = db.ExecContext(ctx, `PRAGMA busy_timeout = 5000; PRAGMA synchronous = FULL;`); err != nil {
		return nil, err
	}
	schema := map[string]string{"reviews": fmt.Sprintf(`CREATE TABLE reviews (
 id TEXT PRIMARY KEY CHECK (id = '%s'),
 title TEXT NOT NULL CHECK (length(title) > 0),
 status TEXT NOT NULL CHECK (status IN ('%s', '%s')),
 revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND %d)
);`, reviews.ExampleReviewID, reviews.Open, reviews.Resolved, int64(exactint.Maximum)), "invocations": `CREATE TABLE invocations (
 id TEXT PRIMARY KEY,
 input_json TEXT NOT NULL CHECK(json_valid(input_json)),
 result BLOB NOT NULL
);`}
	if err = admitSchema(ctx, db, schema); err != nil {
		return nil, fmt.Errorf("reviews: open private database: %w", err)
	}
	return &store{db: db, inputSchema: inputSchema}, nil
}

func (s *store) Close() error { return s.db.Close() }

func (s *store) list(ctx context.Context) ([]reviews.Snapshot, error) {
	var snapshot reviews.Snapshot
	if err := s.db.QueryRowContext(ctx, `SELECT id, title, status, revision FROM reviews`).Scan(&snapshot.ID, &snapshot.Title, &snapshot.Status, &snapshot.Revision); err != nil {
		return nil, err
	}
	review, err := restoreReview(snapshot)
	if err != nil {
		return nil, err
	}
	return []reviews.Snapshot{review.snapshot()}, nil
}

// Receipt reads the original owner result without entering Update. Method and
// input binding remain required even after the review has advanced again.
func (s *store) receipt(ctx context.Context, request qimumcp.ReceiptRequest, updateMethod string) (qimumcp.Receipt, error) {
	if err := validateInvocationID(request.ID); err != nil {
		return qimumcp.Receipt{}, err
	}
	if request.Method != updateMethod {
		return qimumcp.Receipt{}, errors.New("reviews: receipt method conflicts")
	}
	var arguments, encoded []byte
	err := s.db.QueryRowContext(ctx, `SELECT input_json, result FROM invocations WHERE id = ?`, request.ID).Scan(&arguments, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return qimumcp.Receipt{}, nil
	}
	if err != nil {
		return qimumcp.Receipt{}, err
	}
	var input reviews.Update
	if err := json.Unmarshal(arguments, &input, json.RejectUnknownMembers(true)); err != nil {
		return qimumcp.Receipt{}, err
	}
	if err := s.inputSchema.Validate(arguments); err != nil {
		return qimumcp.Receipt{}, err
	}
	admitted, err := jsonvalue.Parse(arguments)
	if err != nil {
		return qimumcp.Receipt{}, err
	}
	if admitted.Digest() != request.InputDigest {
		return qimumcp.Receipt{}, errors.New("reviews: receipt input conflicts")
	}
	var result reviews.UpdateResult
	if err := json.Unmarshal(encoded, &result, json.RejectUnknownMembers(true)); err != nil {
		return qimumcp.Receipt{}, err
	}
	if err := result.Validate(); err != nil {
		return qimumcp.Receipt{}, err
	}
	reply, err := qimumcp.EncodeReply(result, nil, nil)
	if err != nil {
		return qimumcp.Receipt{}, err
	}
	return qimumcp.Receipt{Found: true, Reply: reply.JSON()}, nil
}

func (s *store) update(ctx context.Context, invocationID string, admitted jsonvalue.Value) (_ reviews.UpdateResult, err error) {
	if err := validateInvocationID(invocationID); err != nil {
		return reviews.UpdateResult{}, err
	}
	if err := admitted.Validate(); err != nil {
		return reviews.UpdateResult{}, err
	}
	var input reviews.Update
	if err := json.Unmarshal(admitted.JSON(), &input, json.RejectUnknownMembers(true), jsonvalue.Numbers()); err != nil {
		return reviews.UpdateResult{}, err
	}
	if err := s.inputSchema.Validate(admitted.JSON()); err != nil {
		return reviews.UpdateResult{}, err
	}
	arguments := admitted.String()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return reviews.UpdateResult{}, err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, rollbackErr)
		}
	}()
	var previousArguments string
	var encoded []byte
	err = tx.QueryRowContext(ctx, `SELECT input_json, result FROM invocations WHERE id = ?`, invocationID).Scan(&previousArguments, &encoded)
	if err == nil {
		if previousArguments != arguments {
			return reviews.UpdateResult{Type: reviews.IdentityConflict}, nil
		}
		var result reviews.UpdateResult
		if err := json.Unmarshal(encoded, &result, json.RejectUnknownMembers(true)); err != nil {
			return reviews.UpdateResult{}, err
		}
		if err := result.Validate(); err != nil {
			return reviews.UpdateResult{}, err
		}
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return reviews.UpdateResult{}, err
	}
	result, err := applyUpdate(ctx, tx, input)
	if err != nil {
		return reviews.UpdateResult{}, err
	}
	encoded, err = json.Marshal(result)
	if err != nil {
		return reviews.UpdateResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO invocations VALUES (?, ?, ?)`, invocationID, arguments, encoded); err != nil {
		return reviews.UpdateResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return reviews.UpdateResult{}, fmt.Errorf("reviews: commit update: %w", err)
	}
	return result, nil
}

func applyUpdate(ctx context.Context, tx *sql.Tx, input reviews.Update) (reviews.UpdateResult, error) {
	if input.Validate() != nil {
		return reviews.UpdateResult{Type: reviews.Invalid}, nil
	}
	var snapshot reviews.Snapshot
	err := tx.QueryRowContext(ctx, `SELECT id, title, status, revision FROM reviews WHERE id = ?`, input.ID).Scan(&snapshot.ID, &snapshot.Title, &snapshot.Status, &snapshot.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return reviews.UpdateResult{Type: reviews.NotFound}, nil
	}
	if err != nil {
		return reviews.UpdateResult{}, err
	}
	review, err := restoreReview(snapshot)
	if err != nil {
		return reviews.UpdateResult{}, err
	}
	next, err := review.change(input.ExpectedRevision, input.Status)
	if errors.Is(err, errConflict) {
		return reviews.UpdateResult{Type: reviews.Conflict, Review: &snapshot}, nil
	}
	if errors.Is(err, errRevisionExhausted) {
		return reviews.UpdateResult{Type: reviews.RevisionExhausted}, nil
	}
	if err != nil {
		return reviews.UpdateResult{}, err
	}
	snapshot = next.snapshot()
	if _, err := tx.ExecContext(ctx, `UPDATE reviews SET status = ?, revision = ? WHERE id = ?`, snapshot.Status, snapshot.Revision, snapshot.ID); err != nil {
		return reviews.UpdateResult{}, err
	}
	return reviews.UpdateResult{Type: reviews.Updated, Review: &snapshot}, nil
}
