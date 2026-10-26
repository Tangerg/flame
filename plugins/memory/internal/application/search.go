package coordination

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	domain "github.com/Tangerg/flame/plugins/memory/internal/domain"
	"github.com/samber/lo"
)

// Returned items transfer ownership so read ordering cannot mutate store state.
type ReadStore interface {
	Items(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
	SearchCorpus(ctx context.Context, project string) ([]domain.Item, error)
	SetEmbeddings(ctx context.Context, updates []domain.EmbeddingUpdate) error
}

// A failed optional embedding call cannot make keyword search unavailable.
type Embedder interface {
	// Cache identity must include the model configuration because it selects the vector coordinate system.
	ID() string
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type ReadModel struct {
	store           ReadStore
	resolveEmbedder func(context.Context) (Embedder, error)
}

func NewReadModel(store ReadStore, resolveEmbedder func(context.Context) (Embedder, error)) (*ReadModel, error) {
	if lo.IsNil(store) {
		return nil, errors.New("agentmemory: read store is required")
	}
	return &ReadModel{store: store, resolveEmbedder: resolveEmbedder}, nil
}

func (r *ReadModel) Items(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error) {
	if err := domain.ValidateTarget(scope, project); err != nil {
		return nil, err
	}
	items, err := r.store.Items(ctx, scope, project)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(items, compareActiveItems)
	return items, nil
}

// Project and user items share one ranking pass so each partition cannot consume a separate quota.
func (r *ReadModel) Search(ctx context.Context, project, query string, topK int) ([]domain.Item, error) {
	if topK <= 0 {
		return nil, nil
	}
	if err := domain.ValidateTarget(domain.ScopeProject, project); err != nil {
		return nil, err
	}
	items, err := r.store.SearchCorpus(ctx, project)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	semantic, ok := r.resolveSemanticQuery(ctx, query)
	if !ok {
		return domain.Rank(query, nil, items, topK), nil
	}
	r.refreshEmbeddings(ctx, semantic, items)
	return domain.Rank(query, semantic.queryVector, items, topK), nil
}

type semanticQuery struct {
	embedder    Embedder
	space       string
	queryVector []float32
}

func (r *ReadModel) resolveSemanticQuery(ctx context.Context, query string) (semanticQuery, bool) {
	if r.resolveEmbedder == nil {
		return semanticQuery{}, false
	}
	embedder, err := r.resolveEmbedder(ctx)
	if err != nil {
		slog.WarnContext(ctx, "agentmemory: resolve embedding model", "error", err)
		return semanticQuery{}, false
	}
	if lo.IsNil(embedder) {
		return semanticQuery{}, false
	}
	space := embedder.ID()
	if space == "" {
		return semanticQuery{}, false
	}
	queryVectors, err := embedder.Embed(ctx, []string{query})
	if err != nil {
		slog.WarnContext(ctx, "agentmemory: embed search query", "error", err)
		return semanticQuery{}, false
	}
	if len(queryVectors) != 1 || !usableVector(queryVectors[0], 0) {
		return semanticQuery{}, false
	}
	return semanticQuery{
		embedder:    embedder,
		space:       space,
		queryVector: slices.Clone(queryVectors[0]),
	}, true
}

func (r *ReadModel) refreshEmbeddings(ctx context.Context, semantic semanticQuery, items []domain.Item) {
	stale := make([]int, 0, len(items))
	texts := make([]string, 0, len(items))
	for index := range items {
		space, vector, cached := items[index].Embedding()
		if cached && space == semantic.space && usableVector(vector, len(semantic.queryVector)) {
			continue
		}
		items[index] = items[index].WithoutEmbedding()
		stale = append(stale, index)
		texts = append(texts, items[index].Content())
	}
	if len(stale) == 0 {
		return
	}
	vectors, err := semantic.embedder.Embed(ctx, texts)
	if err != nil {
		slog.WarnContext(ctx, "agentmemory: embed search corpus", "error", err)
		return
	}
	if len(vectors) != len(stale) {
		return
	}
	updates, ok := buildEmbeddingUpdates(items, stale, semantic, vectors)
	if !ok {
		return
	}
	for offset, index := range stale {
		cached, err := items[index].WithEmbedding(updates[offset])
		if err != nil {
			return
		}
		items[index] = cached
	}
	// Cache writes are best-effort because this search already owns exact vectors.
	if err := r.store.SetEmbeddings(ctx, updates); err != nil {
		slog.WarnContext(ctx, "agentmemory: cache corpus embeddings", "error", err)
	}
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
