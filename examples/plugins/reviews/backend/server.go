package reviews

import (
	"context"
	json "encoding/json/v2"
	"fmt"

	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

func NewServer(store *Store) (*sdk.Server, error) {
	if store == nil {
		return nil, fmt.Errorf("reviews: store is required")
	}
	contracts, err := NewContracts()
	if err != nil {
		return nil, err
	}
	read := func(ctx context.Context) (ListResult, error) {
		reviews, err := store.List(ctx)
		return ListResult{Reviews: reviews}, err
	}
	list, err := tool.NewFunc(tool.FuncConfig{Name: "list_reviews", Description: contracts.List.Descriptor().Description}, func(ctx context.Context, _ struct{}) (ListResult, error) {
		return read(ctx)
	})
	if err != nil {
		return nil, err
	}
	update, err := tool.NewFunc(tool.FuncConfig{Name: "update_review", Description: contracts.Update.Descriptor().Description + " Runtime supplies the invocation identity; do not add a retry key to arguments."}, func(ctx context.Context, input Update) (UpdateResult, error) {
		id, err := pluginsdk.InvocationID(ctx)
		if err != nil {
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
	if err := pluginsdk.RegisterTools(server, list, update); err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterServer(server, pluginsdk.ReceiptMethod, nil, func(ctx context.Context, _ *sdk.ServerSession, request pluginsdk.ReceiptRequest) (pluginsdk.Receipt, error) {
		return store.Receipt(ctx, request, contracts.Update.Name())
	}); err != nil {
		return nil, err
	}
	if err := contracts.List.RegisterServer(server, nil, func(ctx context.Context, _ *sdk.ServerSession, _ pluginsdk.Command[struct{}]) (ListResult, error) {
		return read(ctx)
	}); err != nil {
		return nil, err
	}
	if err := contracts.Update.RegisterServer(server, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[Update]) (UpdateResult, error) {
		return store.Update(ctx, command.ID, command.Input)
	}); err != nil {
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
