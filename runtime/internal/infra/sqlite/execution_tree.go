package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
)

var ErrExecutionTreeConflict = errors.New("sqlite: execution tree writer or content conflict")

var ErrExecutionTreeCommitConflict = fmt.Errorf("%w: commit identity already used", ErrExecutionTreeConflict)

type ExecutionTreeRecord struct {
	SessionID, RootID, Writer, Digest string
	Payload                           []byte
	Sequence                          uint64
	CommitID, CommitDigest            string
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
	var sequence string
	err := conn(ctx, e.db).QueryRowContext(ctx, `SELECT writer, digest, payload, sequence, commit_id, commit_digest FROM execution_trees WHERE session_id = ? AND root_id = ?`, sessionID, rootID).Scan(&record.Writer, &record.Digest, &record.Payload, &sequence, &record.CommitID, &record.CommitDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionTreeRecord{}, false, nil
	}
	if err != nil {
		return ExecutionTreeRecord{}, false, err
	}
	record.Sequence, err = strconv.ParseUint(sequence, 10, 64)
	return record, err == nil, err
}

func (e *ExecutorCheckpointStore) SaveExecutionTree(ctx context.Context, previousWriter, previousDigest string, next ExecutionTreeRecord) error {
	return RunInTx(ctx, e.db, func(ctx context.Context) error {
		current, found, err := e.LoadExecutionTree(ctx, next.SessionID, next.RootID)
		if err != nil {
			return err
		}
		if found && previousWriter == next.Writer && current.Writer != next.Writer {
			return ErrExecutionTreeConflict
		}
		var committedDigest string
		err = conn(ctx, e.db).QueryRowContext(ctx, `SELECT digest FROM execution_tree_commits WHERE root_id = ? AND commit_id = ?`, next.RootID, next.CommitID).Scan(&committedDigest)
		if err == nil {
			if found && committedDigest == next.CommitDigest && current.Writer == next.Writer && current.Sequence == next.Sequence && current.CommitID == next.CommitID && current.CommitDigest == next.CommitDigest && current.Digest == next.Digest && bytes.Equal(current.Payload, next.Payload) {
				return nil
			}
			return ErrExecutionTreeCommitConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if next.CommitID == "" || next.CommitDigest == "" || current.Writer != previousWriter || current.Digest != previousDigest {
			return ErrExecutionTreeConflict
		}
		switch {
		case !found:
			if previousWriter != "" || previousDigest != "" || next.Sequence != 1 {
				return ErrExecutionTreeConflict
			}
		case current.Writer != next.Writer:
			if next.Sequence != 0 {
				return ErrExecutionTreeConflict
			}
		default:
			if current.Sequence == math.MaxUint64 || next.Sequence != current.Sequence+1 {
				return ErrExecutionTreeConflict
			}
		}
		sequence := strconv.FormatUint(next.Sequence, 10)
		if !found {
			_, err = conn(ctx, e.db).ExecContext(ctx, `INSERT INTO execution_trees(root_id,session_id,writer,digest,payload,sequence,commit_id,commit_digest) VALUES(?,?,?,?,?,?,?,?)`, next.RootID, next.SessionID, next.Writer, next.Digest, next.Payload, sequence, next.CommitID, next.CommitDigest)
		} else {
			_, err = conn(ctx, e.db).ExecContext(ctx, `UPDATE execution_trees SET writer = ?, digest = ?, payload = ?, sequence = ?, commit_id = ?, commit_digest = ? WHERE root_id = ? AND session_id = ?`, next.Writer, next.Digest, next.Payload, sequence, next.CommitID, next.CommitDigest, next.RootID, next.SessionID)
		}
		if err != nil {
			return err
		}
		_, err = conn(ctx, e.db).ExecContext(ctx, `INSERT INTO execution_tree_commits(root_id,commit_id,digest) VALUES(?,?,?)`, next.RootID, next.CommitID, next.CommitDigest)
		return err
	})
}
