package segment

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func testRootExecutorCheckpoint() run.Checkpoint {
	const rootMemberID = "member_1"

	selection, err := modelref.New("anthropic", "claude")
	if err != nil {
		panic(err)
	}
	return testsupport.MustCheckpoint(run.CheckpointState{
		RootMemberID:   rootMemberID,
		Payload:        []byte("opaque root checkpoint"),
		BuildID:        checkpointBuildID,
		Scope:          run.ExecutionScope{SessionID: "ses_1"},
		ModelSelection: selection,
	})
}

type recordingExecutorCheckpointStore struct {
	saved     []run.Checkpoint
	deleted   [][]string
	saveErr   error
	deleteErr error
}

func (r *recordingExecutorCheckpointStore) SaveCheckpoint(
	_ context.Context,
	checkpoint run.Checkpoint,
) error {
	r.saved = append(r.saved, checkpoint)
	return r.saveErr
}

func (r *recordingExecutorCheckpointStore) LoadCheckpoint(
	_ context.Context,
	rootMemberID string,
) (run.Checkpoint, error) {
	for index := len(r.saved) - 1; index >= 0; index-- {
		if r.saved[index].RootMemberID() == rootMemberID {
			return r.saved[index], nil
		}
	}
	return run.Checkpoint{}, run.ErrCheckpointNotFound
}

func (r *recordingExecutorCheckpointStore) DeleteCheckpoints(
	_ context.Context,
	_ string,
	rootIDs []string,
) error {
	r.deleted = append(r.deleted, append([]string(nil), rootIDs...))
	return r.deleteErr
}
