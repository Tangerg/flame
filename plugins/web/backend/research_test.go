package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	rpc "github.com/Tangerg/flame/plugins/web/internal/contract"
	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/Tangerg/scope/tools/web"
)

func TestFreshFetchAndOfflineBrowseRetainTheirOwnSnapshotAcrossReopenAndEviction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.db")
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	content := strings.Repeat("知识", 5000)
	upstream := func(context.Context, *web.FetchRequest) (*web.FetchResponse, error) {
		calls++
		return &web.FetchResponse{Content: content, Format: web.FormatMarkdown}, nil
	}
	search := func(context.Context, *web.SearchRequest) (*web.SearchResponse, error) {
		return nil, errors.New("unused")
	}
	research, err := newResearch(store, upstream, search)
	if err != nil {
		t.Fatal(err)
	}
	request := web.FetchRequest{URL: "https://example.test/article"}
	first, err := research.fetchPage(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	content = "new source content"
	second, err := research.fetchPage(t.Context(), request)
	if err != nil || first.SnapshotID == second.SnapshotID || calls != 2 {
		t.Fatalf("fresh source query: %+v %v calls=%d", second, err, calls)
	}
	if err := store.db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()
	research.store = store
	continued, err := research.browse(t.Context(), rpc.BrowseInput{SnapshotID: first.SnapshotID, Offset: *first.NextOffset, Limit: capture.MaxPageRunes})
	if err != nil || first.Content+continued.Content != strings.Repeat("知识", 5000) || calls != 2 {
		t.Fatalf("offline snapshot continuation: %+v %v", continued, err)
	}
	latestID := ""
	for index := range capture.MaxDocuments {
		document, err := capture.NewDocument(capture.Content{URL: fmt.Sprintf("https://example.test/%d", index), Format: capture.Text, Text: "capture"})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.capture(t.Context(), document); err != nil {
			t.Fatal(err)
		}
		latestID = document.Snapshot().ID
	}
	if _, err := research.browse(t.Context(), rpc.BrowseInput{SnapshotID: first.SnapshotID, Limit: 1}); !errors.Is(err, capture.ErrNotFound) || calls != 2 {
		t.Fatalf("eviction silently fetched another version: %v calls=%d", err, calls)
	}
	if _, err := store.db.ExecContext(t.Context(), `UPDATE snapshots SET snapshot='{"id":"forged"}' WHERE id=?`, latestID); err != nil {
		t.Fatal(err)
	}
	if _, err := research.browse(t.Context(), rpc.BrowseInput{SnapshotID: latestID, Limit: 1}); !errors.Is(err, errStoredSnapshot) {
		t.Fatalf("corrupt cache became another snapshot: %v", err)
	}
}
