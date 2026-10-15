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

	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/jsonschema"

	_ "modernc.org/sqlite"
)

// Store owns this backend's business transitions and immutable invocation
// receipts. The review and its receipt commit together in the private database.
type Store struct {
	db          *sql.DB
	inputSchema jsonschema.Schema
}

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
	inputSchema, err := jsonschema.For[Update]()
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
);`, ExampleReviewID, Open, Resolved, MaxRevision), "invocations": `CREATE TABLE invocations (
 id TEXT PRIMARY KEY,
 input_json TEXT NOT NULL CHECK(json_valid(input_json)),
 result BLOB NOT NULL
);`}
	if err = admitSchema(ctx, db, schema); err != nil {
		return nil, fmt.Errorf("reviews: open private database: %w", err)
	}
	return &Store{db: db, inputSchema: inputSchema}, nil
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

// Receipt reads the original owner result without entering Update. Method and
// input binding remain required even after the review has advanced again.
func (s *Store) Receipt(ctx context.Context, request pluginsdk.ReceiptRequest, updateMethod string) (pluginsdk.Receipt, error) {
	if err := validateInvocationID(request.ID); err != nil {
		return pluginsdk.Receipt{}, err
	}
	if request.Method != updateMethod {
		return pluginsdk.Receipt{}, errors.New("reviews: receipt method conflicts")
	}
	var arguments, encoded []byte
	err := s.db.QueryRowContext(ctx, `SELECT input_json, result FROM invocations WHERE id = ?`, request.ID).Scan(&arguments, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return pluginsdk.Receipt{}, nil
	}
	if err != nil {
		return pluginsdk.Receipt{}, err
	}
	var input Update
	if err := json.Unmarshal(arguments, &input, json.RejectUnknownMembers(true)); err != nil {
		return pluginsdk.Receipt{}, err
	}
	if err := s.inputSchema.Validate(arguments); err != nil {
		return pluginsdk.Receipt{}, err
	}
	admitted, err := jsoninput.Parse(arguments)
	if err != nil {
		return pluginsdk.Receipt{}, err
	}
	if admitted.Digest() != request.InputDigest {
		return pluginsdk.Receipt{}, errors.New("reviews: receipt input conflicts")
	}
	var result UpdateResult
	if err := json.Unmarshal(encoded, &result, json.RejectUnknownMembers(true)); err != nil {
		return pluginsdk.Receipt{}, err
	}
	if err := result.Validate(); err != nil {
		return pluginsdk.Receipt{}, err
	}
	reply, err := pluginsdk.Reply(result, nil, nil)
	if err != nil {
		return pluginsdk.Receipt{}, err
	}
	data, err := json.Marshal(reply)
	if err != nil {
		return pluginsdk.Receipt{}, err
	}
	return pluginsdk.Receipt{Found: true, Reply: data}, nil
}

func (s *Store) Update(ctx context.Context, invocationID string, input Update, admitted jsoninput.Input) (_ UpdateResult, err error) {
	if err := validateInvocationID(invocationID); err != nil {
		return UpdateResult{}, err
	}
	if err := admitted.Validate(); err != nil {
		return UpdateResult{}, err
	}
	var received Update
	if err := json.Unmarshal(admitted.JSON(), &received, json.RejectUnknownMembers(true)); err != nil {
		return UpdateResult{}, err
	}
	if received != input {
		return UpdateResult{}, errors.New("reviews: update differs from admitted input")
	}
	if err := s.inputSchema.Validate(admitted.JSON()); err != nil {
		return UpdateResult{}, err
	}
	arguments := admitted.String()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpdateResult{}, err
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
	if input.Validate() != nil {
		return UpdateResult{Type: Invalid}, nil
	}
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
