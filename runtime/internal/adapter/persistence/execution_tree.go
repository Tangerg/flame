package persistence

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

var errResultPublicationConflict = errors.New("sqlite: result publication belongs to another Session or content")

func (s *ExecutorCheckpointStore) ExecutionResultCommitted(ctx context.Context, sessionID identity.SessionID, publication runs.ResultPublication) (bool, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) (bool, error) {
		var owner identity.SessionID
		var stored string
		err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT session_id,digest FROM result_publications WHERE publication_id = ?`, publication.ID).Scan(&owner, &stored)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if owner != sessionID || stored != publication.Digest {
			return false, errResultPublicationConflict
		}
		return true, nil
	})
}

func (s *ExecutorCheckpointStore) LoadExecutionTree(ctx context.Context, sessionID identity.SessionID, rootID string) (runs.ExecutionTreeHead, bool, error) {
	return scopedPair(ctx, s.db.read, func(ctx context.Context) (runs.ExecutionTreeHead, bool, error) {
		head := runs.ExecutionTreeHead{SessionID: sessionID, RootID: rootID}
		var sequence string
		err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT writer, digest, payload, sequence, commit_id, commit_digest FROM execution_trees WHERE session_id = ? AND root_id = ?`, sessionID, rootID).Scan(&head.Writer, &head.Digest, &head.Payload, &sequence, &head.CommitID, &head.CommitDigest)
		if errors.Is(err, sql.ErrNoRows) {
			return runs.ExecutionTreeHead{}, false, nil
		}
		if err != nil {
			return runs.ExecutionTreeHead{}, false, err
		}
		head.Sequence, err = strconv.ParseUint(sequence, 10, 64)
		return head, err == nil, err
	})
}

// SaveExecutionTree advances one tree head by compare-and-swap on the writer and
// digest the executor last observed. Replaying the exact committed head is a
// no-op; any other reuse of a commit identity is a conflict.
func (s *ExecutorCheckpointStore) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	if err := update.Validate(); err != nil {
		return err
	}
	next := update.Head
	return s.db.write(ctx, func(ctx context.Context) error {
		current, found, err := s.LoadExecutionTree(ctx, next.SessionID, next.RootID)
		if err != nil {
			return err
		}
		if found && update.PreviousWriter == next.Writer && current.Writer != next.Writer {
			return runs.ErrExecutionTreeWriterConflict
		}
		committedDigest, committed, err := s.committedTreeDigest(ctx, next.RootID, next.CommitID)
		if err != nil {
			return err
		}
		if committed {
			if found && committedDigest == next.CommitDigest && current.SameCommit(next) {
				return nil
			}
			return runs.ErrExecutionTreeCommitConflict
		}
		if err := admitTreeAdvance(current, found, update); err != nil {
			return err
		}
		return s.writeExecutionTree(ctx, found, next)
	})
}

func (s *ExecutorCheckpointStore) committedTreeDigest(ctx context.Context, rootID, commitID string) (string, bool, error) {
	var digest string
	err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT digest FROM execution_tree_commits WHERE root_id = ? AND commit_id = ?`, rootID, commitID).Scan(&digest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return digest, err == nil, err
}

// admitTreeAdvance accepts a new commit only from the writer and digest the
// update observed. The executor owns batch positions: one atomic append can
// advance several records, and changing writers does not reset them.
func admitTreeAdvance(current runs.ExecutionTreeHead, found bool, update runs.ExecutionTreeUpdate) error {
	next := update.Head
	if next.CommitID == "" || next.CommitDigest == "" || current.Writer != update.PreviousWriter {
		return runs.ErrExecutionTreeWriterConflict
	}
	if current.Digest != update.PreviousDigest || next.Sequence == 0 || found && next.Sequence <= current.Sequence {
		return runs.ErrExecutionTreeCommitConflict
	}
	return nil
}

func (s *ExecutorCheckpointStore) writeExecutionTree(ctx context.Context, found bool, next runs.ExecutionTreeHead) error {
	sequence := strconv.FormatUint(next.Sequence, 10)
	var err error
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
}
