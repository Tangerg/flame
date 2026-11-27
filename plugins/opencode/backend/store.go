package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/plugins/opencode/internal/wire"
)

const sessionSchema = `CREATE TABLE IF NOT EXISTS native_sessions (
	id TEXT PRIMARY KEY, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`

func sessionID(machine, native, cwd string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{machine, native, cwd}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func emptySession(machine, agent, native, cwd string) wire.Session {
	return wire.Session{
		ID: sessionID(machine, native, cwd), Machine: machine, Agent: agent, NativeID: native, CWD: cwd,
		Events: []wire.Event{}, Permissions: []wire.PermissionRequest{},
	}
}

func (c *connector) stored(ctx context.Context, id string) (wire.Session, error) {
	var value wire.Session
	var raw string
	err := c.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		return c.db.Executor(tx).QueryRowContext(tx, "SELECT snapshot FROM native_sessions WHERE id = ?", id).Scan(&raw)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return value, refuse("native session is not registered")
	}
	if err != nil {
		return value, err
	}
	err = json.Unmarshal([]byte(raw), &value)
	return value, err
}

func (c *connector) save(ctx context.Context, value wire.Session) error {
	value.Connected, value.CanFork = false, false
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = c.db.Executor(ctx).ExecContext(ctx, `INSERT INTO native_sessions(id,snapshot) VALUES(?,?)
		ON CONFLICT(id) DO UPDATE SET snapshot = excluded.snapshot`, value.ID, string(raw))
	return err
}

func (c *connector) change(ctx context.Context, id string, apply func(*wire.Session)) error {
	return c.db.RunInTx(ctx, func(tx context.Context) error {
		value, err := c.stored(tx, id)
		if err != nil {
			return err
		}
		apply(&value)
		return c.save(tx, value)
	})
}

type refusal struct{ text string }

func (e refusal) Error() string   { return e.text }
func refuse(message string) error { return refusal{text: message} }

func classify(err error) (string, bool) {
	if _, known := errors.AsType[refusal](err); known {
		return "native_session_unavailable", true
	}
	return "", false
}

func observedFailure(err error) error {
	return fmt.Errorf("OpenCode session observation failed: %w", err)
}
