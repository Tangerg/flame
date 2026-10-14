// Package sqlite owns strict SQLite schema admission and transaction lifetimes.
// Business schemas and records remain with their calling owners.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	_ "modernc.org/sqlite"
)

// OpenSchema admits an exact owner-supplied schema in its private database.
func OpenSchema(ctx context.Context, path string, statements []string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)",
		url.PathEscape(filepath.ToSlash(path)),
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}
	// modernc.org/sqlite serializes writes internally; one connection
	// is sufficient and avoids "database is locked" surprises under
	// concurrent transactions.
	db.SetMaxOpenConns(1)

	if err := InstallSchema(ctx, db, statements); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}

func InstallSchema(ctx context.Context, db *sql.DB, stmts []string) error {
	err := RunIndependentTx(ctx, db, func(ctx context.Context) error {
		tx := Connection(ctx, db)
		if err := requireCurrentSchema(ctx, tx, stmts); err != nil {
			return err
		}
		for _, stmt := range stmts {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("sqlite: install current schema: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("sqlite: install current schema: %w", err)
	}
	return nil
}

// requireCurrentSchema admits a data directory that is empty or exactly
// current: every table, index and trigger the current statements define, with
// their definitions, and nothing else. The statements would only partly apply
// to any other directory, so it is refused before one of them runs; there is
// no migration path, and a former shape is never repaired in place.
func requireCurrentSchema(ctx context.Context, tx Executor, stmts []string) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT name, sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite\_%' ESCAPE '\'`)
	if err != nil {
		return fmt.Errorf("sqlite: inspect schema: %w", err)
	}
	stored := map[string]string{}
	for rows.Next() {
		var name, definition string
		if err := rows.Scan(&name, &definition); err != nil {
			_ = rows.Close()
			return fmt.Errorf("sqlite: inspect schema: %w", err)
		}
		stored[name] = definition
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("sqlite: inspect schema: %w", err)
	}
	if len(stored) == 0 {
		return nil
	}
	for _, stmt := range stmts {
		if !strings.HasPrefix(stmt, "CREATE ") {
			continue
		}
		name, definition, err := schemaObject(stmt)
		if err != nil {
			return err
		}
		if stored[name] != definition {
			return fmt.Errorf("sqlite: incompatible %s schema; open a fresh data directory", name)
		}
		delete(stored, name)
	}
	for _, name := range slices.Sorted(maps.Keys(stored)) {
		return fmt.Errorf("sqlite: incompatible schema object %s; open a fresh data directory", name)
	}
	return nil
}

// schemaObject names the object a current statement creates and the definition
// SQLite records for it, which omits the IF NOT EXISTS clause.
func schemaObject(stmt string) (name, definition string, err error) {
	const clause = " IF NOT EXISTS "
	before, after, found := strings.Cut(stmt, clause)
	if !found {
		return "", "", fmt.Errorf("sqlite: schema statement lacks %q: %.40s", strings.TrimSpace(clause), stmt)
	}
	end := strings.IndexFunc(after, func(r rune) bool { return unicode.IsSpace(r) || r == '(' })
	if end <= 0 {
		return "", "", fmt.Errorf("sqlite: schema statement names no object: %.40s", stmt)
	}
	return after[:end], before + " " + after, nil
}
