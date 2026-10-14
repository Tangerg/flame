package pluginservices

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type roleCallbackEmbedder struct {
	t     *testing.T
	calls atomic.Int32
}

func (*roleCallbackEmbedder) ID() string { return "configured-space" }
func (e *roleCallbackEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil || invocation.ID == "caller" || invocation.Origin != capability.Model || invocation.Parent != "caller" || invocation.Target != (capability.Target{Scope: "call", Reference: invocation.ID}) {
		e.t.Errorf("ordinary embedding lost original ownership: %+v, %v", invocation, err)
	}
	e.calls.Add(1)
	return [][]float32{{1}}, nil
}

func TestOrdinaryCallbackPublishesEmbeddingRoleWithoutFeaturePolicy(t *testing.T) {
	for _, state := range []string{"configured", "absent", "failed", "typed nil"} {
		t.Run(state, func(t *testing.T) {
			callbacks := pluginsdk.NewCallbacks()
			t.Cleanup(func() {
				if err := callbacks.Close(); err != nil {
					t.Error(err)
				}
			})
			model := &roleCallbackEmbedder{t: t}
			var resolutions atomic.Int32
			callback, err := EmbeddingRoleCallback(func(context.Context) (models.TextEmbedder, error) {
				resolutions.Add(1)
				switch state {
				case "absent":
					return nil, nil
				case "failed":
					return nil, errors.New("configured role lookup failed")
				case "typed nil":
					return (*roleCallbackEmbedder)(nil), nil
				default:
					return model, nil
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := callbacks.PublishRPC(http.NotFoundHandler(), callback); err != nil {
				t.Fatal(err)
			}
			session := new(sdk.ClientSession)
			callbacks.AdmitSession(session)
			ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "caller", Origin: capability.Model, Parent: "executing-owner", Target: capability.Target{Scope: "installation", Reference: "author"}})
			if err != nil {
				t.Fatal(err)
			}
			ctx = executionctx.WithInvocationOwner(ctx, "executing-owner")
			ref, release, err := callbacks.OpenRPC(ctx, session, pluginsdk.CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 4096})
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			for _, id := range []string{"first", "second", "third"} {
				vectors, err := pluginsdk.CallTextEmbedding(t.Context(), ref, id, []string{"text"})
				if state == "configured" {
					if err != nil || len(vectors) != 1 || len(vectors[0]) != 1 || vectors[0][0] != 1 {
						t.Fatalf("ordinary embedding = %+v, %v", vectors, err)
					}
				} else {
					refusal, known := errors.AsType[*capability.Refusal](err)
					if err == nil || known != (state == "absent") || known && refusal.Code != "model_role_unconfigured" {
						t.Fatalf("role absence and failure were merged: %v", err)
					}
				}
			}
			want := int32(0)
			if state == "configured" {
				want = 3
			}
			if model.calls.Load() != want || resolutions.Load() != 3 {
				t.Fatalf("model calls=%d resolutions=%d", model.calls.Load(), resolutions.Load())
			}
			if _, err := pluginsdk.CallTextEmbedding(t.Context(), ref, "executing-owner", []string{"text"}); err == nil {
				t.Fatal("independent embedding claimed the executing owner identity")
			}
			if _, err := pluginsdk.CallTextEmbedding(t.Context(), ref, "oversized", []string{strings.Repeat("x", 8192)}); err == nil {
				t.Fatal("callback byte budget was ignored")
			}
			if model.calls.Load() != want || resolutions.Load() != 3 {
				t.Fatal("over-budget input reached model resolution")
			}
			release()
			if _, err := pluginsdk.CallTextEmbedding(t.Context(), ref, "retired", []string{"text"}); !errors.Is(err, pluginsdk.ErrStaleCall) {
				t.Fatalf("retired callback remained usable: %v", err)
			}
		})
	}
}
