package agentmemory

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

type fakeItemSource struct {
	items    []domain.Item
	err      error
	cacheErr error
	updates  []domain.EmbeddingUpdate
}

func (f *fakeItemSource) SearchCorpus(context.Context, string) ([]domain.Item, error) {
	return cloneMemoryItems(f.items), f.err
}

func (f *fakeItemSource) Items(context.Context, domain.Scope, string) ([]domain.Item, error) {
	return cloneMemoryItems(f.items), f.err
}

func (f *fakeItemSource) SetEmbeddings(_ context.Context, updates []domain.EmbeddingUpdate) error {
	for _, update := range updates {
		f.updates = append(f.updates, update.Clone())
	}
	if f.cacheErr != nil {
		return f.cacheErr
	}
	for _, update := range updates {
		for index := range f.items {
			if f.items[index].ID() != update.ItemID || domain.Digest(f.items[index].Content()) != update.ContentDigest {
				continue
			}
			cached, err := f.items[index].WithEmbedding(update)
			if err != nil {
				return err
			}
			f.items[index] = cached
		}
	}
	return nil
}

type fakeEmbedder struct {
	id      string
	vectors map[string][]float32
	err     error
}

type pointerEmbedder struct{ id string }

func (p *pointerEmbedder) ID() string { return p.id }

func (*pointerEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, nil
}

