package persistence

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func TestTranscriptAppendCannotRewriteOrReopenDurableFacts(t *testing.T) {
	store, _ := openTranscriptAndBlobs(t)
	at := time.Unix(1, 0).UTC()
	identity := transcript.ItemIdentity{SessionID: "ses_1", RunID: "run_1", ItemID: "item_tool", OccurredAt: at}
	started, err := transcript.NewToolCall(identity, transcript.ToolInvocation{Name: "shell"}, tool.SafetyClassSafe)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := started.CompleteToolCall(transcript.ToolInvocation{Name: "shell"}, at, at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendItem(t.Context(), started); err != nil {
		t.Fatalf("append running ToolCall: %v", err)
	}
	if err := store.SettleItem(t.Context(), completed); err != nil {
		t.Fatalf("settle ToolCall: %v", err)
	}
	if err := store.AppendItem(t.Context(), completed.State()); err != nil {
		t.Fatalf("re-append identical settled ToolCall: %v", err)
	}
	if err := store.SettleItem(t.Context(), completed); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("second settlement error = %v, want ErrIdentityConflict", err)
	}
	if err := store.AppendItem(t.Context(), started); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("reopen ToolCall error = %v, want ErrIdentityConflict", err)
	}
	stored, found, err := store.Item(t.Context(), identity.ItemID)
	if err != nil || !found || !stored.Equal(completed.State()) {
		t.Fatalf("terminal Item changed: found=%t err=%v item=%+v", found, err, stored.Snapshot())
	}

	identity.ItemID = "item_message"
	original, err := transcript.NewUserMessage(identity, []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := transcript.NewUserMessage(identity, []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "changed"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendItem(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendItem(t.Context(), changed); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("rewrite message error = %v, want ErrIdentityConflict", err)
	}
	stored, found, err = store.Item(t.Context(), identity.ItemID)
	if err != nil || !found || !stored.Equal(original) {
		t.Fatalf("message changed: found=%t err=%v item=%+v", found, err, stored.Snapshot())
	}
}

// A running ToolCall reaches storage only at a park or child admission, so a
// settlement must also land when its running state never did, while a
// strict replacement of an Item that was never written stays an error.
func TestTranscriptSettlementInsertsANeverDurableRunningState(t *testing.T) {
	store, _ := openTranscriptAndBlobs(t)
	at := time.Unix(1, 0).UTC()
	identity := transcript.ItemIdentity{SessionID: "ses_1", RunID: "run_1", ItemID: "item_tool", OccurredAt: at}
	started, err := transcript.NewToolCall(identity, transcript.ToolInvocation{Name: "shell"}, tool.SafetyClassSafe)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := started.CompleteToolCall(transcript.ToolInvocation{Name: "shell"}, at, at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceItem(t.Context(), completed); err == nil {
		t.Fatal("ReplaceItem wrote an Item that was never durable")
	}
	if err := store.SettleItem(t.Context(), completed); err != nil {
		t.Fatalf("SettleItem: %v", err)
	}
	stored, found, err := store.Item(t.Context(), identity.ItemID)
	if err != nil || !found || !stored.Equal(completed.State()) {
		t.Fatalf("settled Item: found=%t err=%v item=%+v", found, err, stored.Snapshot())
	}
}
