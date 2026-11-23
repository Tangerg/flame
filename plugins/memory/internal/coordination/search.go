package coordination

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/Tangerg/flame/plugins/memory/internal/agentmemory"
	"github.com/samber/lo"
)

type ReadStore interface {
	Items(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error)
	SearchCorpus(ctx context.Context, project string) ([]agentmemory.Item, error)
	SetEmbeddings(ctx context.Context, updates []agentmemory.EmbeddingUpdate) error
}

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

func (r *ReadModel) Items(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	if err := agentmemory.ValidateTarget(scope, project); err != nil {
		return nil, err
	}
	items, err := r.store.Items(ctx, scope, project)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(items, compareActiveItems)
	return items, nil
}

func (r *ReadModel) Search(ctx context.Context, project, query string, topK int) ([]agentmemory.Item, error) {
	if topK <= 0 {
		return nil, nil
	}
	if err := agentmemory.ValidateTarget(agentmemory.ScopeProject, project); err != nil {
		return nil, err
	}
	items, err := r.store.SearchCorpus(ctx, project)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	semantic, ok := r.resolveSemanticQuery(ctx, query)
	if !ok {
		return agentmemory.Rank(query, nil, items, topK), nil
	}
	r.refreshEmbeddings(ctx, semantic, items)
	return agentmemory.Rank(query, semantic.queryVector, items, topK), nil
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
		warnSemanticDegraded(ctx, "resolve embedding model", err)
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
		warnSemanticDegraded(ctx, "embed search query", err)
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

func (r *ReadModel) refreshEmbeddings(ctx context.Context, semantic semanticQuery, items []agentmemory.Item) {
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
		warnSemanticDegraded(ctx, "embed search corpus", err)
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
	if err := r.store.SetEmbeddings(ctx, updates); err != nil {
		warnSemanticDegraded(ctx, "cache corpus embeddings", err)
	}
}

// Semantic ranking only enhances lexical search, so its failures are reported and never fail a search.
func warnSemanticDegraded(ctx context.Context, stage string, err error) {
	slog.WarnContext(ctx, "agentmemory: semantic search degraded", "stage", stage, "error", err)
}

func buildEmbeddingUpdates(
	items []agentmemory.Item,
	stale []int,
	semantic semanticQuery,
	vectors [][]float32,
) ([]agentmemory.EmbeddingUpdate, bool) {
	updates := make([]agentmemory.EmbeddingUpdate, 0, len(stale))
	for offset, index := range stale {
		if !usableVector(vectors[offset], len(semantic.queryVector)) {
			return nil, false
		}
		update, err := agentmemory.NewEmbeddingUpdate(items[index], semantic.space, vectors[offset])
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
	return agentmemory.ValidateEmbeddingVector(vector) == nil
}
