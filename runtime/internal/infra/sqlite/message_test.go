package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history/storetest"
)

// TestMessageStoreSatisfiesTheHistoryStoreContract runs Scope's own conformance
// suite against the durable store: it fixes which history capabilities this
// store publishes, and proves that a canceled context or an invalid
// conversation identity is refused before the database is touched. The store
// implements the contract Scope defines, so Scope is what decides whether it
// does.
func TestMessageStoreSatisfiesTheHistoryStoreContract(t *testing.T) {
	storetest.Run(t, new(sqlite.MessageStore), storetest.Capabilities{
		Reader: true, Writer: true, Clearer: true,
	})
}

func TestMessageStorePreservesOpaqueProviderCallIDs(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMessageStore(db)
	id := " provider\u200b" + strings.Repeat("界", 513) + "\n"
	want := []chat.Message{
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: id, Name: "inspect", Arguments: `{}`})),
		chat.NewToolMessage(chat.ToolResult{ID: id, Name: "inspect", Output: chat.NewTextToolOutput("contents")}),
	}
	if _, err := store.Write(t.Context(), "conv", want...); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(t.Context(), "conv")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("stored messages changed a provider correlation ID")
	}
}

// TestMessageStore_ReplaceIsTransactional pins the retention-safety fix:
// Replace sets a conversation's history to exactly the given messages in one
// transaction (DELETE + INSERT), so truncate/compaction can't leave the
// conversation wiped if the rewrite fails. Append (Write) accumulates; Replace
// overwrites; empty clears.
func TestMessageStore_ReplaceIsTransactional(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMessageStore(db)
	ctx := context.Background()

	if _, writeErr := store.Write(ctx, "conv",
		chat.NewUserMessage(chat.NewTextPart("one")), chat.NewUserMessage(chat.NewTextPart("two")), chat.NewUserMessage(chat.NewTextPart("three"))); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	// Replace the history with just the first message — exact overwrite.
	if replaceErr := store.Replace(ctx, "conv", chat.NewUserMessage(chat.NewTextPart("one"))); replaceErr != nil {
		t.Fatalf("Replace: %v", replaceErr)
	}
	got, err := store.Read(ctx, "conv")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("after Replace len = %d, want 1 (overwrite, not append)", len(got))
	}

	// Replace with nothing clears it.
	if err := store.Replace(ctx, "conv"); err != nil {
		t.Fatalf("Replace empty: %v", err)
	}
	if got, _ := store.Read(ctx, "conv"); len(got) != 0 {
		t.Fatalf("after empty Replace len = %d, want 0", len(got))
	}
}

// TestMessageStore_CountMatchesReadLength pins the Counter capability: Count
// returns the stored message count via COUNT(*) — equal to len(Read) — so a
// watermark read doesn't load and unmarshal the whole history. Unknown
// conversation is 0, not an error.
func TestMessageStore_CountMatchesReadLength(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMessageStore(db)
	ctx := context.Background()

	if n, countErr := store.Count(ctx, "conv"); countErr != nil || n != 0 {
		t.Fatalf("Count of empty = (%d, %v), want (0, nil)", n, countErr)
	}

	if _, writeErr := store.Write(ctx, "conv",
		chat.NewUserMessage(chat.NewTextPart("one")), chat.NewUserMessage(chat.NewTextPart("two")), chat.NewUserMessage(chat.NewTextPart("three"))); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}
	got, err := store.Read(ctx, "conv")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	n, err := store.Count(ctx, "conv")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != len(got) || n != 3 {
		t.Fatalf("Count = %d, len(Read) = %d, want both 3", n, len(got))
	}
}

