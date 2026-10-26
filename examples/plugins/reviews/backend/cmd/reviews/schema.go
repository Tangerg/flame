package main

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"slices"
	"strings"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"
)

// The private format changed from a re-encoded DTO to the admitted request.
// Admission precedes all writes; opening an old directory never converts it.
func admitSchema(ctx context.Context, db *sql.DB, schema map[string]string) (err error) {
	rows, err := db.QueryContext(ctx, `SELECT name,sql FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return err
	}
	found := map[string]string{}
	for rows.Next() {
		var name, statement string
		if err := rows.Scan(&name, &statement); err != nil {
			return errors.Join(err, rows.Close())
		}
		found[name] = statement
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	if len(found) != 0 {
		if len(found) != len(schema) {
			return errors.New("incompatible private schema; retain this directory and use a fresh directory")
		}
		// sqlite_schema omits the trailing statement delimiter.
		for name, expected := range schema {
			if found[name] != strings.TrimSuffix(expected, ";") {
				return errors.New("incompatible private schema; retain this directory and use a fresh directory")
			}
		}
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, rollbackErr)
		}
	}()
	for _, name := range slices.Sorted(maps.Keys(schema)) {
		if _, err := tx.ExecContext(ctx, schema[name]); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO reviews VALUES (?, 'Inspect the plugin ownership boundary', ?, 1)`, reviews.ExampleReviewID, reviews.Open); err != nil {
		return err
	}
	return tx.Commit()
}
