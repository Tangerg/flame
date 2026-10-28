package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/qimu/portable"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"

	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

func newServer(store *store) (*sdk.Server, error) {
	if store == nil {
		return nil, fmt.Errorf("reviews: store is required")
	}
	contracts, err := reviews.NewContracts()
	if err != nil {
		return nil, err
	}
	read := func(ctx context.Context) (reviews.ListResult, error) {
		snapshots, err := store.list(ctx)
		return reviews.ListResult{Reviews: snapshots}, err
	}
	// One analyzer business, reached identically through the typed query and the
	// model Tool below; both call the declared analyzer dependency as host.
	analyze := func(ctx context.Context) (result reviews.Analysis, resultErr error) {
		current, err := store.list(ctx)
		if err != nil {
			return reviews.Analysis{}, err
		}
		if len(current) != 1 {
			return reviews.Analysis{}, fmt.Errorf("reviews: expected the owned example review")
		}
		invocation, err := qimumcp.InvocationFromContext(ctx)
		if err != nil {
			return reviews.Analysis{}, err
		}
		ref, err := qimumcp.CallbackFromContext(ctx)
		if err != nil {
			return reviews.Analysis{}, err
		}
		client, err := runtimeclient.ConnectCallback(ctx, ref)
		if err != nil {
			return reviews.Analysis{}, err
		}
		defer func() { resultErr = errors.Join(resultErr, client.Close()) }()
		return analyzeReview(ctx, client, invocation.Target, current[0])
	}
	list, err := tool.NewFunc(tool.FuncConfig{Name: "list_reviews", Description: contracts.List.Descriptor().Description}, func(ctx context.Context, _ struct{}) (reviews.ListResult, error) {
		return read(ctx)
	})
	if err != nil {
		return nil, err
	}
	update, err := tool.NewFunc(tool.FuncConfig{Name: "update_review", Description: contracts.Update.Descriptor().Description + " Runtime supplies the invocation identity; do not add a retry key to arguments."}, func(ctx context.Context, input reviews.Update) (reviews.UpdateResult, error) {
		id, err := pluginsdk.InvocationID(ctx)
		if err != nil {
			return reviews.UpdateResult{}, rejected("bounded invocation identity is required")
		}
		admitted, err := qimumcp.RequestInput(ctx)
		if err != nil {
			return reviews.UpdateResult{}, err
		}
		result, err := store.update(ctx, id, input, admitted)
		if err != nil {
			return reviews.UpdateResult{}, err
		}
		if result.Type == reviews.Updated {
			return result, nil
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return reviews.UpdateResult{}, err
		}
		output := chat.NewTextToolOutput(string(encoded))
		output.Details = encoded
		failure, err := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindFailed, Output: output, Cause: fmt.Errorf("reviews: %s", result.Type)})
		if err != nil {
			return reviews.UpdateResult{}, err
		}
		return reviews.UpdateResult{}, failure
	})
	if err != nil {
		return nil, err
	}
	analyzeTool, err := tool.NewFunc(tool.FuncConfig{Name: "analyze_review", Description: contracts.Analyzers.Descriptor().Description}, func(ctx context.Context, _ struct{}) (reviews.Analysis, error) {
		return analyze(ctx)
	})
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame-review-example", Version: "1.0.0"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := backend.OnSetup(func(_ context.Context, setup qimumcp.Setup) (io.Closer, error) {
		values, err := setup.Contributions()
		if err != nil {
			return nil, err
		}
		required, err := reviews.NewAnalyzerContracts()
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			if value.Export != "reviews" || len(value.Requires) != 1 || value.Requires[0].ID != "analyzers" || !value.Requires[0].Optional {
				return nil, fmt.Errorf("reviews: named optional analyzers dependency is required")
			}
			actual, err := portable.AdmitCapability(value.Requires[0].Contract)
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
	receipts, err := qimumcp.ReceiptQuery()
	if err != nil {
		return nil, err
	}
	if err := receipts.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[qimumcp.ReceiptRequest]) (qimumcp.Receipt, error) {
		return store.receipt(ctx, command.Input, contracts.Update.Name())
	}); err != nil {
		return nil, err
	}
	if err := contracts.List.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, _ qimumcp.Command[struct{}]) (reviews.ListResult, error) {
		return read(ctx)
	}); err != nil {
		return nil, err
	}
	if err := contracts.Update.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[reviews.Update]) (reviews.UpdateResult, error) {
		admitted, err := command.RequestInput()
		if err != nil {
			return reviews.UpdateResult{}, err
		}
		return store.update(ctx, command.ID, command.Input, admitted)
	}); err != nil {
		return nil, err
	}
	if err := contracts.Analyzers.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, _ qimumcp.Command[struct{}]) (reviews.Analysis, error) {
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
