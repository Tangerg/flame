package sqlite_test

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// TestSearchIndexFollowsAnItemThatStopsBeingSearchable covers the direction the
// index write used to skip. AppendItem is an upsert, so a message whose content
// is replaced by something with no searchable text keeps its row in
// history_items — and the search index must lose the text that row no longer
// contains, or SearchTranscript answers with content the transcript does not
// hold.
func TestSearchIndexFollowsAnItemThatStopsBeingSearchable(t *testing.T) {
	transcripts, _ := openTranscriptAndBlobs(t)
	ctx := t.Context()

	if err := transcripts.AppendItem(ctx,
		msgItem("s9", "a9", transcript.AgentMessage, "zzquerytoken deployment notes"),
	); err != nil {
		t.Fatal(err)
	}
	hits, err := transcripts.SearchTranscript(ctx, "zzquerytoken", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("indexed message = %d hits, %v; want 1", len(hits), err)
	}

	replaced := testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "s9", ID: "a9", RunID: "run-1", Kind: transcript.AgentMessage,
		OccurredAt: time.Unix(1, 0).UTC(),
		Content: []transcript.ContentBlock{
			{Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte{1, 2, 3}},
		},
	})
	if err := transcripts.AppendItem(ctx, replaced); err != nil {
		t.Fatal(err)
	}

	hits, err = transcripts.SearchTranscript(ctx, "zzquerytoken", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("search still returns %d hits for text the transcript no longer contains", len(hits))
	}
}
