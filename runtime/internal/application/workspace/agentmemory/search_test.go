package agentmemory

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

type fakeItemSource struct {
	items         []domain.Item
	err           error
	cacheErr      error
	mutateUpdates bool
	updates       []domain.EmbeddingUpdate
}

func (f *fakeItemSource) SearchCorpus(context.Context, string) ([]domain.Item, error) {
	return slices.Clone(f.items), f.err
}

func (f *fakeItemSource) Items(context.Context, domain.Scope, string) ([]domain.Item, error) {
	return slices.Clone(f.items), f.err
}

func (f *fakeItemSource) SetEmbeddings(_ context.Context, updates []domain.EmbeddingUpdate) error {
	if f.mutateUpdates && len(updates) > 0 && len(updates[0].Vector) > 0 {
		updates[0].Vector[0] = 0
	}
	f.updates = append(f.updates, updates...)
	if f.cacheErr != nil {
		return f.cacheErr
	}
	for _, update := range updates {
		for index := range f.items {
			if f.items[index].ID != update.ItemID || domain.Digest(f.items[index].Content) != update.ContentDigest {
				continue
			}
			f.items[index].EmbeddingSpace = update.Space
			f.items[index].Embedding = slices.Clone(update.Vector)
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
	if len(got) != 1 || got[0].ID != testMemoryItemID('a') {
		t.Fatalf("keyword search = %+v, want just item a", got)
	}
}

func TestReadModelItemsProtectsExactActiveCatalog(t *testing.T) {
	valid := readModelItem(t, '1', domain.ScopeProject, "/repo", "valid fact")
	pending, err := domain.NewProposal(
		testMemoryItemID('2'), "/repo", "pending fact",
		time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	foreign := readModelItem(t, '3', domain.ScopeProject, "/other", "foreign fact")
	invalid := valid
	invalid.Content = " canonical violation "
	for _, test := range []struct {
		name  string
		items []domain.Item
	}{
		{name: "invalid item", items: []domain.Item{invalid}},
		{name: "foreign target", items: []domain.Item{foreign}},
		{name: "non-active item", items: []domain.Item{pending}},
		{name: "duplicate identity", items: []domain.Item{valid, valid}},
		{name: "duplicate content", items: []domain.Item{valid, readModelItem(t, '4', domain.ScopeProject, "/repo", "valid fact")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := mustNewReadModel(t, &fakeItemSource{items: test.items}, nil)
			if _, err := reader.Items(t.Context(), domain.ScopeProject, "/repo"); err == nil {
				t.Fatal("corrupt target catalog was accepted")
			}
		})
	}
}

func TestReadModelOwnsReturnedItemEmbeddings(t *testing.T) {
	item := readModelItem(t, '1', domain.ScopeProject, "/repo", "valid fact")
	item.EmbeddingSpace, item.Embedding = "fake", []float32{1, 0}
	store := &fakeItemSource{items: []domain.Item{item}}
	reader := mustNewReadModel(t, store, nil)

	listed, err := reader.Items(t.Context(), domain.ScopeProject, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	listed[0].Embedding[0] = 9
	if store.items[0].Embedding[0] != 1 {
		t.Fatalf("store embedding changed through returned Item: %v", store.items[0].Embedding)
	}

	searched, err := reader.Search(t.Context(), "/repo", "valid fact", 1)
	if err != nil {
		t.Fatal(err)
	}
	searched[0].Embedding[0] = 8
	if store.items[0].Embedding[0] != 1 {
		t.Fatalf("store embedding changed through search result: %v", store.items[0].Embedding)
	}
}

func TestReadModelSearchProtectsCombinedCatalog(t *testing.T) {
	projectItem := readModelItem(t, '1', domain.ScopeProject, "/repo", "project fact")
	userItem := readModelItem(t, '2', domain.ScopeUser, "", "user fact")
	reader := mustNewReadModel(t, &fakeItemSource{items: []domain.Item{projectItem, userItem}}, nil)
	if got, err := reader.Search(t.Context(), "/repo", "fact", 2); err != nil || len(got) != 2 {
		t.Fatalf("Search valid combined catalog = (%+v, %v)", got, err)
	}

	foreign := readModelItem(t, '3', domain.ScopeProject, "/other", "foreign fact")
	reader = mustNewReadModel(t, &fakeItemSource{items: []domain.Item{foreign}}, nil)
	if _, err := reader.Search(t.Context(), "/repo", "fact", 1); err == nil {
		t.Fatal("foreign project search item was accepted")
	}

	duplicateAcrossTargets := userItem
	duplicateAcrossTargets.ID = projectItem.ID
	reader = mustNewReadModel(t, &fakeItemSource{items: []domain.Item{projectItem, duplicateAcrossTargets}}, nil)
	if _, err := reader.Search(t.Context(), "/repo", "fact", 2); err == nil {
		t.Fatal("cross-target duplicate identity was accepted")
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
	if len(got) != 1 || got[0].ID != testMemoryItemID('a') {
		t.Fatalf("degraded search = %+v, want keyword hit a", got)
	}
}

func TestSearchFusesVectorMatchWithoutKeywordOverlap(t *testing.T) {
	// "b" shares no query terms but is the nearest vector — fusion must surface it.
	a := readModelItem(t, 'a', domain.ScopeProject, "/repo", "- unrelated note about tabs")
	a.EmbeddingSpace, a.Embedding = "fake", []float32{0, 1}
	b := readModelItem(t, 'b', domain.ScopeProject, "/repo", "- the build pipeline lives in ci")
	b.EmbeddingSpace, b.Embedding = "fake", []float32{1, 0}
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
		if item.ID == testMemoryItemID('b') {
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
	a.EmbeddingSpace, a.Embedding = "provider:old-space", []float32{1, 0}
	b := readModelItem(t, 'b', domain.ScopeProject, "/repo", "beta memory")
	b.EmbeddingSpace, b.Embedding = "provider:old-space", []float32{0, 1}
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
	if len(got) != 1 || got[0].ID != testMemoryItemID('b') {
		t.Fatalf("search after embedding-role change = %+v, want item b from the current vector space", got)
	}
	if len(store.updates) != 2 || store.updates[0].Space != "provider:new-space" {
		t.Fatalf("cache updates = %+v, want both items bound to the current space", store.updates)
	}
}

func TestSearchOwnsVectorsAcrossEmbeddingCacheWrite(t *testing.T) {
	a := readModelItem(t, 'a', domain.ScopeProject, "/repo", "alpha memory")
	b := readModelItem(t, 'b', domain.ScopeProject, "/repo", "beta memory")
	store := &fakeItemSource{items: items(a, b), mutateUpdates: true}
	resolve := func(context.Context) (Embedder, error) {
		return fakeEmbedder{vectors: map[string][]float32{
			"find target":  {1, 0},
			"alpha memory": {1, 0},
			"beta memory":  {0, 1},
		}}, nil
	}
	reader := mustNewReadModel(t, store, resolve)

	got, err := reader.Search(t.Context(), "/repo", "find target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("search after cache store mutated its update = %+v, want alpha", got)
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