func TestMessageStoreReadRejectsMalformedRows(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMessageStore(db)
	if _, err := store.Write(t.Context(), "conv", chat.NewUserMessage(chat.NewTextPart("valid"))); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO messages(conversation_id, message) VALUES (?, ?)`,
		"conv", `{"role":"user","parts":"not-an-array"}`,
	); err != nil {
		t.Fatal(err)
	}

	if messages, err := store.Read(t.Context(), "conv"); err == nil {
		t.Fatalf("read silently returned %d messages after skipping a malformed durable row", len(messages))
	}
}

// TestMessageStore_ReplaceRollsBackAsOneStep pins what makes Replace safe for
// retention: its DELETE and INSERT belong to one transaction, and that
// transaction is the caller's when there is one. A rewrite that fails leaves
// the prior history whole rather than a wiped conversation.
func TestMessageStore_ReplaceRollsBackAsOneStep(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMessageStore(db)
	if _, err := store.Write(t.Context(), "conv",
		chat.NewUserMessage(chat.NewTextPart("one")), chat.NewUserMessage(chat.NewTextPart("two"))); err != nil {
		t.Fatal(err)
	}

	abandoned := errors.New("rewrite abandoned")
	if err := sqlite.RunInTx(t.Context(), db, func(ctx context.Context) error {
		if replaceErr := store.Replace(ctx, "conv", chat.NewUserMessage(chat.NewTextPart("replacement"))); replaceErr != nil {
			return replaceErr
		}
		return abandoned
	}); !errors.Is(err, abandoned) {
		t.Fatalf("transaction error = %v, want the abandoned rewrite", err)
	}

	messages, err := store.Read(t.Context(), "conv")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("history after an abandoned rewrite = %d messages, want the original 2", len(messages))
	}
}

// TestMessageStoreTruncateKeepsAPrefixInPlace: retention removes the tail by
// position. Reading the history back to re-install a prefix would rewrite every
// kept message and would drop anything appended between the two halves, so the
// deletion has to be the whole operation — and it must not reach another
// conversation's rows.
func TestMessageStoreTruncateKeepsAPrefixInPlace(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMessageStore(db)
	ctx := t.Context()

	for _, text := range []string{"one", "two", "three"} {
		if _, err := store.Write(ctx, "conv", chat.NewUserMessage(chat.NewTextPart(text))); err != nil {
			t.Fatalf("Write %s: %v", text, err)
		}
	}
	if _, err := store.Write(ctx, "other", chat.NewUserMessage(chat.NewTextPart("keep"))); err != nil {
		t.Fatalf("Write other: %v", err)
	}

	if err := store.Truncate(ctx, "conv", 5); err != nil {
		t.Fatalf("Truncate beyond the end: %v", err)
	}
	if count, _ := store.Count(ctx, "conv"); count != 3 {
		t.Fatalf("count after keeping more than exist = %d, want 3", count)
	}

	if err := store.Truncate(ctx, "conv", 2); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	kept, err := store.Read(ctx, "conv")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(kept) != 2 || kept[0].Parts[0].Text != "one" || kept[1].Parts[0].Text != "two" {
		t.Fatalf("kept = %+v, want the first two messages", kept)
	}
	if count, _ := store.Count(ctx, "other"); count != 1 {
		t.Fatal("truncation reached another conversation")
	}

	// A later append continues after the kept prefix rather than reusing its
	// coordinates, so a watermark taken now still means what it says.
	if _, err := store.Write(ctx, "conv", chat.NewUserMessage(chat.NewTextPart("four"))); err != nil {
		t.Fatalf("Write after truncation: %v", err)
	}
	after, _ := store.Read(ctx, "conv")
	if len(after) != 3 || after[2].Parts[0].Text != "four" {
		t.Fatalf("after append = %+v, want the prefix plus the new message", after)
	}

	if err := store.Truncate(ctx, "conv", 0); err != nil {
		t.Fatalf("Truncate to empty: %v", err)
	}
	if count, _ := store.Count(ctx, "conv"); count != 0 {
		t.Fatalf("count after truncating to zero = %d, want 0", count)
	}
	if err := store.Truncate(ctx, "conv", -1); err == nil {
		t.Fatal("Truncate accepted a negative keep count")
	}
}
