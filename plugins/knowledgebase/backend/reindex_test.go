package main

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/knowledgebase/internal/contract"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/sqlite"
)

func TestStoredTextRemainsReadableAndReindexUsesOneDurablePublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "knowledge.db")
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.db.Close() })
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	space, calls := "space-a", 0
	var embeddingErr error
	var duringEmbedding func()
	var captured []string
	library, _ := newLibrary(store, func(context.Context) (embed, error) {
		return func(_ context.Context, texts []string) (pluginsdk.TextEmbeddings, error) {
			calls++
			captured = texts
			if duringEmbedding != nil {
				duringEmbedding()
			}
			vectors := make([][]float32, len(texts))
			for i := range vectors {
				vectors[i] = []float32{1, 0}
			}
			return pluginsdk.TextEmbeddings{Space: space, Vectors: vectors}, embeddingErr
		}, nil
	})
	content := knowledge.Content{ID: "guide", Title: "Guide", Source: "docs/guide", Text: strings.Repeat("正文🙂", 400)}
	input := rpc.ImportInput{Content: content}
	admitted, _ := jsonvalue.Encode(input)
	original, err := library.importDocument(t.Context(), contracts.Import.Name(), "import", input, admitted)
	if err != nil {
		t.Fatal(err)
	}
	space = "space-b"
	read, err := library.read(t.Context(), rpc.ReadInput{ID: content.ID})
	if err != nil || read.Text != content.Text || read.Document != original || calls != 1 {
		t.Fatalf("reading original text required a matching embedding role: %+v calls=%d %v", read, calls, err)
	}
	if _, err := library.search(t.Context(), rpc.SearchInput{Query: "正文", Limit: 1}); !errors.Is(err, knowledge.ErrEmbeddingSpace) {
		t.Fatalf("changed space was searched as if compatible: %v", err)
	}
	reindex := rpc.DocumentInput{ID: content.ID, ExpectedRevision: original.Revision}
	reindexJSON, _ := jsonvalue.Encode(reindex)
	rebuilt, err := library.reindexDocument(t.Context(), contracts.Reindex.Name(), "reindex", reindex, reindexJSON)
	if err != nil || rebuilt.Revision != original.Revision+1 || rebuilt.Space != space || strings.Join(captured, "") != content.Text {
		t.Fatalf("reindex did not use the captured stored text: %+v %v", rebuilt, err)
	}
	if matches, err := library.search(t.Context(), rpc.SearchInput{Query: "正文", Limit: 1}); err != nil || len(matches) != 1 || matches[0].Document != rebuilt {
		t.Fatalf("reindex did not restore retrieval: %+v %v", matches, err)
	}
	before := calls
	if _, err := library.reindexDocument(t.Context(), contracts.Reindex.Name(), "stale", reindex, reindexJSON); !errors.Is(err, knowledge.ErrRevisionConflict) || calls != before {
		t.Fatalf("stale reindex dispatched embedding: calls=%d %v", calls, err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.db.Close() })
	library.store = reopened
	read, err = library.read(t.Context(), rpc.ReadInput{ID: content.ID})
	if err != nil || read.Text != content.Text || read.Document != rebuilt {
		t.Fatalf("reindex changed the original content: %+v %v", read, err)
	}
	if replay, err := library.reindexDocument(t.Context(), contracts.Reindex.Name(), "reindex", reindex, reindexJSON); err != nil || !reflect.DeepEqual(replay, rebuilt) || calls != before {
		t.Fatalf("restart repeated reindex: %+v calls=%d %v", replay, calls, err)
	}
	current := rpc.DocumentInput{ID: content.ID, ExpectedRevision: rebuilt.Revision}
	currentJSON, _ := jsonvalue.Encode(current)
	duringEmbedding = func() {
		duringEmbedding = nil
		changed := content
		changed.Text = "a concurrent human edit"
		draft, _ := knowledge.NewDraft(changed)
		indexed, _ := draft.Index(space, [][]float32{{1, 0}})
		if err := sqlite.RunInTx(t.Context(), reopened.db, func(ctx context.Context) error {
			_, err := reopened.replace(ctx, content.ID, rebuilt.Revision, indexed)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := library.reindexDocument(t.Context(), contracts.Reindex.Name(), "racing", current, currentJSON); !errors.Is(err, knowledge.ErrRevisionConflict) {
		t.Fatalf("reindex overwrote a later human revision: %v", err)
	}
	read, err = library.read(t.Context(), rpc.ReadInput{ID: content.ID})
	if err != nil || read.Text != "a concurrent human edit" || read.Document.Revision != rebuilt.Revision+1 {
		t.Fatalf("publication lost the human edit: %+v %v", read, err)
	}
	current.ExpectedRevision = read.Document.Revision
	currentJSON, _ = jsonvalue.Encode(current)
	embeddingErr = errors.New("embedding response lost")
	if _, err := library.reindexDocument(t.Context(), contracts.Reindex.Name(), "unknown", current, currentJSON); !errors.Is(err, embeddingErr) {
		t.Fatal(err)
	}
	before = calls
	if _, err := library.reindexDocument(t.Context(), contracts.Reindex.Name(), "unknown", current, currentJSON); !errors.Is(err, pluginstorage.ErrCommandUnconfirmed) || calls != before {
		t.Fatalf("unknown reindex dispatched again: calls=%d %v", calls, err)
	}
}
