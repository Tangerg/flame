package reviews

import (
	"context"
	json "encoding/json/v2"
	"fmt"

	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

const InvocationMetadataKey = "io.github.tangerg.flame/invocationId"

func NewServer(store *Store) (*sdk.Server, error) {
	if store == nil {
		return nil, fmt.Errorf("reviews: store is required")
	}
	list, err := tool.NewFunc(tool.FuncConfig{Name: "list_reviews", Description: "Read the review backend's current review and revision."}, func(ctx context.Context, _ struct{}) (struct {
		Reviews []Snapshot `json:"reviews"`
	}, error) {
		reviews, err := store.List(ctx)
		return struct {
			Reviews []Snapshot `json:"reviews"`
		}{Reviews: reviews}, err
	})
	if err != nil {
		return nil, err
	}
	update, err := tool.NewFunc(tool.FuncConfig{Name: "update_review", Description: "Change one review's status using its inspected revision. Runtime supplies the invocation identity; do not add a retry key to arguments."}, func(ctx context.Context, input Update) (UpdateResult, error) {
		id, _ := scopemcp.RequestMetaFromContext(ctx)[InvocationMetadataKey].(string)
		if validateInvocationID(id) != nil {
			return UpdateResult{}, rejected("bounded invocation identity is required")
		}
		result, err := store.Update(ctx, id, input)
		if err != nil {
			return UpdateResult{}, err
		}
		if result.Type == Updated {
			return result, nil
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return UpdateResult{}, err
		}
		output := chat.NewTextToolOutput(string(encoded))
		output.Details = encoded
		failure, err := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindFailed, Output: output, Cause: fmt.Errorf("reviews: %s", result.Type)})
		if err != nil {
			return UpdateResult{}, err
		}
		return UpdateResult{}, failure
	})
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame-review-example", Version: "1.0.0"}, nil)
	server.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, request sdk.Request) (sdk.Result, error) {
			if call, ok := request.(*sdk.CallToolRequest); ok && call.Params != nil {
				ctx = scopemcp.WithRequestMeta(ctx, call.Params.Meta)
			}
			return next(ctx, method, request)
		}
	})
	if err := scopemcp.Register(server, list, update); err != nil {
		return nil, err
	}
	return server, nil
}

func rejected(message string) error {
	failure, err := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindRejected, Output: chat.NewTextToolOutput(message)})
	if err != nil {
		return err
	}
	return failure
}
