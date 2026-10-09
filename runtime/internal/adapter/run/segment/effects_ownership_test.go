package segment

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestCommitEventRollsBackAllProjectionsWhenProgressIsStale(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	startedAt := time.Unix(1, 0).UTC()
	at := startedAt.Add(time.Second)
	draft := run.Draft{
		RunID: "run_progress", SessionID: "ses_progress", SegmentID: "seg_progress",
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: startedAt,
	}
	state := sqlite.NewRunStore(db)
	if err := state.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	current := testsupport.MustProgressedRun(draft, testsupport.MustRunMetrics(testsupport.RunMetricsInput{Steps: 2}), 20, at)
	if err := state.UpdateProgress(t.Context(), current); err != nil {
		t.Fatal(err)
	}
	history := sqlite.NewTranscriptStore(db)
	messages := sqlite.NewMessageStore(db)
	conversation := mustConversationStore(t, messages)
	effects := mustNewEffects(Config{
		Transcript: history, Conversation: conversation, State: state, RunProgress: state,
		Tx: func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	commit := runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, SegmentID: draft.SegmentID,
		CommitID: testCommitID("run_commit_stale_progress"),
		Items: []transcript.Item{testsupport.MustRestoreItem(testsupport.ItemInput{
			SessionID: draft.SessionID, RunID: draft.RunID, ID: "item_stale", Kind: transcript.AgentMessage,
			Status: transcript.ItemCompleted, OccurredAt: at,
			Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "stale answer"}},
		})},
		ConversationMessages: []chat.Message{chat.NewAssistantMessage(chat.NewTextPart("stale answer"))},
		Progress:             new(testsupport.MustProgressedRun(draft, testsupport.MustRunMetrics(testsupport.RunMetricsInput{Steps: 1}), 10, at)),
	}
	if err := effects.CommitEvent(t.Context(), commit); err == nil {
		t.Fatal("stale event committed its projections")
	}
	stored, found, err := state.Run(t.Context(), draft.RunID)
	if err != nil || !found || !stored.Equal(current) {
		t.Fatalf("Run changed after rejected progress: found=%t err=%v", found, err)
	}
	items, err := history.List(t.Context(), draft.SessionID)
	if err != nil || len(items) != 0 {
		t.Fatalf("rejected event left transcript Items: %v, %v", items, err)
	}
	count, err := conversation.Count(t.Context(), draft.SessionID)
	if err != nil || count != 0 {
		t.Fatalf("rejected event left conversation messages: count=%d err=%v", count, err)
	}
	committed, err := state.RunCommitCommitted(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, commit.CommitID)
	if err != nil || committed {
		t.Fatalf("rejected event left a success receipt: committed=%t err=%v", committed, err)
	}
}
