package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"time"

	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const entriesSchema = `CREATE TABLE IF NOT EXISTS entries(name TEXT PRIMARY KEY, revision TEXT NOT NULL, grant TEXT NOT NULL, enabled INTEGER NOT NULL, description TEXT NOT NULL, handshake_seconds INTEGER NOT NULL CHECK(handshake_seconds>=0))`
const attemptsSchema = `CREATE TABLE IF NOT EXISTS attempts(id TEXT PRIMARY KEY, name TEXT NOT NULL, epoch TEXT NOT NULL, status TEXT NOT NULL, started INTEGER NOT NULL, finished INTEGER)`

type catalog struct {
	db  *sql.DB
	now func() time.Time
}

func (c catalog) list(ctx context.Context, s wire.Selector) (wire.Directory, error) {
	rows, err := sqlite.Connection(ctx, c.db).QueryContext(ctx, `SELECT name,revision,grant,enabled,description,handshake_seconds FROM entries WHERE ?='' OR name=? ORDER BY name`, s.Name, s.Name)
	out := wire.Directory{Entries: []wire.Entry{}}
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e wire.Entry
		if err := rows.Scan(&e.Name, &e.Revision, &e.LaunchGrant, &e.Enabled, &e.Description, &e.HandshakeSeconds); err != nil {
			return out, err
		}
		out.Entries = append(out.Entries, e)
	}
	return out, rows.Err()
}
func (c catalog) change(ctx context.Context, _ *sdk.ServerSession, ch wire.Change) (wire.Decision, error) {
	if ch.Name == "" {
		return wire.Conflict, nil
	}
	var revision, grant string
	db := sqlite.Connection(ctx, c.db)
	err := db.QueryRowContext(ctx, `SELECT revision,grant FROM entries WHERE name=?`, ch.Name).Scan(&revision, &grant)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if revision != ch.Expected {
		return wire.Conflict, nil
	}
	e := ch.Replacement
	if e == nil {
		if grant == "" {
			return wire.Conflict, nil
		}
		_, err = db.ExecContext(ctx, `DELETE FROM entries WHERE name=?`, ch.Name)
	} else {
		if e.Revision != "" || e.Name != ch.Name || e.LaunchGrant == "" || e.HandshakeSeconds < 0 {
			return wire.Conflict, nil
		}
		_, err = db.ExecContext(ctx, `INSERT INTO entries VALUES(?,?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET revision=excluded.revision,grant=excluded.grant, enabled=excluded.enabled, description=excluded.description, handshake_seconds=excluded.handshake_seconds`, e.Name, rand.Text(), e.LaunchGrant, e.Enabled, e.Description, e.HandshakeSeconds)
	}
	if err != nil {
		return "", err
	}
	if e == nil || !e.Enabled || grant != e.LaunchGrant {
		err = c.cancelPending(ctx, ch.Name)
	}
	return wire.Applied, err
}
func (c catalog) cancelPending(ctx context.Context, name string) error {
	_, err := sqlite.Connection(ctx, c.db).ExecContext(ctx, `UPDATE attempts SET status=?,finished=? WHERE name=? AND status=?`, wire.Canceled, c.now().UnixNano(), name, wire.Pending)
	return err
}
func (c catalog) start(ctx context.Context, _ *sdk.ServerSession, s wire.Start) (wire.AttemptResult, error) {
	if s.Epoch == "" || s.AttemptID == "" {
		return wire.AttemptResult{}, nil
	}
	var enabled bool
	db := sqlite.Connection(ctx, c.db)
	err := db.QueryRowContext(ctx, `SELECT enabled FROM entries WHERE name=? AND grant=?`, s.Name, s.Grant).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !enabled {
		return wire.AttemptResult{}, nil
	}
	if err != nil {
		return wire.AttemptResult{}, err
	}
	if err := c.cancelPending(ctx, s.Name); err != nil {
		return wire.AttemptResult{}, err
	}
	now := c.now()
	if _, err := db.ExecContext(ctx, `DELETE FROM attempts WHERE finished<=?`, now.Add(-wire.AttemptRetentionSeconds*time.Second).UnixNano()); err != nil {
		return wire.AttemptResult{}, err
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO attempts(id,name,epoch,status,started) VALUES(?,?,?,?,?)`, s.AttemptID, s.Name, s.Epoch, wire.Pending, now.UnixNano()); err != nil {
		return wire.AttemptResult{}, err
	}
	return c.get(ctx, wire.Request{ID: s.AttemptID, Epoch: s.Epoch})
}
func (c catalog) finish(ctx context.Context, _ *sdk.ServerSession, s wire.Completion) (wire.Decision, error) {
	if s.Status != wire.Succeeded && s.Status != wire.Failed && s.Status != wire.Canceled {
		return wire.Conflict, nil
	}
	r, err := sqlite.Connection(ctx, c.db).ExecContext(ctx, `UPDATE attempts SET status=?,finished=? WHERE id=? AND epoch=? AND status=?`, s.Status, c.now().UnixNano(), s.ID, s.Epoch, wire.Pending)
	if err != nil {
		return "", err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return wire.Conflict, nil
	}
	return wire.Applied, nil
}
func (c catalog) get(ctx context.Context, r wire.Request) (wire.AttemptResult, error) {
	var a wire.Attempt
	var epoch string
	var started int64
	var finished sql.NullInt64
	err := sqlite.Connection(ctx, c.db).QueryRowContext(ctx, `SELECT id,name,epoch,status,started,finished FROM attempts WHERE id=? AND (finished IS NULL OR finished>?)`, r.ID, c.now().Add(-wire.AttemptRetentionSeconds*time.Second).UnixNano()).Scan(&a.ID, &a.Name, &epoch, &a.Status, &started, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return wire.AttemptResult{}, nil
	}
	if err != nil {
		return wire.AttemptResult{}, err
	}
	a.StartedAt = time.Unix(0, started).UTC().Format(time.RFC3339Nano)
	if finished.Valid {
		a.FinishedAt = time.Unix(0, finished.Int64).UTC().Format(time.RFC3339Nano)
	}
	if a.Status == wire.Pending && epoch != r.Epoch {
		return wire.AttemptResult{}, nil
	}
	return wire.AttemptResult{Attempt: &a}, nil
}

func (c catalog) recover(ctx context.Context, _ *sdk.ServerSession, r wire.Recovery) (wire.Decision, error) {
	if r.Epoch == "" {
		return wire.Conflict, nil
	}
	_, err := sqlite.Connection(ctx, c.db).ExecContext(ctx, `UPDATE attempts SET status=?,finished=? WHERE epoch<>? AND status=?`, wire.Unconfirmed, c.now().UnixNano(), r.Epoch, wire.Pending)
	return wire.Applied, err
}
