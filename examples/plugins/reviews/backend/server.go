package reviews

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/capability"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
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
	// One analyzer business, reached identically through the typed query and the
	// model Tool below; both call the declared analyzer dependency as host.
	analyze := func(ctx context.Context) (result Analysis, resultErr error) {
		current, err := store.List(ctx)
		if err != nil {
			return Analysis{}, err
		}
		if len(current) != 1 {
			return Analysis{}, fmt.Errorf("reviews: expected the owned example review")
		}
		invocation, err := pluginsdk.InvocationFromContext(ctx)
		if err != nil {
			return Analysis{}, err
		}
		ref, err := pluginsdk.CallbackFromContext(ctx)
		if err != nil {
			return Analysis{}, err
		}
		client, err := runtimeclient.ConnectCallback(ctx, ref)
		if err != nil {
			return Analysis{}, err
		}
		defer func() { resultErr = errors.Join(resultErr, client.Close()) }()
		return AnalyzeReview(ctx, client, invocation.Target, current[0])
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
		admitted, err := pluginsdk.RequestInput(ctx)
		if err != nil {
			return UpdateResult{}, err
		}
		result, err := store.Update(ctx, id, input, admitted)
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
	analyzeTool, err := tool.NewFunc(tool.FuncConfig{Name: "analyze_review", Description: contracts.Analyzers.Descriptor().Description}, func(ctx context.Context, _ struct{}) (Analysis, error) {
		return analyze(ctx)
	})
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame-review-example", Version: "1.0.0"}, nil)
	backend, err := pluginsdk.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := backend.OnSetup(func(_ context.Context, setup pluginsdk.Setup) (io.Closer, error) {
		values, err := setup.Contributions()
		if err != nil {
			return nil, err
		}
		required, err := NewAnalyzerContracts()
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			if value.Export != "reviews" || len(value.Requires) != 1 || value.Requires[0].ID != "analyzers" || !value.Requires[0].Optional {
				return nil, fmt.Errorf("reviews: named optional analyzers dependency is required")
			}
			actual, err := capability.FromDescriptor(value.Requires[0].Contract)
			if err != nil {
				return nil, err
			}
			if actual.Identity() != required.Capability.Identity() {
				return nil, fmt.Errorf("reviews: analyzer setup contract differs from the consumer")
			}
		}
		return nil, nil
	}); err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, list, update, analyzeTool); err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterServer(backend, pluginsdk.ReceiptMethod, nil, func(ctx context.Context, _ *sdk.ServerSession, request pluginsdk.ReceiptRequest) (pluginsdk.Receipt, error) {
		return store.Receipt(ctx, request, contracts.Update.Name())
	}); err != nil {
		return nil, err
	}
	if err := contracts.List.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, _ pluginsdk.Command[struct{}]) (ListResult, error) {
		return read(ctx)
	}); err != nil {
		return nil, err
	}
	if err := contracts.Update.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[Update]) (UpdateResult, error) {
		admitted, err := command.RequestInput()
		if err != nil {
			return UpdateResult{}, err
		}
		return store.Update(ctx, command.ID, command.Input, admitted)
	}); err != nil {
		return nil, err
	}
	if err := contracts.Analyzers.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, _ pluginsdk.Command[struct{}]) (Analysis, error) {
		return analyze(ctx)
	}); err != nil {
		return nil, err
	}
	if err := backend.BindCapability("reviews", contracts.Capability); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
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
