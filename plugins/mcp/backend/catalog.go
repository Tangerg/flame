package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"time"

	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	framesqlite "github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const entriesSchema = `CREATE TABLE IF NOT EXISTS entries(name TEXT PRIMARY KEY, revision TEXT NOT NULL, grant TEXT NOT NULL, enabled INTEGER NOT NULL CHECK(enabled IN (0,1)), description TEXT NOT NULL, handshake_seconds INTEGER NOT NULL CHECK(handshake_seconds>=0))`
const attemptsSchema = `CREATE TABLE IF NOT EXISTS attempts(
 id TEXT PRIMARY KEY, name TEXT NOT NULL, epoch TEXT NOT NULL,
 status TEXT NOT NULL, started INTEGER NOT NULL, finished INTEGER,
 CHECK(finished IS NULL OR finished>=started),
 CHECK((status='` + string(wire.Pending) + `' AND finished IS NULL) OR
 (status IN ('` + string(wire.Unconfirmed) + `','` + string(wire.Succeeded) + `','` + string(wire.Failed) + `','` + string(wire.Canceled) + `') AND finished IS NOT NULL))
)`

type catalog struct {
	db  *framesqlite.Database
	now func() time.Time
}

func (c catalog) list(ctx context.Context, selector wire.Selector) (wire.Directory, error) {
	directory := wire.Directory{Entries: []wire.Entry{}}
	err := c.db.Read(ctx, func(reader framesqlite.Reader) error {
		rows, err := reader.QueryContext(ctx, `SELECT name,revision,grant,enabled,description,handshake_seconds FROM entries WHERE ?='' OR name=? ORDER BY name`, selector.Name, selector.Name)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var entry wire.Entry
			if err := rows.Scan(&entry.Name, &entry.Revision, &entry.LaunchGrant, &entry.Enabled, &entry.Description, &entry.HandshakeSeconds); err != nil {
				return err
			}
			directory.Entries = append(directory.Entries, entry)
		}
		return rows.Err()
	})
	return directory, err
}

func (c catalog) prepareChange(ctx context.Context, _ *sdk.ServerSession, change wire.Change) (pluginstorage.CommandPlan[wire.Decision], error) {
	if change.Name == "" {
		return pluginstorage.NewCommandPlan(wire.Conflict, nil)
	}
	var revision, grant string
	err := framesqlite.Connection(ctx, c.db).QueryRowContext(ctx, `SELECT revision,grant FROM entries WHERE name=?`, change.Name).Scan(&revision, &grant)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return pluginstorage.CommandPlan[wire.Decision]{}, err
	}
	if revision != change.Expected {
		return pluginstorage.NewCommandPlan(wire.Conflict, nil)
	}
	if change.Replacement == nil {
		if grant == "" {
			return pluginstorage.NewCommandPlan(wire.Conflict, nil)
		}
		return pluginstorage.NewCommandPlan(wire.Applied, func(ctx context.Context) error {
			if _, err := framesqlite.Connection(ctx, c.db).ExecContext(ctx, `DELETE FROM entries WHERE name=?`, change.Name); err != nil {
				return err
			}
			return c.cancelPending(ctx, change.Name, c.now())
		})
	}
	entry := *change.Replacement
	if entry.Revision != "" || entry.Name != change.Name || entry.LaunchGrant == "" || entry.HandshakeSeconds < 0 {
		return pluginstorage.NewCommandPlan(wire.Conflict, nil)
	}
	entry.Revision = rand.Text()
	return pluginstorage.NewCommandPlan(wire.Applied, func(ctx context.Context) error {
		if _, err := framesqlite.Connection(ctx, c.db).ExecContext(ctx, `INSERT INTO entries VALUES(?,?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET revision=excluded.revision,grant=excluded.grant, enabled=excluded.enabled, description=excluded.description, handshake_seconds=excluded.handshake_seconds`, entry.Name, entry.Revision, entry.LaunchGrant, entry.Enabled, entry.Description, entry.HandshakeSeconds); err != nil {
			return err
		}
		if !entry.Enabled || grant != entry.LaunchGrant {
			return c.cancelPending(ctx, change.Name, c.now())
		}
		return nil
	})
}

func (c catalog) cancelPending(ctx context.Context, name string, finished time.Time) error {
	_, err := framesqlite.Connection(ctx, c.db).ExecContext(ctx, `UPDATE attempts SET status=?,finished=? WHERE name=? AND status=?`, wire.Canceled, finished.UnixNano(), name, wire.Pending)
	return err
}

