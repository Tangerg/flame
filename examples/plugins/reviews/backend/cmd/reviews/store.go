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

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/frame/sqlite"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/scope/core/jsonschema"
)

// store owns this backend's business transitions and immutable invocation
// receipts. The review and its receipt commit together in the private database.
type store struct {
	db          *sqlite.Database
	inputSchema jsonschema.Schema
}

const maximumInvocationIDBytes = 256

func validateInvocationID(id string) error {
	if id == "" || len(id) > maximumInvocationIDBytes || !utf8.ValidString(id) {
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
	statements := []string{fmt.Sprintf(`CREATE TABLE IF NOT EXISTS reviews (
 id TEXT PRIMARY KEY CHECK (id = '%s'),
 title TEXT NOT NULL CHECK (length(title) > 0),
 status TEXT NOT NULL CHECK (status IN ('%s', '%s')),
 revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND %d)
)`, reviews.ExampleReviewID, reviews.Open, reviews.Resolved, int64(exactint.Maximum)), `CREATE TABLE IF NOT EXISTS invocations (
 id TEXT PRIMARY KEY,
 input_json TEXT NOT NULL CHECK(json_valid(input_json)),
 result BLOB NOT NULL
)`, fmt.Sprintf(`INSERT INTO reviews VALUES ('%s', 'Inspect the plugin ownership boundary', '%s', 1)`, reviews.ExampleReviewID, reviews.Open)}
	db, err := sqlite.OpenSchema(ctx, filepath.Join(directory, "reviews.sqlite"), statements)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("reviews: open private database: %w", err), db.Close())
	}
	return &store{db: db, inputSchema: inputSchema}, nil
}

func (s *store) Close() error { return s.db.Close() }

func (s *store) list(ctx context.Context) ([]reviews.Snapshot, error) {
	var snapshot reviews.Snapshot
	if err := s.db.Read(ctx, func(reader sqlite.Reader) error {
		return reader.QueryRowContext(ctx, `SELECT id, title, status, revision FROM reviews`).Scan(&snapshot.ID, &snapshot.Title, &snapshot.Status, &snapshot.Revision)
	}); err != nil {
		return nil, err
	}
	review, err := restoreReview(snapshot)
	if err != nil {
		return nil, err
	}
	return []reviews.Snapshot{review.snapshot()}, nil
}

// receipt reads the original owner result without entering Update. Method and
// input binding remain required even after the review has advanced again.
func (s *store) receipt(ctx context.Context, request pluginmcp.ReceiptRequest, updateMethod string) (pluginmcp.Receipt, error) {
	if err := validateInvocationID(string(request.ID)); err != nil {
		return pluginmcp.Receipt{}, err
	}
	if request.Method != updateMethod {
		return pluginmcp.Receipt{}, errors.New("reviews: receipt method conflicts")
	}
	var arguments, encoded []byte
	err := s.db.Read(ctx, func(reader sqlite.Reader) error {
		return reader.QueryRowContext(ctx, `SELECT input_json, result FROM invocations WHERE id = ?`, request.ID).Scan(&arguments, &encoded)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return pluginmcp.Receipt{}, nil
	}
	if err != nil {
		return pluginmcp.Receipt{}, err
	}
	var input reviews.Update
	if err := json.Unmarshal(arguments, &input, json.RejectUnknownMembers(true)); err != nil {
		return pluginmcp.Receipt{}, err
	}
	if err := s.inputSchema.Validate(arguments); err != nil {
		return pluginmcp.Receipt{}, err
	}
	admitted, err := framevalue.ParseJSON(arguments)
	if err != nil {
		return pluginmcp.Receipt{}, err
	}
	if admitted.Digest() != request.InputDigest {
		return pluginmcp.Receipt{}, errors.New("reviews: receipt input conflicts")
	}
	var result reviews.UpdateResult
	if err := json.Unmarshal(encoded, &result, json.RejectUnknownMembers(true)); err != nil {
		return pluginmcp.Receipt{}, err
	}
	if err := result.Validate(); err != nil {
		return pluginmcp.Receipt{}, err
	}
	reply, err := pluginmcp.EncodeReply(result, nil, nil)
	if err != nil {
		return pluginmcp.Receipt{}, err
	}
	return pluginmcp.Receipt{Found: true, Reply: reply.Bytes()}, nil
}

func (s *store) update(ctx context.Context, invocationID string, admitted framevalue.JSON) (_ reviews.UpdateResult, err error) {
	if err := validateInvocationID(invocationID); err != nil {
		return reviews.UpdateResult{}, err
	}
	if err := admitted.Validate(); err != nil {
		return reviews.UpdateResult{}, err
	}
	var input reviews.Update
	if err := json.Unmarshal(admitted.Bytes(), &input, json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
		return reviews.UpdateResult{}, err
	}
	if err := s.inputSchema.Validate(admitted.Bytes()); err != nil {
		return reviews.UpdateResult{}, err
	}
	arguments := admitted.String()
	var result reviews.UpdateResult
	err = sqlite.RunIndependentTx(ctx, s.db, func(ctx context.Context) error {
		connection := sqlite.Connection(ctx, s.db)
		var previousArguments string
		var encoded []byte
		err := connection.QueryRowContext(ctx, `SELECT input_json, result FROM invocations WHERE id = ?`, invocationID).Scan(&previousArguments, &encoded)
		if err == nil {
			if previousArguments != arguments {
				result = reviews.UpdateResult{Type: reviews.IdentityConflict}
				return nil
			}
			if err := json.Unmarshal(encoded, &result, json.RejectUnknownMembers(true)); err != nil {
				return err
			}
			return result.Validate()
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := s.db.Read(ctx, func(reader sqlite.Reader) error {
			var err error
			result, err = decideUpdate(ctx, reader, input)
			return err
		}); err != nil {
			return err
		}
		encoded, err = json.Marshal(result)
		if err != nil {
			return err
		}
		if result.Type == reviews.Updated {
			snapshot := result.Review
			if _, err := connection.ExecContext(ctx, `UPDATE reviews SET status = ?, revision = ? WHERE id = ?`, snapshot.Status, snapshot.Revision, snapshot.ID); err != nil {
				return err
			}
		}
		_, err = connection.ExecContext(ctx, `INSERT INTO invocations VALUES (?, ?, ?)`, invocationID, arguments, encoded)
		return err
	})
	if err != nil {
		return reviews.UpdateResult{}, errors.Join(pluginmcp.ErrOutcomeUnconfirmed, err)
	}
	return result, nil
}

func decideUpdate(ctx context.Context, reader sqlite.Reader, input reviews.Update) (reviews.UpdateResult, error) {
	if input.ValidateChange() != nil {
		return reviews.UpdateResult{Type: reviews.Invalid}, nil
	}
	var snapshot reviews.Snapshot
	err := reader.QueryRowContext(ctx, `SELECT id, title, status, revision FROM reviews WHERE id = ?`, input.ID).Scan(&snapshot.ID, &snapshot.Title, &snapshot.Status, &snapshot.Revision)
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
	return reviews.UpdateResult{Type: reviews.Updated, Review: &snapshot}, nil
}
