package plugin

import (
	"context"
	"encoding/json/jsontext"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/tool"
	"testing"
)

func TestToolBoundaryRetainsOriginalArgumentsAndOptionalEmptyObject(t *testing.T) {
	type input struct {
		N float64 `json:"n"`
	}
	var captured jsonvalue.Value
	read := func(ctx context.Context) (string, error) {
		var err error
		captured, err = qimumcp.RequestInput(ctx)
		return captured.String(), err
	}
	number, err := tool.NewFunc(tool.FuncConfig{Name: "number", Description: "Read a number."}, func(ctx context.Context, _ input) (string, error) { return read(ctx) })
	if err != nil {
		t.Fatal(err)
	}
	empty, err := tool.NewFunc(tool.FuncConfig{Name: "empty", Description: "Read no arguments."}, func(ctx context.Context, _ struct{}) (string, error) { return read(ctx) })
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	if err := RegisterTools(server, number, empty); err != nil {
		t.Fatal(err)
	}
	session := connectMethodSessions(t, server, sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil))
	if result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "number", Arguments: jsontext.Value(`{"n":1.0}`)}); err != nil || result.IsError || captured.String() != `{"n":1.0}` {
		t.Fatalf("Tool lost original numeric input: %s, %v", captured.String(), err)
	}
	if result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "empty"}); err != nil || result.IsError || captured.String() != `{}` {
		t.Fatalf("optional Tool arguments = %s, %v", captured.String(), err)
	}
}
