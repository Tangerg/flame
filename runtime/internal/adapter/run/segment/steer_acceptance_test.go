package segment

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

func TestSteerAcceptanceSharesTreeTransactionAndExactApplication(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state, history := sqlite.NewRunStore(db), sqlite.NewTranscriptStore(db)
	draft := run.Draft{RunID: "run_steer", SessionID: "ses_steer", SegmentID: "seg_steer", CreatedAt: time.Unix(1, 0).UTC(), ModelSelection: testsupport.DefaultModelSelection()}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatal(err)
	}
	commands := persistence.NewCommandAcceptanceStore(db)
	trees := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	input := []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "original guidance"}, {Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte("image")}}
	acceptance := runs.CommandAcceptance{Key: "steer-original", Fingerprint: strings.Repeat("a", 64), Operation: runs.CommandSteerInput, SessionID: draft.SessionID, RunID: draft.RunID, SegmentID: draft.SegmentID, UserItemID: "item_steer", SteerInput: input}
	payload := []byte(`{"head":"signal admitted"}`)
	head := runs.ExecutionTreeHead{Sequence: 1, CommitID: "steer-admitted", CommitDigest: "steer-admitted", SessionID: draft.SessionID, RootID: "member_root", Writer: "writer", Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(payload)), Payload: payload}
	checkpoint := runs.ExecutorCheckpoint{SessionID: draft.SessionID, RootMemberID: head.RootID, BuildID: "sha256:" + strings.Repeat("b", 64), Payload: []byte(`{"pending":"original"}`)}
	rollback := true
	transactions := 0
	effects := mustNewEffects(Config{State: state, Transcript: history, ExecutionTrees: trees, ExecutorCheckpoints: trees, CommandAcceptances: commands, Tx: func(ctx context.Context, apply func(context.Context) error) error {
		transactions++
		err := sqlitebase.RunInTx(ctx, db, func(ctx context.Context) error {
			if err := apply(ctx); err != nil {
				return err
			}
			if rollback {
				return errors.New("before COMMIT")
			}
			return nil
		})
		if err == nil {
			return errors.New("COMMIT acknowledgement lost")
		}
		return err
	}})
	commit := runs.ExecutionTreeCommit{Update: runs.ExecutionTreeUpdate{Head: head}, Context: &checkpoint, InputAcceptances: []runs.CommandAcceptance{acceptance}}
	if err := effects.CommitExecutionTree(ctx, commit); err == nil {
		t.Fatal("rollback accepted input")
	}
	if record, err := commands.Lookup(ctx, acceptance.Key); err != nil || record.Found {
		t.Fatalf("rollback left acceptance: %+v,%v", record, err)
	}
	if _, found, err := trees.LoadExecutionTree(ctx, draft.SessionID, head.RootID); err != nil || found {
		t.Fatalf("rollback left head: %t,%v", found, err)
	}
	rollback = false
	if err := effects.CommitExecutionTree(ctx, commit); err != nil {
		t.Fatal(err)
	}
	if transactions != 2 {
		t.Fatalf("uncertain ACK repeated writes: %d", transactions)
	}
	record, err := commands.Lookup(ctx, acceptance.Key)
	if err != nil || !record.Found || !record.Acceptance.Equal(acceptance) || record.AppliedItem != nil {
		t.Fatalf("receipt: %+v,%v", record, err)
	}
	if _, err := trees.LoadCheckpoint(ctx, head.RootID); err != nil {
		t.Fatal(err)
	}
	acceptedInputs, err := commands.SessionSteerInputs(ctx, draft.SessionID)
	if err != nil || len(acceptedInputs) != 1 || !acceptedInputs[0].Equal(acceptance) {
		t.Fatalf("accepted inputs=%+v,%v", acceptedInputs, err)
	}
	other := acceptance.Clone()
	other.SteerInput[0].Text = "changed draft"
	if err := commands.Save(ctx, other); !errors.Is(err, runs.ErrCommandKeyConflict) {
		t.Fatalf("changed command: %v", err)
	}
	item, err := transcript.NewUserMessage(transcript.ItemIdentity{SessionID: draft.SessionID, RunID: draft.RunID, ItemID: acceptance.UserItemID, OccurredAt: draft.CreatedAt}, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := history.AppendItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	record, err = commands.Lookup(ctx, acceptance.Key)
	if err != nil || record.AppliedItem == nil || record.AppliedItem.ID() != item.ID() {
		t.Fatalf("application=%+v,%v", record, err)
	}
	acceptedInputs, err = commands.SessionSteerInputs(ctx, draft.SessionID)
	if err != nil || len(acceptedInputs) != 1 || !acceptedInputs[0].Equal(acceptance) {
		t.Fatalf("applied input identity was lost: %+v,%v", acceptedInputs, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM runs WHERE run_id=?`, draft.RunID); err != nil {
		t.Fatal(err)
	}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatal(err)
	}
	if err := commands.Save(ctx, acceptance); err != nil {
		t.Fatal(err)
	}
	record, err = commands.Lookup(ctx, acceptance.Key)
	if err != nil || record.CurrentRun != nil || record.AppliedItem != nil || !record.Acceptance.Equal(acceptance) {
		t.Fatalf("retired command rebound: %+v,%v", record, err)
	}
}
