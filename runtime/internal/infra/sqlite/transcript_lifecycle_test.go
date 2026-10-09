package sqlite_test

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
	for _, item := range []transcript.Item{started, completed, completed} {
		if err := store.AppendItem(t.Context(), item); err != nil {
			t.Fatalf("append valid lifecycle: %v", err)
		}
	}
	if err := store.AppendItem(t.Context(), started); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("reopen ToolCall error = %v, want ErrIdentityConflict", err)
	}
	stored, found, err := store.Item(t.Context(), identity.ItemID)
	if err != nil || !found || !stored.Equal(completed) {
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
