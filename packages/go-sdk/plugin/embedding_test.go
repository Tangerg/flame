package plugin

import (
	"context"
	"errors"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"math"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestTextEmbeddingCallbackKeepsChildAttributionAndCallerBudget(t *testing.T) {
	callbacks := qimumcp.NewCallbacks()
	t.Cleanup(func() {
		if err := callbacks.Close(); err != nil {
			t.Error(err)
		}
	})
	session := new(sdk.ClientSession)
	if err := callbacks.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	owner := portable.Invocation{ID: "original-tool", Origin: extension.Model, Parent: "executing-run", Target: portable.Target{Scope: "installation", Reference: "owner"}}
	ctx, err := qimumcp.WithInvocation(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	callback, err := NewTextEmbeddingCallback(func(ctx context.Context, command qimumcp.Command[[]string]) (TextEmbeddings, error) {
		invocation, err := qimumcp.InvocationFromContext(ctx)
		if err != nil || invocation.ID != command.ID || invocation.Origin != owner.Origin || invocation.Parent != owner.ID || invocation.Target != (portable.Target{Scope: "call", Reference: command.ID}) {
			t.Errorf("embedding changed original ownership: %+v, %v", invocation, err)
		}
		input, err := command.RequestInput()
		if err != nil || input.String() != `["captured text"]` || !slices.Equal(command.Input, []string{"captured text"}) {
			t.Errorf("embedding changed admitted input: %+v, %v", command, err)
		}
		calls.Add(1)
		return TextEmbeddings{Space: "captured-space", Vectors: [][]float32{{1, 2}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := callbacks.Open(ctx, session, qimumcp.CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 4096}, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, id := range []string{"first", "second", "third"} {
		vectors, err := CallTextEmbedding(t.Context(), ref, id, []string{"captured text"})
		if err != nil || vectors.Space != "captured-space" || len(vectors.Vectors) != 1 || !slices.Equal(vectors.Vectors[0], []float32{1, 2}) {
			t.Fatalf("exchange %s: %+v, %v", id, vectors, err)
		}
	}
	if _, err := CallTextEmbedding(t.Context(), ref, owner.ID, []string{"captured text"}); err == nil {
		t.Error("independent embedding reused its calling owner's identity")
	}
	if calls.Load() != 3 {
		t.Fatalf("physical calls = %d", calls.Load())
	}
	release()
	if _, err := CallTextEmbedding(t.Context(), ref, "retired", []string{"captured text"}); !errors.Is(err, qimumcp.ErrStaleCall) {
		t.Fatalf("retired grant accepted work: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatal("retirement dispatched another embedding")
	}
}

func TestTextEmbeddingCallbackRejectsMalformedCorrespondence(t *testing.T) {
	for _, vectors := range [][][]float32{nil, {{}, {}}, {{1}, {1, 2}}} {
		callbacks := qimumcp.NewCallbacks()
		session := new(sdk.ClientSession)
		if err := callbacks.AdmitSession(session); err != nil {
			t.Fatal(err)
		}
		callback, err := NewTextEmbeddingCallback(func(context.Context, qimumcp.Command[[]string]) (TextEmbeddings, error) {
			return TextEmbeddings{Space: "captured-space", Vectors: vectors}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		ref, release, err := callbacks.Open(t.Context(), session, qimumcp.CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 4096}, callback)
		if err != nil {
			t.Fatal(err)
		}
		_, callErr := CallTextEmbedding(t.Context(), ref, "one", []string{"first", "second"})
		release()
		if err := callbacks.Close(); err != nil {
			t.Fatal(err)
		}
		if callErr == nil {
			t.Fatalf("malformed vectors became success: %+v", vectors)
		}
	}
}

func TestTextEmbeddingResultRequiresItsActualSpaceAndFiniteVectors(t *testing.T) {
	for _, result := range []TextEmbeddings{
		{Vectors: [][]float32{{1}}},
		{Space: "space", Vectors: [][]float32{{float32(math.Inf(1))}}},
		{Space: "space", Vectors: [][]float32{{float32(math.NaN())}}},
	} {
		if err := validateTextEmbeddings([]string{"text"}, result); err == nil {
			t.Fatalf("unidentified or non-finite result admitted: %+v", result)
		}
	}
}
