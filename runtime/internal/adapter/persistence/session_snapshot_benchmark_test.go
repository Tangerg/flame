package persistence

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/qimu/sqlite"
	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// Measure the coherent storage read and application validation together. Network
// encoding and client rendering are separate costs; this is not a first-paint benchmark.
func BenchmarkSessionMaterialSnapshot(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("items_%d", count), func(b *testing.B) {
			db, err := openDatabase(b.Context(), filepath.Join(b.TempDir(), "runtime.db"))
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = db.Close() })
			tx := func(ctx context.Context, fn func(context.Context) error) error {
				return sqlite.RunInTx(ctx, db, fn)
			}
			messages := newMessageStore(db)
			runs := newRunStore(db)
			history, err := runsapp.NewConversationHistory(mustConversationStore(b, messages))
			if err != nil {
				b.Fatal(err)
			}
			sessions := newSessionStore(db)
			items := newTranscriptStore(db)
			stores, err := NewSessionStores(SessionStoresConfig{
				CommandAcceptances: newCommandAcceptanceStore(db),
				Sessions:           sessions, Transcript: items, Runs: runs, History: history,
				Interrupts:          newInterruptStore(db),
				ExecutorCheckpoints: newExecutorCheckpointStore(db),
				Plan:                newPlanStore(db), ApprovalRules: newApprovalRuleStore(db),
				ApprovalModes: newModeStore(db), ToolResults: newToolResultStore(db),
				ChildRunStarts: newChildRunStartReservationStore(db), Goals: newGoalStore(db),
				GoalRuns: mustGoalRuns(b, newGoalStore(db)), Tx: tx,
				ReadSnapshot: func(ctx context.Context, read func(context.Context) error) error {
					return sqlite.RunReadSnapshot(ctx, db, read)
				},
			})
			if err != nil {
				b.Fatal(err)
			}
			now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
			text := strings.Repeat("x", 1024)
			err = tx(b.Context(), func(ctx context.Context) error {
				if err := sessions.Insert(ctx, testsupport.MustRestoreSession(session.Snapshot{
					ID: "ses_bench", Workspace: testsupport.MustWorkspace("/workspace"), Title: "benchmark",
					CreatedAt: now, UpdatedAt: now, Revision: 1,
				})); err != nil {
					return err
				}
				for index := range count {
					runID := fmt.Sprintf("run_%d", index/20)
					if index%20 == 0 {
						if err := runs.Restore(ctx, testsupport.MustRestoreRun(run.Snapshot{
							ID: runID, SessionID: "ses_bench", State: run.Completed,
							CreatedAt: now, UpdatedAt: now, FinishedAt: now,
						})); err != nil {
							return err
						}
					}
					if err := items.AppendItem(ctx, testsupport.MustRestoreItem(testsupport.ItemInput{
						ID: fmt.Sprintf("item_%d", index), SessionID: "ses_bench", RunID: runID,
						OccurredAt: now.Add(time.Duration(index) * time.Millisecond), Status: transcript.ItemCompleted,
						Kind: transcript.AgentMessage, MessagePhase: transcript.MessageCommentary,
						Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: text}},
					})); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(count * len(text)))
			for b.Loop() {
				snapshot, err := stores.ReadMaterialSnapshot(b.Context(), "ses_bench")
				if err != nil {
					b.Fatal(err)
				}
				if len(snapshot.Items) != count {
					b.Fatalf("items = %d, want %d", len(snapshot.Items), count)
				}
			}
		})
	}
}
