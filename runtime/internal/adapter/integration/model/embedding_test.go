package model

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/embedding"
)

type embeddingModelFixture struct{ served string }

func (f embeddingModelFixture) Call(_ context.Context, request *embedding.Request) (*embedding.Response, error) {
	outputs := make([]*embedding.Output, len(request.Texts))
	for index := range request.Texts {
		output, err := embedding.NewOutput([]float64{float64(index) + 0.25, 0.75}, nil)
		if err != nil {
			return nil, err
		}
		outputs[index] = output
	}
	return embedding.NewResponse(outputs, &embedding.ResponseMetadata{Model: f.served, Usage: &embedding.Usage{InputTokens: 13}, CreatedAt: time.Unix(10, 0).UTC()})
}

func TestObservedEmbedderJournalsEndpointEvidenceThroughSharedOwner(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewModelInvocationStore(db)
	subject := &embedder{id: "space", model: embeddingModelFixture{served: "served-embed"}, selection: testsupport.MustModelSelection("openai", "embed-model"), journal: store}
	ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "embed-call", Origin: capability.Automatic, Parent: "owner-operation", Target: capability.Target{Scope: "call", Reference: "embed-call"}})
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := subject.Embed(ctx, []string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 || len(vectors[0]) != 2 {
		t.Fatalf("embedding vectors lost shape: %+v", vectors)
	}
	rows, err := store.PageEndpointInvocations(t.Context(), "embed-call", 0, "", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("embedding endpoint evidence: %+v %v", rows, err)
	}
	row := rows[0]
	if row.Invocation.Kind != modelref.EndpointEmbedding || row.State != "completed" || row.ServedModel != "served-embed" {
		t.Fatalf("embedding endpoint projection changed: %+v", row)
	}
	if row.Usage == nil || row.Usage.InputTokens != 13 || row.Usage.OutputTokens != 0 {
		t.Fatalf("embedding usage is not input-only: %+v", row.Usage)
	}
	if row.Response != nil || !row.HasResponse() || !strings.Contains(row.RequestJSON.String(), "alpha") {
		t.Fatalf("embedding evidence changed shape: %+v", row)
	}
	if row.Invocation.Caller.ID != "embed-call" || row.Invocation.Caller.Parent != "owner-operation" {
		t.Fatalf("embedding endpoint lost its original caller: %+v", row.Invocation.Caller)
	}
}

func TestEmbeddingSpaceIDIncludesEndpointIdentityWithoutPersistingIt(t *testing.T) {
	const (
		provider  = "openai-compatible"
		model     = "text-embedding"
		firstURL  = "https://first.example.test/v1"
		secondURL = "https://second.example.test/v1"
	)

	first := embeddingSpaceID(provider, model, firstURL)
	if first == embeddingSpaceID(provider, model, secondURL) {
		t.Fatal("embedding space did not change with the endpoint")
	}
	if first != embeddingSpaceID(provider, model, firstURL) {
		t.Fatal("embedding space is not deterministic")
	}
	if strings.Contains(first, firstURL) {
		t.Fatal("embedding space persisted the raw endpoint")
	}
}

func TestEmbeddingSpaceIDPreservesFieldBoundaries(t *testing.T) {
	left := embeddingSpaceID("provider", "model-part", "endpoint")
	right := embeddingSpaceID("provider-model", "part", "endpoint")
	if left == right {
		t.Fatal("different embedding identity fields produced the same space")
	}
}
