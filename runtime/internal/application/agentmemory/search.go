package agentmemory

import (
	"context"

	domain "github.com/Tangerg/flame/runtime/internal/domain/agentmemory"
)

// SearchStore supplies the active memory corpus visible from one project
// context: exact-project items plus user-scoped items. It also owns the derived
// embedding cache. Cache writes are conditional on the exact content digest, so
// a late model response cannot overwrite an item edited while embedding was in
// flight.
type SearchStore interface {
	SearchCorpus(ctx context.Context, project string) ([]domain.Item, error)
	SetEmbeddings(ctx context.Context, updates []domain.EmbeddingUpdate) error
}

// Embedder supplies an optional semantic signal. Embedding remains best-effort:
// keyword ranking is still useful when the model is absent or unhealthy.
type Embedder interface {
	// ID identifies the exact non-secret client configuration that selects the
	// vector coordinate system, including any custom endpoint identity.
	ID() string
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// Searcher coordinates corpus I/O and optional embedding, then delegates pure
// ranking to the agent-memory domain.
type Searcher struct {
	store           SearchStore
	resolveEmbedder func(context.Context) (Embedder, error)
}

// NewSearcher constructs the search use case. A nil resolver selects
// keyword-only search.
func NewSearcher(store SearchStore, resolveEmbedder func(context.Context) (Embedder, error)) *Searcher {
	return &Searcher{store: store, resolveEmbedder: resolveEmbedder}
}

// Search returns up to topK relevant project- and user-scoped memory items for
// one project context. Ranking happens over the combined corpus so neither
// partition receives a separate top-k quota or query embedding.
func (s *Searcher) Search(ctx context.Context, project, query string, topK int) ([]domain.Item, error) {
	if s == nil || s.store == nil || topK <= 0 {
		return nil, nil
	}
	items, err := s.store.SearchCorpus(ctx, project)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	semantic, ok := s.resolveSemanticQuery(ctx, query)
	if !ok {
		return domain.Rank(query, nil, items, topK), nil
	}
	s.refreshEmbeddings(ctx, semantic, items)
	return domain.Rank(query, semantic.queryVector, items, topK), nil
}

type semanticQuery struct {
	embedder    Embedder
	space       string
	queryVector []float32
}

func (s *Searcher) resolveSemanticQuery(ctx context.Context, query string) (semanticQuery, bool) {
	if s.resolveEmbedder == nil {
		return semanticQuery{}, false
	}
	embedder, err := s.resolveEmbedder(ctx)
	if err != nil || embedder == nil {
		return semanticQuery{}, false
	}
	space := embedder.ID()
	if space == "" {
		return semanticQuery{}, false
	}
	queryVectors, err := embedder.Embed(ctx, []string{query})
	if err != nil || len(queryVectors) != 1 || !usableVector(queryVectors[0], 0) {
		return semanticQuery{}, false
	}
	return semanticQuery{
		embedder:    embedder,
		space:       space,
		queryVector: queryVectors[0],
	}, true
}

func (s *Searcher) refreshEmbeddings(ctx context.Context, semantic semanticQuery, items []domain.Item) {
	stale := make([]int, 0, len(items))
	texts := make([]string, 0, len(items))
	for index := range items {
		if items[index].EmbeddingSpace == semantic.space && usableVector(items[index].Embedding, len(semantic.queryVector)) {
			continue
		}
		items[index].EmbeddingSpace = ""
		items[index].Embedding = nil
		stale = append(stale, index)
		texts = append(texts, items[index].Content)
	}
	if len(stale) == 0 {
		return
	}
	vectors, err := semantic.embedder.Embed(ctx, texts)
	if err != nil || len(vectors) != len(stale) {
		return
	}
	updates, ok := buildEmbeddingUpdates(items, stale, semantic, vectors)
	if !ok {
		return
	}
	for offset, index := range stale {
		items[index].EmbeddingSpace = updates[offset].Space
		items[index].Embedding = updates[offset].Vector
	}
	// The cache is derived state: the current request already owns exact
	// vectors, so a failed or losing conditional write must not turn a useful
	// search into an application failure.
	_ = s.store.SetEmbeddings(ctx, updates)
}

func buildEmbeddingUpdates(
	items []domain.Item,
	stale []int,
	semantic semanticQuery,
	vectors [][]float32,
) ([]domain.EmbeddingUpdate, bool) {
	updates := make([]domain.EmbeddingUpdate, 0, len(stale))
	for offset, index := range stale {
		if !usableVector(vectors[offset], len(semantic.queryVector)) {
			return nil, false
		}
		update, err := domain.NewEmbeddingUpdate(items[index], semantic.space, vectors[offset])
		if err != nil {
			return nil, false
		}
		updates = append(updates, update)
	}
	return updates, true
}

func usableVector(vector []float32, dimension int) bool {
	if len(vector) == 0 || dimension > 0 && len(vector) != dimension {
		return false
	}
	return domain.ValidateEmbeddingVector(vector) == nil
}
