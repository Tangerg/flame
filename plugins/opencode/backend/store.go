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

func sessionID(machine, native, cwd string, program agentProgram) string {
	parts := []string{machine, native, cwd}
	if program.ID != defaultAgent {
		parts = append(parts, program.ID)
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func emptySession(machine, native, cwd string, link *agentLink) wire.Session {
	return wire.Session{
		ID: sessionID(machine, native, cwd, link.program), Machine: machine, Agent: link.name,
		AgentID: link.program.ID, Authority: link.program.authority, NativeID: native, CWD: cwd,
		Events: []wire.Event{}, Permissions: []wire.PermissionRequest{},
	}
}

type sessionSnapshot struct {
	wire.Session
	Authority string `json:"authority,omitempty"`
}

func (c *connector) stored(ctx context.Context, id string) (wire.Session, error) {
	var raw string
	err := c.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		return c.db.Executor(tx).QueryRowContext(tx, "SELECT snapshot FROM native_sessions WHERE id = ?", id).Scan(&raw)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return wire.Session{}, refuse("native session is not registered")
	}
	if err != nil {
		return wire.Session{}, err
	}
	return decodeSession(raw)
}

func decodeSession(raw string) (wire.Session, error) {
	var record sessionSnapshot
	err := json.Unmarshal([]byte(raw), &record)
	record.Session.Authority = record.Authority
	return record.Session, err
}

func (c *connector) original(ctx context.Context, profile, native string) (wire.Session, error) {
	var raw string
	err := c.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		return c.db.Executor(tx).QueryRowContext(tx, `SELECT snapshot FROM native_sessions
			WHERE json_extract(snapshot, '$.machine')=? AND json_extract(snapshot,'$.nativeId')=?
			AND COALESCE(json_extract(snapshot,'$.agentId'),?)=? LIMIT 1`, c.machine, native, defaultAgent, profile).Scan(&raw)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return wire.Session{}, nil
	}
	if err != nil {
		return wire.Session{}, err
	}
	return decodeSession(raw)
}

func (c *connector) save(ctx context.Context, value wire.Session) error {
	value.Connected, value.CanFork = false, false
	value.Capabilities = nil
	raw, err := json.Marshal(sessionSnapshot{value, value.Authority})
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
	return fmt.Errorf("acp: native session observation failed: %w", err)
}
