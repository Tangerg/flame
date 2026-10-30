package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/core/chat"
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
	db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "flame.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := db.ModelInvocations
	subject := &embedder{id: "space", model: embeddingModelFixture{served: "served-embed"}, selection: testsupport.MustModelSelection("openai", "embed-model"), journal: store}
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "embed-call", Origin: extension.Automatic, Parent: "owner-operation", Target: portable.Target{Scope: extension.ScopeCall, Reference: "embed-call"}})
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
	if row.Snapshot().Invocation.Kind != modelref.EndpointEmbedding || row.State() != modelref.EndpointCompleted || row.ServedModel() != "served-embed" {
		t.Fatalf("embedding endpoint projection changed: %+v", row)
	}
	if row.Snapshot().Usage == nil || row.Snapshot().Usage.InputTokens != 13 || row.Snapshot().Usage.OutputTokens != 0 {
		t.Fatalf("embedding usage is not input-only: %+v", row.Snapshot().Usage)
	}
	if row.Response() != nil || !row.HasResponse() || !strings.Contains(row.Snapshot().RequestJSON.String(), "alpha") {
		t.Fatalf("embedding evidence changed shape: %+v", row)
	}
	if row.Snapshot().Invocation.Caller.ID != "embed-call" || row.Snapshot().Invocation.Caller.Parent != "owner-operation" {
		t.Fatalf("embedding endpoint lost its original caller: %+v", row.Snapshot().Invocation.Caller)
	}
}

func TestEmbeddingRetainsPhysicalEvidenceWhenPricingFails(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cause := errors.New("pricing unavailable")
	subject := &embedder{
		id: "space", model: embeddingModelFixture{served: "served-embed"},
		selection: testsupport.MustModelSelection("openai", "embed-model"), journal: db.ModelInvocations,
		pricing: func(string, string, *chat.Usage) (accounting.Cost, error) { return accounting.Cost{}, cause },
	}
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "embed-pricing", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: "embed-pricing"}})
	if err != nil {
		t.Fatal(err)
	}
	if vectors, err := subject.Embed(ctx, []string{"alpha"}); vectors != nil || !errors.Is(err, cause) {
		t.Fatalf("pricing failure became success: %+v, %v", vectors, err)
	}
	rows, err := db.ModelInvocations.PageEndpointInvocations(t.Context(), "embed-pricing", 0, "", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("embedding endpoint observation: %+v, %v", rows, err)
	}
	if row := rows[0]; row.State() != modelref.EndpointFailed || !row.HasResponse() || row.Snapshot().Usage == nil || row.Snapshot().Usage.InputTokens != 13 || row.Snapshot().CostUSD != nil || row.Snapshot().Failure != cause.Error() {
		t.Fatalf("pricing failure lost embedding evidence or invented a price: %+v", row)
	}
}

func TestEmbeddingRefusesUnrepresentableIndexCoordinatesAndRetainsEvidence(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, coordinate := range []float64{math.MaxFloat64, -math.MaxFloat64} {
		t.Run(fmt.Sprint(coordinate), func(t *testing.T) {
			invocationID := t.Name()
			subject := &embedder{
				id: "space",
				model: embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
					output, err := embedding.NewOutput([]float64{coordinate}, nil)
					if err != nil {
						return nil, err
					}
					return embedding.NewResponse([]*embedding.Output{output}, &embedding.ResponseMetadata{Model: "served-embed", Usage: &embedding.Usage{InputTokens: 1}})
				}),
				selection: testsupport.MustModelSelection("openai", "embed-model"), journal: db.ModelInvocations,
			}
			ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: invocationID, Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: invocationID}})
			if err != nil {
				t.Fatal(err)
			}
			vectors, err := subject.Embed(ctx, []string{"alpha"})
			if err == nil || vectors != nil {
				t.Fatalf("unrepresentable index vector escaped: %v, %v", vectors, err)
			}
			rows, err := db.ModelInvocations.PageEndpointInvocations(t.Context(), invocationID, 0, "", 10)
			if err != nil || len(rows) != 1 {
				t.Fatalf("physical evidence: %+v, %v", rows, err)
			}
			row := rows[0]
			if row.State() != modelref.EndpointFailed || !row.HasResponse() || row.Snapshot().Usage == nil || row.Snapshot().Usage.InputTokens != 1 || !strings.Contains(row.Snapshot().Failure, "float32 range") {
				t.Fatalf("index projection failure lost physical evidence: %+v", row)
			}
		})
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
