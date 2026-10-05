package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/scope/core/chat"
)

func TestResultPublicationsFollowTheExactConversationRound(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runs, messages := sqlite.NewRunStore(db), sqlite.NewMessageStore(db)
	const sessionID, runID, segmentID = "ses_results", "run_results", "seg_open"
	if err := runs.Admit(t.Context(), runDraft(runID, sessionID)); err != nil {
		t.Fatal(err)
	}
	request := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "reused_call", Name: "inspect", Arguments: `{}`}))
	if _, err := messages.Write(t.Context(), sessionID, request); err != nil {
		t.Fatal(err)
	}
	first := chat.ToolResult{ID: "reused_call", Name: "inspect", Output: chat.NewTextToolOutput("first exact result")}
	cause := errors.New("publication transaction failed")
	err = sqlite.RunInTx(t.Context(), db, func(ctx context.Context) error {
		if err := runs.RecordResultPublication(ctx, sessionID, runID, segmentID, "pub_rollback", "digest_rollback", []chat.ToolResult{first}); err != nil {
			return err
		}
		return cause
	})
	if !errors.Is(err, cause) {
		t.Fatalf("rollback error = %v", err)
	}
	if got, err := runs.UnpublishedToolResults(t.Context(), sessionID, runID); err != nil || len(got) != 0 {
		t.Fatalf("failed transaction published results: count=%d err=%v", len(got), err)
	}
	if err := runs.RecordResultPublication(t.Context(), sessionID, runID, segmentID, "pub_first", "digest_first", []chat.ToolResult{first}); err != nil {
		t.Fatal(err)
	}
	got, err := runs.UnpublishedToolResults(t.Context(), sessionID, runID)
	if err != nil || !reflect.DeepEqual(got, []chat.ToolResult{first}) {
		t.Fatalf("pending exact results = %+v, err %v", got, err)
	}
	if _, err := messages.Write(t.Context(), sessionID, chat.NewToolMessage(first), request); err != nil {
		t.Fatal(err)
	}
	if got, err := runs.UnpublishedToolResults(t.Context(), sessionID, runID); err != nil || len(got) != 0 {
		t.Fatalf("prior round result reused by provider call ID: count=%d err=%v", len(got), err)
	}
	second := first
	second.Output = chat.NewTextToolOutput("second exact result")
	if err := runs.RecordResultPublication(t.Context(), sessionID, runID, segmentID, "pub_second", "digest_second", []chat.ToolResult{second}); err != nil {
		t.Fatal(err)
	}
	got, err = runs.UnpublishedToolResults(t.Context(), sessionID, runID)
	if err != nil || !reflect.DeepEqual(got, []chat.ToolResult{second}) {
		t.Fatalf("second round exact results = %+v, err %v", got, err)
	}
	if err := messages.Replace(t.Context(), sessionID, request); err != nil {
		t.Fatal(err)
	}
	if got, err := runs.UnpublishedToolResults(t.Context(), sessionID, runID); err != nil || len(got) != 0 {
		t.Fatalf("rewritten history adopted retired round results: count=%d err=%v", len(got), err)
	}
	committed, err := runs.ResultPublicationCommitted(t.Context(), sessionID, runID, segmentID, "pub_second", "digest_second")
	if err != nil || !committed {
		t.Fatalf("history rewrite removed immutable receipt: committed=%v err=%v", committed, err)
	}
	var retainedAnchors int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM result_publications WHERE source_message_seq IS NOT NULL`).Scan(&retainedAnchors); err != nil || retainedAnchors != 0 {
		t.Fatalf("deleted history retained receipt foreign keys: count=%d err=%v", retainedAnchors, err)
	}
}
