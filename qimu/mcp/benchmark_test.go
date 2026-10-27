package mcp_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type queryDocument struct {
	Text     string `json:"text"`
	Revision uint64 `json:"revision"`
}

type queryParams struct {
	sdk.ParamsBase
	Input queryDocument `json:"input"`
}

type queryResult struct {
	sdk.ResultBase
	Value queryDocument `json:"value"`
}

// BenchmarkQueryRoundTrip compares the public typed Qimu boundary to a typed
// custom MCP method on the very same session and in-memory byte transport.
// Both handlers only echo the same document. Qimu also admits its contract and
// input/output schema, preserves exact JSON input, and classifies its reply.
// Connection, publication, and schema construction are outside the timed loop;
// this measures warm unary calls, not process startup or Host acquisition.
func BenchmarkQueryRoundTrip(b *testing.B) {
	const rawMethod = "benchmark/echo"
	server := sdk.NewServer(&sdk.Implementation{Name: "benchmark-server"}, nil)
	if err := sdk.AddReceivingCustomMethod(server, rawMethod, func(_ context.Context, _ *sdk.ServerSession, params *queryParams) (*queryResult, error) {
		return &queryResult{Value: params.Input}, nil
	}); err != nil {
		b.Fatal(err)
	}
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := backend.Close(); err != nil {
			b.Error(err)
		}
	})
	method, err := qimumcp.NewMethod[queryDocument, queryDocument](qimumcp.MethodConfig{
		Name: "benchmark/typed-echo", Kind: portable.Query, Target: "installation", Description: "Echo the input document.",
	})
	if err != nil {
		b.Fatal(err)
	}
	if err := method.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command qimumcp.Command[queryDocument]) (queryDocument, error) {
		return command.Input, nil
	}); err != nil {
		b.Fatal(err)
	}
	if err := backend.Publish(); err != nil {
		b.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "benchmark-client"}, nil)
	if err := sdk.AddSendingCustomMethod[*queryParams, *queryResult](client, rawMethod); err != nil {
		b.Fatal(err)
	}
	if err := qimumcp.RegisterClient(client); err != nil {
		b.Fatal(err)
	}
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(b.Context(), serverTransport, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = serverSession.Close() })
	clientSession, err := client.Connect(b.Context(), clientTransport, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = clientSession.Close() })

	for _, size := range []int{32, 4096} {
		b.Run(fmt.Sprintf("textBytes=%d", size), func(b *testing.B) {
			input := queryDocument{Text: strings.Repeat("x", size), Revision: 7}
			b.Run("SDKCustom", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					result, err := sdk.CallCustomMethod[*queryParams, *queryResult](b.Context(), clientSession, rawMethod, &queryParams{Input: input})
					if err != nil {
						b.Fatal(err)
					}
					if result == nil || result.Value != input {
						b.Fatal("the MCP reply changed the document")
					}
				}
			})
			b.Run("QimuTyped", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					result, err := method.Call(b.Context(), clientSession, input)
					if err != nil {
						b.Fatal(err)
					}
					if result != input {
						b.Fatal("the Qimu reply changed the document")
					}
				}
			})
		})
	}
}
