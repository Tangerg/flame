package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	pluginstorage "github.com/Tangerg/flame/packages/plugin-sdk/go/sqlite"
	"github.com/Tangerg/flame/plugins/usagereports/backend/domain"
	"github.com/Tangerg/flame/plugins/usagereports/backend/rpc"
	"github.com/Tangerg/flame/runtime/protocol"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

type store struct{ db *sql.DB }

var errStoredReport = errors.New("usage reports: invalid stored report")

func openStore(ctx context.Context, path string) (*store, error) {
	db, err := sqlitebase.OpenSchema(ctx, path, []string{pluginstorage.CommandsSchema, `CREATE TABLE IF NOT EXISTS reports (id TEXT PRIMARY KEY, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`})
	if err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}

func decodeReport(id, raw string) (rpc.Report[protocol.UsageSummary], error) {
	var snapshot domain.Snapshot
	if err := json.Unmarshal([]byte(raw), &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return rpc.Report[protocol.UsageSummary]{}, errors.Join(errStoredReport, err)
	}
	report, err := domain.Restore(snapshot)
	if err != nil || snapshot.ID != id {
		return rpc.Report[protocol.UsageSummary]{}, errors.Join(errStoredReport, err)
	}
	projected, err := project(report)
	if err != nil {
		return rpc.Report[protocol.UsageSummary]{}, errors.Join(errStoredReport, err)
	}
	return projected, nil
}

func (s *store) read(ctx context.Context, id string) (rpc.Report[protocol.UsageSummary], error) {
	var raw string
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT snapshot FROM reports WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return rpc.Report[protocol.UsageSummary]{}, domain.ErrNotFound
	}
	if err != nil {
		return rpc.Report[protocol.UsageSummary]{}, err
	}
	return decodeReport(id, raw)
}

func (s *store) list(ctx context.Context) ([]rpc.Header, error) {
	rows, err := sqlitebase.Connection(ctx, s.db).QueryContext(ctx, `SELECT id,snapshot FROM reports ORDER BY rowid DESC LIMIT ?`, domain.MaxReports+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]rpc.Header, 0)
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		report, err := decodeReport(id, raw)
		if err != nil {
			return nil, err
		}
		result = append(result, report.Header)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) > domain.MaxReports {
		return nil, errStoredReport
	}
	return result, nil
}

func (s *store) completeCapture(ctx context.Context, prepared *pluginstorage.PreparedCommand, report domain.Report, captureErr error) (rpc.Header, error) {
	var header rpc.Header
	outcomeErr := captureErr
	err := sqlitebase.RunInTx(ctx, s.db, func(ctx context.Context) error {
		if outcomeErr == nil {
			var count int
			if err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT COUNT(*) FROM reports`).Scan(&count); err != nil {
				return err
			}
			if count >= domain.MaxReports {
				outcomeErr = domain.ErrCapacity
			} else {
				value, err := project(report)
				if err != nil {
					return err
				}
				header = value.Header
				encoded, err := json.Marshal(report.Snapshot())
				if err != nil {
					return err
				}
				if _, err := sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO reports(id,snapshot) VALUES(?,?)`, header.ID, string(encoded)); err != nil {
					return err
				}
			}
		}
		reply, err := pluginsdk.Reply(header, outcomeErr, classify)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(reply)
		if err != nil {
			return err
		}
		return prepared.Complete(ctx, encoded)
	})
	if err != nil {
		return rpc.Header{}, err
	}
	return header, outcomeErr
}

func (s *store) delete(ctx context.Context, id string) (bool, error) {
	if _, err := s.read(ctx, id); err != nil {
		return false, err
	}
	_, err := sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `DELETE FROM reports WHERE id=?`, id)
	return err == nil, err
}
