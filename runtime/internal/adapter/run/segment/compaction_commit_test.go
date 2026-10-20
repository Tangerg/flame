package segment

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
	"github.com/Tangerg/scope/core/chat"
)

type compactionContextState struct{ sessions []string }

func (s *compactionContextState) ForgetSessionContext(id string) { s.sessions = append(s.sessions, id) }

func TestCompactionCommitsHistoryWatermarksAndItemTogether(t *testing.T) {
	for _, boundary := range []string{"success", "item failure", "commit acknowledgement lost"} {
		t.Run(boundary, func(t *testing.T) {
			ctx := t.Context()
			db, err := sqlite.Open(ctx, ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			state := sqlite.NewRunStore(db)
			items := sqlite.NewTranscriptStore(db)
			history := mustConversationStore(t, sqlite.NewMessageStore(db))
			at := time.Unix(20, 0).UTC()
			previous := testsupport.MustRestoreRun(run.Snapshot{
				ID: "run_previous", SessionID: "ses_1", State: run.Completed,
				CreatedAt: at, UpdatedAt: at, FinishedAt: at, MessageMark: run.MessageMarkAt(8),
			})
			if err := state.Restore(ctx, previous); err != nil {
				t.Fatal(err)
			}
			if err := state.Admit(ctx, testsupport.RunDraft(run.Draft{RunID: "run_active", SessionID: "ses_1", SegmentID: "seg_active", CreatedAt: at.Add(time.Second)})); err != nil {
				t.Fatal(err)
			}
			var original []chat.Message
			for range 4 {
				original = append(original, chat.NewUserMessage(chat.NewTextPart("question")), chat.NewAssistantMessage(chat.NewTextPart("answer")))
			}
			if err := history.Write(ctx, "ses_1", original...); err != nil {
				t.Fatal(err)
			}
			replacement := append([]chat.Message{chat.NewSystemMessage("summary")}, original[6:]...)
			proposal, err := conversation.NewCompaction(8, 6, 1, replacement)
			if err != nil {
				t.Fatal(err)
			}
			current, err := state.ListRuns(ctx, "ses_1")
			if err != nil {
				t.Fatal(err)
			}
			plan, err := runs.PlanConversationCompaction("ses_1", proposal, current)
			if err != nil {
				t.Fatal(err)
			}
			item, err := transcript.NewCompaction(transcript.ItemIdentity{SessionID: "ses_1", RunID: "run_active", ItemID: "item_compaction", OccurredAt: at.Add(2 * time.Second)}, "summary", 5)
			if err != nil {
				t.Fatal(err)
			}
			commit := runs.EventCommit{RunID: "run_active", SessionID: "ses_1", SegmentID: "seg_active", CommitID: testCommitID("run_commit_compaction"), Items: []transcript.Item{item}, Compaction: new(plan)}
			tx := func(ctx context.Context, apply func(context.Context) error) error {
				return sqlitebase.RunInTx(ctx, db, apply)
			}
			compactions, err := persistence.NewConversationCompactions(history, state, tx)
			if err != nil {
				t.Fatal(err)
			}
			contextState := new(compactionContextState)
			admit := tx
			if boundary == "commit acknowledgement lost" {
				admit = func(ctx context.Context, apply func(context.Context) error) error {
					if err := tx(ctx, apply); err != nil {
						return err
					}
					return errors.New("commit acknowledgement lost")
				}
			}
			effects := sqliteEffects(sqliteOpeningStores{transcript: items}, Config{State: state, Conversation: history, Compactions: compactions, ContextState: contextState, Tx: admit})
			if boundary == "item failure" {
				if _, err := db.ExecContext(ctx, `CREATE TRIGGER refuse_compaction_item BEFORE INSERT ON history_items BEGIN SELECT RAISE(ABORT, 'item write failed'); END`); err != nil {
					t.Fatal(err)
				}
				if err := effects.CommitEvent(ctx, commit); err == nil {
					t.Fatal("failed Item write reported success")
				}
				stored, err := history.Read(ctx, "ses_1")
				if err != nil || !reflect.DeepEqual(stored, original) {
					t.Fatalf("failed commit changed history: %v, %v", stored, err)
				}
				old, found, err := state.Run(ctx, previous.ID())
				if err != nil || !found || old.MessageMark() != previous.MessageMark() {
					t.Fatalf("failed commit changed watermark: %v, %v", old.MessageMark(), err)
				}
				if len(contextState.sessions) != 0 {
					t.Fatal("failed commit retired unchanged context authority")
				}
				if _, err := db.ExecContext(ctx, `DROP TRIGGER refuse_compaction_item`); err != nil {
					t.Fatal(err)
				}
			}
			if err := effects.CommitEvent(ctx, commit); err != nil {
				t.Fatal(err)
			}
			if err := effects.CommitEvent(ctx, commit); err != nil {
				t.Fatalf("original commit could not reconcile: %v", err)
			}
			stored, err := history.Read(ctx, "ses_1")
			if err != nil || !reflect.DeepEqual(stored, replacement) {
				t.Fatalf("committed history = %v, %v", stored, err)
			}
			old, found, err := state.Run(ctx, previous.ID())
			if err != nil || !found || old.MessageMark() != run.MessageMarkAt(3) {
				t.Fatalf("committed watermark = %v, %v", old.MessageMark(), err)
			}
			projected, err := items.List(ctx, "ses_1")
			if err != nil || len(projected) != 1 || !projected[0].Equal(item) {
				t.Fatalf("compaction Item = %v, %v", projected, err)
			}
			if len(contextState.sessions) == 0 || contextState.sessions[0] != "ses_1" {
				t.Fatal("committed compaction retained stale context authority")
			}
		})
	}
}
