package agentmemory

import (
	"context"
	"errors"
	"reflect"
	"slices"

	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

// ReadStore supplies the active memory views used in model context. Search
// combines the exact project with user memory; Items reads one exact target.
// Returned items and collections transfer ownership to the caller. SetEmbeddings
// borrows its updates synchronously; the cache remains conditional on exact content identity.
type ReadStore interface {
	Items(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
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

// ReadModel is the sole Application boundary for Agent Memory entering model
// context, whether pinned directly or recalled by search.
type ReadModel struct {
	store           ReadStore
	resolveEmbedder func(context.Context) (Embedder, error)
}

// NewReadModel constructs model-context reads over a required store. A nil
// resolver selects keyword-only search.
func NewReadModel(store ReadStore, resolveEmbedder func(context.Context) (Embedder, error)) (*ReadModel, error) {
	if nilDependency(store) {
		return nil, errors.New("agentmemory: read store is required")
	}
	return &ReadModel{store: store, resolveEmbedder: resolveEmbedder}, nil
}

// Items returns the complete active catalog for one exact target.
func (r *ReadModel) Items(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error) {
	if err := domain.ValidateTarget(scope, project); err != nil {
		return nil, err
	}
	items, err := r.store.Items(ctx, scope, project)
	if err != nil {
		return nil, err
	}
	if err := validateActiveTargetCatalog(items, scope, project); err != nil {
		return nil, err
	}
	return items, nil
}

// Search returns up to topK relevant project- and user-scoped memory items for
// one project context. Ranking happens over the combined corpus so neither
// partition receives a separate top-k quota or query embedding.
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
	if err := validateSearchCatalog(items, project); err != nil {
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
	if err != nil || nilDependency(embedder) {
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
		queryVector: slices.Clone(queryVectors[0]),
	}, true
}

func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (r *ReadModel) refreshEmbeddings(ctx context.Context, semantic semanticQuery, items []domain.Item) {
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
	_ = r.store.SetEmbeddings(ctx, updates)
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
