package sqlite_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestTranscriptReadsRetainConflictingContentFactsForValidation(t *testing.T) {
	for _, test := range []struct{ name, content, want string }{
		{"text with media type", `{"kind":"text","text":"hello","mediaType":"image/png"}`, "text content cannot carry media"},
		{"text with media bytes", `{"kind":"text","text":"hello","data":"aGk="}`, "text content cannot carry media"},
		{"image with text", `{"kind":"image","mediaType":"image/png","data":"aGk=","text":"hidden"}`, "image content cannot carry text"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := sqlite.Open(t.Context(), ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			store := sqlite.NewTranscriptStore(db)
			item, err := transcript.NewUserMessage(transcript.ItemIdentity{
				SessionID: "ses_decode", RunID: "run_decode", ItemID: "item_decode", OccurredAt: time.Unix(1, 0).UTC(),
			}, []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.AppendItem(t.Context(), item); err != nil {
				t.Fatal(err)
			}
			payload := `{"status":"completed","kind":"userMessage","content":[` + test.content + `]}`
			if _, err := db.ExecContext(t.Context(), `UPDATE history_items SET payload = ? WHERE item_id = ?`, payload, item.ID()); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.Item(t.Context(), item.ID()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Item read error = %v, want %q", err, test.want)
			}
			if _, err := store.List(t.Context(), item.SessionID()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Session read error = %v, want %q", err, test.want)
			}
		})
	}
}
