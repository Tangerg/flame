package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/plugins/github/internal/wire"
	"github.com/Tangerg/frame/sqlite"
)

const issueDeliveriesSchema = `CREATE TABLE IF NOT EXISTS issue_deliveries (
	id TEXT PRIMARY KEY, scope TEXT NOT NULL, number INTEGER NOT NULL,
	revision INTEGER NOT NULL, status TEXT NOT NULL, record BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS issue_deliveries_scope ON issue_deliveries(scope, number DESC);`

const (
	issueQueued   = "queued"
	issueRunning  = "running"
	issueChecking = "checking"
	issueReady    = "ready"
	issueBlocked  = "blocked"
	issueRevoked  = "revoked"
)

type issueDeliveryRecord struct {
	wire.IssueDelivery
	Revision    int64          `json:"revision"`
	RepoID      int64          `json:"repoId"`
	IssueID     int64          `json:"issueId"`
	Rule        string         `json:"rule"`
	Source      string         `json:"source"`
	Input       string         `json:"input"`
	Namespace   string         `json:"namespace"`
	Publication *issueApproval `json:"publication,omitempty"`
}

type issueApproval struct {
	Key      string    `json:"key"`
	Digest   string    `json:"digest"`
	Approved time.Time `json:"approved"`
}

func issueKey(project string, repoID, issueID int64) string {
	return digestText(fmt.Sprintf("issue verification\n%s\n%d\n%d", project, repoID, issueID))
}

func digestText(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func (s *store) enqueueIssue(ctx context.Context, scope string, work issueDeliveryRecord) (bool, error) {
	data, err := json.Marshal(work)
	if err != nil {
		return false, err
	}
	var created bool
	err = s.db.RunInTx(ctx, func(ctx context.Context) error {
		result, err := s.db.Executor(ctx).ExecContext(ctx, `INSERT INTO issue_deliveries(id,scope,number,revision,status,record)
			VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, work.ID, scope, work.Number, work.Revision, work.Status, data)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		created = n == 1
		return err
	})
	return created, err
}

func readIssueRecord(raw []byte) (issueDeliveryRecord, error) {
	var work issueDeliveryRecord
	err := json.Unmarshal(raw, &work, json.RejectUnknownMembers(true))
	return work, err
}

func (s *store) readIssueDelivery(ctx context.Context, scope, id string) (issueDeliveryRecord, error) {
	var work issueDeliveryRecord
	err := s.db.Read(ctx, func(reader sqlite.Reader) error {
		var raw []byte
		if err := reader.QueryRowContext(ctx, `SELECT record FROM issue_deliveries WHERE id=? AND scope=?`, id, scope).Scan(&raw); err != nil {
			return err
		}
		var err error
		work, err = readIssueRecord(raw)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return issueDeliveryRecord{}, refused(refusalNotFound, "issue delivery is not visible")
	}
	return work, err
}

func (s *store) issueDeliveries(ctx context.Context, scope string, before int64, active bool) ([]issueDeliveryRecord, error) {
	items := make([]issueDeliveryRecord, 0)
	err := s.db.Read(ctx, func(reader sqlite.Reader) error {
		query := `SELECT record FROM issue_deliveries WHERE scope=? AND (?=0 OR number<?)
			AND (?=0 OR status IN ('queued','running','checking','ready')) ORDER BY number DESC LIMIT 25`
		rows, err := reader.QueryContext(ctx, query, scope, before, before, active)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			item, err := readIssueRecord(raw)
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *store) saveIssueDelivery(ctx context.Context, scope string, work issueDeliveryRecord) error {
	expected := work.Revision
	work.Revision++
	raw, err := json.Marshal(work)
	if err != nil {
		return err
	}
	return s.db.RunInTx(ctx, func(ctx context.Context) error {
		updated, err := s.db.Executor(ctx).ExecContext(ctx, `UPDATE issue_deliveries SET revision=?,status=?,record=?
			WHERE id=? AND scope=? AND revision=?`, work.Revision, work.Status, raw, work.ID, scope, expected)
		if err != nil {
			return err
		}
		n, err := updated.RowsAffected()
		if err == nil && n != 1 {
			return refused(refusalConflict, "issue delivery changed")
		}
		return err
	})
}
