package segment

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestResultPublicationTransactionReceiptsAndSegmentFence(t *testing.T) {
	for _, lostReceipt := range []bool{false, true} {
		t.Run(map[bool]string{false: "acknowledged", true: "receipt lost"}[lostReceipt], func(t *testing.T) {
			ctx := t.Context()
			db, err := sqlite.Open(ctx, ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			state := sqlite.NewRunStore(db)
			messages := sqlite.NewMessageStore(db)
			history := sqlite.NewTranscriptStore(db)
			started := time.Unix(1, 0).UTC()
			draft := run.Draft{RunID: "run_publication", SessionID: "ses_publication", SegmentID: "seg_first", CreatedAt: started, ModelSelection: testsupport.DefaultModelSelection()}
			if err := state.Admit(ctx, draft); err != nil {
				t.Fatal(err)
			}
			trees := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
			payload := []byte(`{"tree":"settled"}`)
			update := runs.ExecutionTreeUpdate{Head: runs.ExecutionTreeHead{Sequence: 1, CommitID: "initial", CommitDigest: "initial", SessionID: draft.SessionID, RootID: "root", Writer: "writer", Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(payload)), Payload: payload}}
			rollback := true
			transactions := 0
			effects := mustNewEffects(Config{
				ExecutionTrees: trees, State: state, Transcript: history, Conversation: messages, ToolInvocations: sqlite.NewToolInvocationStore(db),
				Tx: func(ctx context.Context, fn func(context.Context) error) error {
					transactions++
					err := sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
						if err := fn(ctx); err != nil {
							return err
						}
						if rollback {
							return errors.New("transaction rejected before COMMIT")
						}
						return nil
					})
					if err == nil && lostReceipt {
						return errors.New("COMMIT response lost")
					}
					return err
				},
			})
			modelResult := chat.ToolResult{ID: "provider_call", Name: "unavailable", IsError: true, Output: chat.NewTextToolOutput("exact rejection\n" + strings.Repeat("界", 900))}
			presentation := tool.StringResult("UI summary")
			commit := runs.EventCommit{
				RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID, CommitID: testCommitID("run_commit_result_first"),
				ResultPublication: &runs.ResultPublication{ID: "effect_result_first", Digest: "sha256:" + strings.Repeat("a", 64)},
				Items: []transcript.Item{testsupport.MustRestoreItem(testsupport.ItemInput{
					SessionID: draft.SessionID, RunID: draft.RunID, ID: "item_rejected", OccurredAt: started, FinishedAt: started.Add(time.Second),
					Status: transcript.ItemCompleted, Kind: transcript.ToolCall, SafetyClass: tool.SafetyClassSafe,
					Tool: &transcript.ToolInvocation{Name: "unavailable", Arguments: tool.Arguments{}, Result: &presentation},
				})},
				ToolInvocations: []runs.ToolInvocationCommit{{CallID: "tool_rejected", ItemID: "item_rejected", SegmentID: draft.SegmentID,
					State: runs.ToolInvocationCompleted, StartedAt: started, FinishedAt: started.Add(time.Second)}},
				ConversationMessages: []chat.Message{chat.NewToolMessage(modelResult)},
			}
			if err := effects.CommitExecutionTree(ctx, update, []runs.EventCommit{commit}); err == nil {
				t.Fatal("rolled-back publication succeeded")
			}
			if found, err := state.ResultPublicationCommitted(ctx, draft.SessionID, draft.RunID, draft.SegmentID, commit.ResultPublication.ID, commit.ResultPublication.Digest); err != nil || found {
				t.Fatalf("rolled-back receipt = %t, %v", found, err)
			}
			if count, err := messages.Count(ctx, draft.SessionID); err != nil || count != 0 {
				t.Fatalf("rolled-back messages = %d, %v", count, err)
			}
			if items, err := history.List(ctx, draft.SessionID); err != nil || len(items) != 0 {
				t.Fatalf("rolled-back items = %v, %v", items, err)
			}
			if _, found, err := trees.LoadExecutionTree(ctx, draft.SessionID, "root"); err != nil || found {
				t.Fatalf("rolled-back tree = %t, %v", found, err)
			}
			rollback = false
			if err := effects.CommitExecutionTree(ctx, update, []runs.EventCommit{commit}); err != nil {
				t.Fatalf("publication: %v", err)
			}
			if transactions != 2 {
				t.Fatalf("ambiguous commit retried writes: %d", transactions)
			}
			if found, err := state.ResultPublicationCommitted(ctx, draft.SessionID, draft.RunID, draft.SegmentID, commit.ResultPublication.ID, commit.ResultPublication.Digest); err != nil || !found {
				t.Fatalf("stored receipt = %t, %v", found, err)
			}
			// A new product write attempt still denotes the same Scope publication.
			commit.CommitID = testCommitID("run_commit_result_duplicate")
			if err := effects.CommitExecutionTree(ctx, update, []runs.EventCommit{commit}); err != nil {
				t.Fatalf("same publication: %v", err)
			}
			var journalCount int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_invocations WHERE state = 'completed'`).Scan(&journalCount); err != nil || journalCount != 1 {
				t.Fatalf("journal = %d, %v", journalCount, err)
			}
			stored, err := messages.Read(ctx, draft.SessionID)
			if err != nil || !reflect.DeepEqual(stored, commit.ConversationMessages) {
				t.Fatalf("canonical messages changed or duplicated: %+v, %v", stored, err)
			}
			conflict := commit
			conflict.ResultPublication = new(*commit.ResultPublication)
			conflict.ResultPublication.Digest = "sha256:" + strings.Repeat("b", 64)
			if found, err := state.ResultPublicationCommitted(ctx, draft.SessionID, draft.RunID, draft.SegmentID, conflict.ResultPublication.ID, conflict.ResultPublication.Digest); err == nil || found {
				t.Fatalf("conflicting receipt = %t, %v", found, err)
			}
			if err := effects.CommitExecutionTree(ctx, update, []runs.EventCommit{conflict}); err == nil {
				t.Fatal("same identity with different content succeeded")
			}
			active, err := run.Admit(draft)
			if err != nil {
				t.Fatal(err)
			}
			waiting, err := active.Suspend(started.Add(2 * time.Second))
			if err != nil {
				t.Fatal(err)
			}
			if err := state.Suspend(ctx, waiting, draft.SegmentID, runtimeidentity.CommitID{}); err != nil {
				t.Fatal(err)
			}
			if err := state.Resume(ctx, draft.SessionID, run.ResumeDraft{RunID: draft.RunID, SegmentID: "seg_next"}, started.Add(3*time.Second)); err != nil {
				t.Fatal(err)
			}
			if found, err := state.ResultPublicationCommitted(ctx, draft.SessionID, draft.RunID, draft.SegmentID, commit.ResultPublication.ID, commit.ResultPublication.Digest); err == nil || found {
				t.Fatalf("stale Segment receipt = %t, %v", found, err)
			}
			if err := effects.CommitExecutionTree(ctx, update, []runs.EventCommit{commit}); err == nil {
				t.Fatal("stale Segment used a stored receipt to publish")
			}
			if found, err := state.ResultPublicationCommitted(ctx, draft.SessionID, draft.RunID, "seg_next", commit.ResultPublication.ID, commit.ResultPublication.Digest); err == nil || found {
				t.Fatalf("foreign Segment reused receipt: %t, %v", found, err)
			}
		})
	}
}
