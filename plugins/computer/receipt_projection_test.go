package computer_test

import (
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"os"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

type projectionCase struct {
	Name     string          `json:"name"`
	Result   json.RawMessage `json:"result"`
	Expected json.RawMessage `json:"expected,omitzero"`
	Rejected bool            `json:"rejected,omitzero"`
}

func TestReceiptProjectionMatchesScope(t *testing.T) {
	encoded, err := os.ReadFile("backend/testdata/tool-results.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []projectionCase
	if err := jsonv2.Unmarshal(encoded, &cases, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			output, err := scopeProjection(t.Context(), test.Result)
			if test.Rejected {
				if err == nil {
					t.Fatal("expected Scope to reject invalid MCP content")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			actual, err := jsonvalue.Encode(output)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := jsonvalue.Parse(test.Expected)
			if err != nil {
				t.Fatal(err)
			}
			if actual != expected {
				t.Fatalf("Scope output differs from the receipt projection:\nactual %s\nexpected %s", actual, expected)
			}
		})
	}
}

func scopeProjection(ctx context.Context, encoded json.RawMessage) (output chat.ToolOutput, err error) {
	var result sdkmcp.CallToolResult
	if err := jsonv2.Unmarshal(encoded, &result); err != nil {
		return output, err
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "receipt-oracle", Version: "1"}, nil)
	server.AddTool(&sdkmcp.Tool{Name: "js", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return &result, nil
	})
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return output, err
	}
	defer func() { err = errors.Join(err, serverSession.Close()) }()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "receipt-consumer", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return output, err
	}
	defer func() { err = errors.Join(err, session.Close()) }()
	tools, err := scopemcp.DiscoverTools(ctx, []scopemcp.ToolSource{{Name: "computer", Session: session}}, scopemcp.ToolDiscoveryConfig{})
	if err != nil {
		return output, err
	}
	bound, err := tool.Bind(tools[0])
	if err != nil {
		return output, err
	}
	invocation, err := bound.Contract().Prepare(chat.ToolCall{ID: "original", Name: bound.Contract().Definition().Name, Arguments: `{}`}, nil)
	if err != nil {
		return output, err
	}
	reply, err := bound.Call(ctx, invocation)
	if failure, ok := errors.AsType[*tool.Failure](err); ok {
		return failure.Output(), nil
	}
	return reply.Output, err
}
