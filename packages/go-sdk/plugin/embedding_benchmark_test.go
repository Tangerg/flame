package plugin

import (
	"context"
	"fmt"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"net/http"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func BenchmarkTextEmbeddingCallback(b *testing.B) {
	for _, size := range []int{1, 16} {
		b.Run(fmt.Sprintf("texts=%d", size), func(b *testing.B) {
			callbacks := qimumcp.NewCallbacks()
			b.Cleanup(func() { _ = callbacks.Close() })
			session := new(sdk.ClientSession)
			if err := callbacks.AdmitSession(session); err != nil {
				b.Fatal(err)
			}
			texts := make([]string, size)
			vectors := make([][]float32, size)
			for i := range texts {
				texts[i] = strings.Repeat("x", 1024)
				vectors[i] = make([]float32, 1536)
				for j := range vectors[i] {
					vectors[i][j] = 0.125
				}
			}
			callback, err := NewTextEmbeddingCallback(func(context.Context, qimumcp.Command[[]string]) (TextEmbeddings, error) {
				return TextEmbeddings{Space: "captured-space", Vectors: vectors}, nil
			})
			if err != nil {
				b.Fatal(err)
			}
			if err := callbacks.PublishRPC(http.NotFoundHandler(), callback); err != nil {
				b.Fatal(err)
			}
			ctx, err := qimumcp.WithInvocation(b.Context(), portable.Invocation{ID: "knowledge-operation", Origin: extension.Human, Target: portable.Target{Scope: "installation", Reference: "knowledge"}})
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				ref, release, err := callbacks.OpenRPC(ctx, session, qimumcp.CallbackLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20})
				if err != nil {
					b.Fatal(err)
				}
				_, err = CallTextEmbedding(ctx, ref, "embedding-child", texts)
				release()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
