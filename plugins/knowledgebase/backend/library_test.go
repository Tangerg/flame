package main

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/knowledgebase/internal/contract"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
)

func TestImportPublishesItsOnlyReceiptAndNeverRepeatsAnUnconfirmedEmbedding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "knowledge.db")
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.db.Close() })
	var calls atomic.Int32
	var unknown atomic.Bool
	library, err := newLibrary(store, func(context.Context) (embed, error) {
		return func(_ context.Context, texts []string) (pluginsdk.TextEmbeddings, error) {
			calls.Add(1)
			if unknown.Load() {
				return pluginsdk.TextEmbeddings{}, errors.New("embedding response lost")
			}
			vectors := make([][]float32, len(texts))
			for i := range vectors {
				vectors[i] = []float32{1, 0}
			}
			return pluginsdk.TextEmbeddings{Space: "space-a", Vectors: vectors}, nil
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	input := rpc.ImportInput{Content: knowledge.Content{ID: "guide", Title: "Guide", Source: "docs/guide.md", Text: "A source-owned document."}}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	first, err := library.importDocument(t.Context(), contracts.Import.Name(), "original", input, admitted)
	if err != nil || first.Revision != 1 {
		t.Fatalf("import = %+v, %v", first, err)
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
	for range 2 {
		replayed, err := library.importDocument(t.Context(), contracts.Import.Name(), "original", input, admitted)
		if err != nil || replayed != first || calls.Load() != 1 {
			t.Fatalf("receipt replay changed ownership: %+v, calls=%d, %v", replayed, calls.Load(), err)
		}
	}
	changed := input
	changed.Content.Text = "Another intent."
	changedJSON, err := jsonvalue.Encode(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.importDocument(t.Context(), contracts.Import.Name(), "original", changed, changedJSON); !errors.Is(err, extension.ErrInvocationConflict) {
		t.Fatalf("changed intent = %v", err)
	}
	staleCalls := calls.Load()
	if _, err := library.importDocument(t.Context(), contracts.Import.Name(), "stale", input, admitted); !errors.Is(err, knowledge.ErrRevisionConflict) || calls.Load() != staleCalls {
		t.Fatalf("stale import dispatched embedding: calls=%d %v", calls.Load(), err)
	}
	if _, err := library.importDocument(t.Context(), contracts.Import.Name(), "stale", input, admitted); err == nil || calls.Load() != staleCalls {
		t.Fatalf("known refusal was re-executed: %v", err)
	}
	input.ExpectedRevision = 1
	admitted, err = jsonvalue.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	unknown.Store(true)
	if _, err := library.importDocument(t.Context(), contracts.Import.Name(), "unknown", input, admitted); err == nil {
		t.Fatal("lost embedding became success")
	}
	before := calls.Load()
	if _, err := library.importDocument(t.Context(), contracts.Import.Name(), "unknown", input, admitted); !errors.Is(err, pluginstorage.ErrCommandUnconfirmed) || calls.Load() != before {
		t.Fatalf("pending work repeated its external exchange: %v", err)
	}
	receipt, err := pluginstorage.ReadReceipt(t.Context(), reopened.db, pluginsdk.ReceiptRequest{ID: "unknown", Method: contracts.Import.Name(), InputDigest: admitted.Digest()})
	if err != nil || receipt.Found {
		t.Fatalf("uncertainty invented a receipt: %+v, %v", receipt, err)
	}
	if headers, err := library.list(t.Context()); err != nil || len(headers) != 1 || headers[0] != first {
		t.Fatalf("failed publication changed the document: %+v, %v", headers, err)
	}
	if _, err := reopened.db.ExecContext(t.Context(), `UPDATE documents SET snapshot='{"content":{}}'`); err != nil {
		t.Fatal(err)
	}
	if _, err := library.list(t.Context()); err == nil || classify(err) != "" {
		t.Fatalf("corruption became a business refusal: %v", err)
	}
}
