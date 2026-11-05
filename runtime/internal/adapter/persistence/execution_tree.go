package persistence

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

var errExecutionTreeConflict = errors.New("sqlite: execution tree writer or content conflict")
var errExecutionTreeCommitConflict = fmt.Errorf("%w: commit position or identity conflict", errExecutionTreeConflict)

type executionTreeRecord struct {
	SessionID, RootID, Writer, Digest string
	Payload                           []byte
	Sequence                          uint64
	CommitID, CommitDigest            string
}

func (s *ExecutorCheckpointStore) executionResultCommitted(ctx context.Context, sessionID, id, digest string) (bool, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) (bool, error) {
		var owner, stored string
		err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT session_id,digest FROM result_publications WHERE publication_id = ?`, id).Scan(&owner, &stored)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if owner != sessionID || stored != digest {
			return false, errExecutionTreeConflict
		}
		return true, nil
	})
}
func (s *ExecutorCheckpointStore) loadExecutionTreeRecord(ctx context.Context, sessionID, rootID string) (executionTreeRecord, bool, error) {
	return scopedPair(ctx, s.db.read, func(ctx context.Context) (executionTreeRecord, bool, error) {
		record := executionTreeRecord{SessionID: sessionID, RootID: rootID}
		var sequence string
		err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT writer, digest, payload, sequence, commit_id, commit_digest FROM execution_trees WHERE session_id = ? AND root_id = ?`, sessionID, rootID).Scan(&record.Writer, &record.Digest, &record.Payload, &sequence, &record.CommitID, &record.CommitDigest)
		if errors.Is(err, sql.ErrNoRows) {
			return executionTreeRecord{}, false, nil
		}
		if err != nil {
			return executionTreeRecord{}, false, err
		}
		record.Sequence, err = strconv.ParseUint(sequence, 10, 64)
		return record, err == nil, err
	})
}
func (s *ExecutorCheckpointStore) saveExecutionTreeRecord(ctx context.Context, previousWriter, previousDigest string, next executionTreeRecord) error {
	return s.db.write(ctx, func(ctx context.Context) error {
		current, found, err := s.loadExecutionTreeRecord(ctx, next.SessionID, next.RootID)
		if err != nil {
			return err
		}
		if found && previousWriter == next.Writer && current.Writer != next.Writer {
			return errExecutionTreeConflict
		}
		var committedDigest string
		err = s.db.connection(ctx).QueryRowContext(ctx, `SELECT digest FROM execution_tree_commits WHERE root_id = ? AND commit_id = ?`, next.RootID, next.CommitID).Scan(&committedDigest)
		if err == nil {
			if found && committedDigest == next.CommitDigest && current.Writer == next.Writer && current.Sequence == next.Sequence && current.CommitID == next.CommitID && current.CommitDigest == next.CommitDigest && current.Digest == next.Digest && bytes.Equal(current.Payload, next.Payload) {
				return nil
			}
			return errExecutionTreeCommitConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if next.CommitID == "" || next.CommitDigest == "" || current.Writer != previousWriter {
			return errExecutionTreeConflict
		}
		if current.Digest != previousDigest {
			return errExecutionTreeCommitConflict
		}
		// The executor owns batch positions. One atomic append can advance
		// several records, and changing writers does not reset their positions.
		if next.Sequence == 0 || found && next.Sequence <= current.Sequence {
			return errExecutionTreeCommitConflict
		}
		sequence := strconv.FormatUint(next.Sequence, 10)
		if !found {
			_, err = s.db.connection(ctx).ExecContext(ctx, `INSERT INTO execution_trees(root_id,session_id,writer,digest,payload,sequence,commit_id,commit_digest) VALUES(?,?,?,?,?,?,?,?)`, next.RootID, next.SessionID, next.Writer, next.Digest, next.Payload, sequence, next.CommitID, next.CommitDigest)
		} else {
			_, err = s.db.connection(ctx).ExecContext(ctx, `UPDATE execution_trees SET writer = ?, digest = ?, payload = ?, sequence = ?, commit_id = ?, commit_digest = ? WHERE root_id = ? AND session_id = ?`, next.Writer, next.Digest, next.Payload, sequence, next.CommitID, next.CommitDigest, next.RootID, next.SessionID)
		}
		if err != nil {
			return err
		}
		_, err = s.db.connection(ctx).ExecContext(ctx, `INSERT INTO execution_tree_commits(root_id,commit_id,digest) VALUES(?,?,?)`, next.RootID, next.CommitID, next.CommitDigest)
		return err
	})
}
