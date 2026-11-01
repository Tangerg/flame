package persistence

import (
	"context"
	"database/sql"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/annotation"
	"github.com/Tangerg/qimu/jsonvalue"
	"github.com/Tangerg/qimu/sqlite"
)

type annotationStore struct{ db *sql.DB }

func (s *annotationStore) Append(ctx context.Context, e annotation.Entry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	// An identical retry keeps the original timestamp, including after a lost commit acknowledgement.
	result, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO session_annotations
		(command_id,installation,release,contribution,contract,session_id,run_id,item_id,content,created_at_ns)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(command_id) DO UPDATE SET command_id=excluded.command_id
		WHERE (installation,release,contribution,contract,session_id,run_id,item_id,content)=
		      (excluded.installation,excluded.release,excluded.contribution,excluded.contract,excluded.session_id,excluded.run_id,excluded.item_id,excluded.content)`,
		e.CommandID, e.Source.InstallationID, e.Source.Release, e.Source.Contribution, e.Source.ContractReference,
		e.Subject.SessionID, e.Subject.RunID, e.Subject.ItemID, e.Content.String(), e.CreatedAt.UnixNano())
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return annotation.ErrConflict
	}
	return err
}

func (s *annotationStore) ListSession(ctx context.Context, sessionID string) ([]annotation.Entry, error) {
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT command_id,installation,release,contribution,contract,
	session_id,run_id,item_id,content,created_at_ns FROM session_annotations WHERE `+sessionAnnotationPredicate+` ORDER BY created_at_ns,command_id`, sessionID, sessionID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]annotation.Entry, 0)
	for rows.Next() {
		var e annotation.Entry
		var content []byte
		var ns int64
		if err := rows.Scan(&e.CommandID, &e.Source.InstallationID, &e.Source.Release, &e.Source.Contribution, &e.Source.ContractReference, &e.Subject.SessionID, &e.Subject.RunID, &e.Subject.ItemID, &content, &ns); err != nil {
			return nil, err
		}
		e.CreatedAt = time.Unix(0, ns).UTC()
		e.Content, err = jsonvalue.Parse(content)
		if err != nil {
			return nil, err
		}
		if err := e.Validate(); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
