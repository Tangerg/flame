package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/plugins/methods/internal/wire"
	"github.com/Tangerg/frame/sqlite"
)

const methodSchema = `
CREATE TABLE IF NOT EXISTS method_versions (
	project TEXT NOT NULL, id TEXT NOT NULL, value BLOB NOT NULL, UNIQUE(project,id)
);
CREATE TABLE IF NOT EXISTS method_executions (
	id TEXT NOT NULL UNIQUE, project TEXT NOT NULL, workspace TEXT NOT NULL,
	state TEXT NOT NULL, revision INTEGER NOT NULL, value BLOB NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS method_open_worktree
	ON method_executions(workspace) WHERE state NOT IN ('approved','blocked');
CREATE INDEX IF NOT EXISTS method_project ON method_executions(project);`

type record struct {
	wire.Execution
	Namespace string `json:"namespace"`
	Before    string `json:"before"`
	Revision  int64  `json:"revision"`
}

type store struct{ db sqlite.Database }

func methodDigest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func (s store) latest(ctx context.Context, project string) (*wire.Version, error) {
	version, err := s.readVersion(ctx, `SELECT value FROM method_versions WHERE project=? ORDER BY rowid DESC LIMIT 1`, project)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return version, err
}

func (s store) version(ctx context.Context, project, id string) (*wire.Version, error) {
	version, err := s.readVersion(ctx, `SELECT value FROM method_versions WHERE project=? AND id=?`, project, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, refuse("the method version is unavailable in this project")
	}
	return version, err
}

func (s store) readVersion(ctx context.Context, query string, args ...any) (*wire.Version, error) {
	var raw []byte
	err := s.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		return s.db.Executor(tx).QueryRowContext(tx, query, args...).Scan(&raw)
	})
	if err != nil {
		return nil, err
	}
	version := new(wire.Version)
	return version, json.Unmarshal(raw, version)
}

func (s store) putVersion(ctx context.Context, project string, version wire.Version) error {
	raw, err := json.Marshal(version)
	if err != nil {
		return err
	}
	_, err = s.db.Executor(ctx).ExecContext(ctx, `INSERT INTO method_versions(project,id,value)
		VALUES(?,?,?) ON CONFLICT(project,id) DO NOTHING`, project, version.ID, raw)
	return err
}

func (s store) openWorktree(ctx context.Context, workspace string) (bool, error) {
	var found bool
	err := s.db.Executor(ctx).QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM method_executions
		WHERE workspace=? AND state NOT IN ('approved','blocked'))`, workspace).Scan(&found)
	return found, err
}

func (s store) putExecution(ctx context.Context, work record) error {
	raw, err := json.Marshal(work)
	if err != nil {
		return err
	}
	_, err = s.db.Executor(ctx).ExecContext(ctx, `INSERT INTO method_executions(id,project,workspace,state,revision,value)
		VALUES(?,?,?,?,?,?)`, work.ID, work.ProjectID, work.Workspace, work.State, work.Revision, raw)
	return err
}

func (s store) execution(ctx context.Context, project, id string) (record, error) {
	var raw []byte
	err := s.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		return s.db.Executor(tx).QueryRowContext(tx, "SELECT value FROM method_executions WHERE project=? AND id=?", project, id).Scan(&raw)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return record{}, refuse("the method execution is unavailable in this project")
	}
	if err != nil {
		return record{}, err
	}
	var work record
	return work, json.Unmarshal(raw, &work)
}

func (s store) executions(ctx context.Context, query string, args ...any) ([]record, error) {
	rows := []record{}
	err := s.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		records, err := s.db.Executor(tx).QueryContext(tx, query, args...)
		if err != nil {
			return err
		}
		defer records.Close()
		for records.Next() {
			var raw []byte
			var work record
			if err := records.Scan(&raw); err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &work); err != nil {
				return err
			}
			rows = append(rows, work)
		}
		return records.Err()
	})
	return rows, err
}

func (s store) update(ctx context.Context, work record) error {
	expected := work.Revision
	work.Revision++
	raw, err := json.Marshal(work)
	if err != nil {
		return err
	}
	return s.db.RunInTx(ctx, func(tx context.Context) error {
		result, err := s.db.Executor(tx).ExecContext(tx, `UPDATE method_executions SET state=?,revision=?,value=?
			WHERE id=? AND project=? AND revision=?`, work.State, work.Revision, raw, work.ID, work.ProjectID, expected)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err == nil && count != 1 {
			return refuse("the method execution changed")
		}
		return err
	})
}

type rejection string

func (r rejection) Error() string { return string(r) }

func refuse(message string) error { return rejection(message) }

func classify(err error) (string, bool) {
	_, known := errors.AsType[rejection](err)
	return "method_refused", known
}