func (c catalog) prepareStart(ctx context.Context, _ *sdk.ServerSession, start wire.Start) (pluginstorage.CommandPlan[wire.AttemptResult], error) {
	if start.Epoch == "" || start.AttemptID == "" {
		return pluginstorage.NewCommandPlan(wire.AttemptResult{}, nil)
	}
	var enabled bool
	err := framesqlite.Connection(ctx, c.db).QueryRowContext(ctx, `SELECT enabled FROM entries WHERE name=? AND grant=?`, start.Name, start.Grant).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !enabled {
		return pluginstorage.NewCommandPlan(wire.AttemptResult{}, nil)
	}
	if err != nil {
		return pluginstorage.CommandPlan[wire.AttemptResult]{}, err
	}
	now := c.now()
	attempt := wire.Attempt{ID: start.AttemptID, Name: start.Name, Status: wire.Pending, StartedAt: now.UTC().Format(time.RFC3339Nano)}
	return pluginstorage.NewCommandPlan(wire.AttemptResult{Attempt: &attempt}, func(ctx context.Context) error {
		if err := c.cancelPending(ctx, start.Name, now); err != nil {
			return err
		}
		connection := framesqlite.Connection(ctx, c.db)
		if _, err := connection.ExecContext(ctx, `DELETE FROM attempts WHERE finished<=?`, now.Add(-wire.AttemptRetentionSeconds*time.Second).UnixNano()); err != nil {
			return err
		}
		_, err := connection.ExecContext(ctx, `INSERT INTO attempts(id,name,epoch,status,started) VALUES(?,?,?,?,?)`, start.AttemptID, start.Name, start.Epoch, wire.Pending, now.UnixNano())
		return err
	})
}

func (c catalog) prepareFinish(ctx context.Context, _ *sdk.ServerSession, completion wire.Completion) (pluginstorage.CommandPlan[wire.Decision], error) {
	if completion.Status != wire.Succeeded && completion.Status != wire.Failed && completion.Status != wire.Canceled {
		return pluginstorage.NewCommandPlan(wire.Conflict, nil)
	}
	var pending bool
	if err := framesqlite.Connection(ctx, c.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM attempts WHERE id=? AND epoch=? AND status=?)`, completion.ID, completion.Epoch, wire.Pending).Scan(&pending); err != nil {
		return pluginstorage.CommandPlan[wire.Decision]{}, err
	}
	if !pending {
		return pluginstorage.NewCommandPlan(wire.Conflict, nil)
	}
	finished := c.now().UnixNano()
	return pluginstorage.NewCommandPlan(wire.Applied, func(ctx context.Context) error {
		_, err := framesqlite.Connection(ctx, c.db).ExecContext(ctx, `UPDATE attempts SET status=?,finished=? WHERE id=?`, completion.Status, finished, completion.ID)
		return err
	})
}

func (c catalog) get(ctx context.Context, request wire.Request) (wire.AttemptResult, error) {
	var attempt wire.Attempt
	var epoch string
	var started int64
	var finished sql.NullInt64
	err := c.db.Read(ctx, func(reader framesqlite.Reader) error {
		return reader.QueryRowContext(ctx, `SELECT id,name,epoch,status,started,finished FROM attempts WHERE id=? AND (finished IS NULL OR finished>?)`, request.ID, c.now().Add(-wire.AttemptRetentionSeconds*time.Second).UnixNano()).Scan(&attempt.ID, &attempt.Name, &epoch, &attempt.Status, &started, &finished)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return wire.AttemptResult{}, nil
	}
	if err != nil {
		return wire.AttemptResult{}, err
	}
	attempt.StartedAt = time.Unix(0, started).UTC().Format(time.RFC3339Nano)
	if finished.Valid {
		attempt.FinishedAt = time.Unix(0, finished.Int64).UTC().Format(time.RFC3339Nano)
	}
	if attempt.Status == wire.Pending && epoch != request.Epoch {
		return wire.AttemptResult{}, nil
	}
	return wire.AttemptResult{Attempt: &attempt}, nil
}

func (c catalog) prepareRecovery(_ context.Context, _ *sdk.ServerSession, recovery wire.Recovery) (pluginstorage.CommandPlan[wire.Decision], error) {
	if recovery.Epoch == "" {
		return pluginstorage.NewCommandPlan(wire.Conflict, nil)
	}
	finished := c.now().UnixNano()
	return pluginstorage.NewCommandPlan(wire.Applied, func(ctx context.Context) error {
		_, err := framesqlite.Connection(ctx, c.db).ExecContext(ctx, `UPDATE attempts SET status=?,finished=? WHERE epoch<>? AND status=?`, wire.Unconfirmed, finished, recovery.Epoch, wire.Pending)
		return err
	})
}