func mustNewReadModel(
	t *testing.T,
	store ReadStore,
	resolve func(context.Context) (Embedder, error),
) *ReadModel {
	t.Helper()
	reader, err := NewReadModel(store, resolve)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func readModelItem(t *testing.T, digit byte, scope domain.Scope, project, content string) domain.Item {
	t.Helper()
	item, err := domain.NewUserItem(
		testMemoryItemID(digit), scope, project, content,
		time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func (f fakeEmbedder) ID() string {
	if f.id != "" {
		return f.id
	}
	return "fake"
}

func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = f.vectors[text]
	}
	return out, nil
}

func items(specs ...domain.Item) []domain.Item { return specs }

func TestSearchKeywordOnlyWhenNoEmbedder(t *testing.T) {
	store := &fakeItemSource{items: items(
		readModelItem(t, 'a', domain.ScopeProject, "/repo", "- run make test to build"),
		readModelItem(t, 'b', domain.ScopeProject, "/repo", "- prefer tabs over spaces"),
		readModelItem(t, 'c', domain.ScopeProject, "/repo", "- deploy with kubectl apply"),
	)}
	s := mustNewReadModel(t, store, nil)
	got, err := s.Search(context.Background(), "/repo", "how do we run tests", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID() != testMemoryItemID('a') {
		t.Fatalf("keyword search = %+v, want just item a", got)
	}
}

func TestSearchDegradesWhenEmbedderFails(t *testing.T) {
	store := &fakeItemSource{items: items(readModelItem(t, 'a', domain.ScopeProject, "/repo", "- run make test"))}
	resolve := func(context.Context) (Embedder, error) { return fakeEmbedder{err: errors.New("no model")}, nil }
	s := mustNewReadModel(t, store, resolve)
	got, err := s.Search(context.Background(), "/repo", "run the tests", 5)
	if err != nil {
		t.Fatalf("embed failure must not fail the search: %v", err)
	}
	if len(got) != 1 || got[0].ID() != testMemoryItemID('a') {
		t.Fatalf("degraded search = %+v, want keyword hit a", got)
	}
}

func TestSearchFusesVectorMatchWithoutKeywordOverlap(t *testing.T) {
	// "b" shares no query terms but is the nearest vector — fusion must surface it.
	a := readModelItem(t, 'a', domain.ScopeProject, "/repo", "- unrelated note about tabs")
	a = withEmbedding(t, a, "fake", []float32{0, 1})
	b := readModelItem(t, 'b', domain.ScopeProject, "/repo", "- the build pipeline lives in ci")
	b = withEmbedding(t, b, "fake", []float32{1, 0})
	store := &fakeItemSource{items: items(a, b)}
	resolve := func(context.Context) (Embedder, error) {
		return fakeEmbedder{vectors: map[string][]float32{"where is the pipeline": {1, 0}}}, nil
	}
	s := mustNewReadModel(t, store, resolve)
	got, err := s.Search(context.Background(), "/repo", "where is the pipeline", 2)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range got {
		if item.ID() == testMemoryItemID('b') {
			found = true
		}
	}
	if !found {
		t.Fatalf("vector match b not surfaced: %+v", got)
	}
}

func TestSearchDoesNotReuseCorpusVectorsFromAnotherEmbeddingSpace(t *testing.T) {
	// The persisted vectors were produced by the previous role and rank a first.
	// The current role gives the same-dimensional space different semantics: b
	// is now the nearest item. Reusing the unlabelled cache silently returns the
	// wrong memory instead of refreshing it or degrading to keyword ranking.
	a := readModelItem(t, 'a', domain.ScopeProject, "/repo", "alpha memory")
	a = withEmbedding(t, a, "provider:old-space", []float32{1, 0})
	b := readModelItem(t, 'b', domain.ScopeProject, "/repo", "beta memory")
	b = withEmbedding(t, b, "provider:old-space", []float32{0, 1})
	store := &fakeItemSource{cacheErr: errors.New("cache write lost"), items: items(a, b)}
	resolve := func(context.Context) (Embedder, error) {
		return fakeEmbedder{
			id: "provider:new-space",
			vectors: map[string][]float32{
				"find the target": {1, 0},
				"alpha memory":    {0, 1},
				"beta memory":     {1, 0},
			},
		}, nil
	}
	s := mustNewReadModel(t, store, resolve)
	got, err := s.Search(t.Context(), "/repo", "find the target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID() != testMemoryItemID('b') {
		t.Fatalf("search after embedding-role change = %+v, want item b from the current vector space", got)
	}
	if len(store.updates) != 2 || store.updates[0].Space != "provider:new-space" {
		t.Fatalf("cache updates = %+v, want both items bound to the current space", store.updates)
	}
}

func TestSearchIsolatesProviderVectorsAndReturnedItems(t *testing.T) {
	a := readModelItem(t, 'a', domain.ScopeProject, "/repo", "alpha memory")
	b := readModelItem(t, 'b', domain.ScopeProject, "/repo", "beta memory")
	store := &fakeItemSource{items: items(a, b)}
	vectors := map[string][]float32{
		"find target": {1, 0}, "alpha memory": {1, 0}, "beta memory": {0, 1},
	}
	resolve := func(context.Context) (Embedder, error) { return fakeEmbedder{vectors: vectors}, nil }
	reader := mustNewReadModel(t, store, resolve)
	got, err := reader.Search(t.Context(), "/repo", "find target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID() != a.ID() {
		t.Fatalf("search = %+v, want alpha", got)
	}
	vectors["alpha memory"][0] = 9
	if _, vector, _ := got[0].Embedding(); vector[0] != 1 || store.updates[0].Vector[0] != 1 {
		t.Fatal("provider reuse changed the cached embedding")
	}
}

func TestSearchEmptyCorpus(t *testing.T) {
	s := mustNewReadModel(t, &fakeItemSource{}, nil)
	got, err := s.Search(context.Background(), "/repo", "anything", 5)
	if err != nil || got != nil {
		t.Fatalf("empty corpus search = (%+v, %v)", got, err)
	}
}

func TestSearchDegradesWhenResolverReturnsTypedNilEmbedder(t *testing.T) {
	store := &fakeItemSource{items: items(readModelItem(t, 'a', domain.ScopeProject, "/repo", "run make test"))}
	resolve := func(context.Context) (Embedder, error) {
		var embedder *pointerEmbedder
		return embedder, nil
	}
	reader := mustNewReadModel(t, store, resolve)

	got, err := reader.Search(t.Context(), "/repo", "make test", 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("typed-nil embedder fallback = (%+v, %v), want keyword result", got, err)
	}
}

func TestNewReadModelRejectsTypedNilStore(t *testing.T) {
	var store *fakeItemSource
	if reader, err := NewReadModel(store, nil); err == nil || reader != nil {
		t.Fatalf("NewReadModel typed-nil store = (%v, %v), want invalid construction", reader, err)
	}
}

type failingCorpusEmbedder struct{ fakeEmbedder }

func (f failingCorpusEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 1 && texts[0] == "make test" {
		return f.fakeEmbedder.Embed(ctx, texts)
	}
	return nil, errors.New("corpus embedding unavailable")
}

func TestSearchDegradationReportsExternalFailures(t *testing.T) {
	for _, stage := range []string{"resolve", "query", "corpus", "cache", "unconfigured"} {
		t.Run(stage, func(t *testing.T) {
			var diagnostics bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			store := &fakeItemSource{items: []domain.Item{readModelItem(t, 'a', domain.ScopeProject, "/repo", "run make test")}}
			embedding := fakeEmbedder{vectors: map[string][]float32{"make test": {1, 0}, "run make test": {1, 0}}}
			var wantError string
			resolve := func(context.Context) (Embedder, error) {
				switch stage {
				case "resolve":
					wantError = "embedding resolution unavailable"
					return nil, errors.New(wantError)
				case "query":
					wantError = "query embedding unavailable"
					embedding.err = errors.New(wantError)
				case "corpus":
					wantError = "corpus embedding unavailable"
					return failingCorpusEmbedder{embedding}, nil
				case "cache":
					wantError = "embedding cache unavailable"
					store.cacheErr = errors.New(wantError)
				case "unconfigured":
					return nil, nil
				}
				return embedding, nil
			}
			reader := mustNewReadModel(t, store, resolve)
			got, err := reader.Search(t.Context(), "/repo", "make test", 1)
			if err != nil || len(got) != 1 || got[0].ID() != testMemoryItemID('a') {
				t.Fatalf("degraded search = (%+v, %v), want keyword hit", got, err)
			}
			if output := diagnostics.String(); wantError == "" && output != "" || wantError != "" && !strings.Contains(output, wantError) {
				t.Fatalf("diagnostic = %q, want failure %q", output, wantError)
			}
		})
	}
}

func withEmbedding(t *testing.T, item domain.Item, space string, vector []float32) domain.Item {
	t.Helper()
	update, err := domain.NewEmbeddingUpdate(item, space, vector)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := item.WithEmbedding(update)
	if err != nil {
		t.Fatal(err)
	}
	return cached
}

// TestReadModelItemsOrdersTiesLikeCuration proves the injected catalog and the
// curation input share one order, so equally recent items cannot swap places
// depending on how storage happened to return them.
func TestReadModelItemsOrdersTiesLikeCuration(t *testing.T) {
	first := readModelItem(t, '1', domain.ScopeProject, "/repo", "first")
	second := readModelItem(t, '2', domain.ScopeProject, "/repo", "second")
	store := &fakeItemSource{items: []domain.Item{first, second}}
	model, err := NewReadModel(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	items, err := model.Items(t.Context(), domain.ScopeProject, "/repo")
	if err != nil || len(items) != 2 || items[0].ID() != second.ID() || items[1].ID() != first.ID() {
		t.Fatalf("read model item order = (%+v, %v), want the identity tie-break", items, err)
	}
}
