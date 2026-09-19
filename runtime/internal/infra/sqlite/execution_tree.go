package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
)

var ErrExecutionTreeConflict = errors.New("sqlite: execution tree writer or content conflict")

type ExecutionTreeRecord struct {
	SessionID, RootID, Writer, Digest string
	Payload                           []byte
}

func (e *ExecutorCheckpointStore) ExecutionResultCommitted(ctx context.Context, sessionID, id, digest string) (bool, error) {
	var owner, stored string
	err := conn(ctx, e.db).QueryRowContext(ctx, `SELECT session_id,digest FROM result_publications WHERE publication_id = ?`, id).Scan(&owner, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if owner != sessionID || stored != digest {
		return false, ErrExecutionTreeConflict
	}
	return true, nil
}

func (e *ExecutorCheckpointStore) LoadExecutionTree(ctx context.Context, sessionID, rootID string) (ExecutionTreeRecord, bool, error) {
	record := ExecutionTreeRecord{SessionID: sessionID, RootID: rootID}
	err := conn(ctx, e.db).QueryRowContext(ctx, `SELECT writer, digest, payload FROM execution_trees WHERE session_id = ? AND root_id = ?`, sessionID, rootID).Scan(&record.Writer, &record.Digest, &record.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionTreeRecord{}, false, nil
	}
	return record, err == nil, err
}

func (e *ExecutorCheckpointStore) SaveExecutionTree(ctx context.Context, previousWriter, previousDigest string, next ExecutionTreeRecord) error {
	return RunInTx(ctx, e.db, func(ctx context.Context) error {
		current, found, err := e.LoadExecutionTree(ctx, next.SessionID, next.RootID)
		if err != nil {
			return err
		}
		if found && current.Writer == next.Writer && current.Digest == next.Digest && bytes.Equal(current.Payload, next.Payload) {
			return nil
		}
		if current.Writer != previousWriter || current.Digest != previousDigest {
			return ErrExecutionTreeConflict
		}
		if !found {
			_, err = conn(ctx, e.db).ExecContext(ctx, `INSERT INTO execution_trees(root_id,session_id,writer,digest,payload) VALUES(?,?,?,?,?)`, next.RootID, next.SessionID, next.Writer, next.Digest, next.Payload)
			return err
		}
		_, err = conn(ctx, e.db).ExecContext(ctx, `UPDATE execution_trees SET writer = ?, digest = ?, payload = ? WHERE root_id = ? AND session_id = ?`, next.Writer, next.Digest, next.Payload, next.RootID, next.SessionID)
		return err
	})
}
