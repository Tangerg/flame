package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/tool"
)

const (
	capabilityExport    = "reviews"
	analyzersDependency = "analyzers"
)

const (
	listReviewsTool   = "list_reviews"
	updateReviewTool  = "update_review"
	analyzeReviewTool = "analyze_review"
)

func newServer(store *store) (*sdk.Server, error) {
	if store == nil {
		return nil, fmt.Errorf("reviews: store is required")
	}
	contracts, err := reviews.NewContracts()
	if err != nil {
		return nil, err
	}
	service := reviewService{store: store, contracts: contracts}
	tools, err := service.modelTools()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame-review-example", Version: "1.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := backend.OnSetup(admitSetup); err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, tools...); err != nil {
		return nil, err
	}
	if err := service.register(backend); err != nil {
		return nil, err
	}
	if err := backend.BindCapability(capabilityExport, contracts.Capability); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}

type reviewService struct {
	store     *store
	contracts reviews.Contracts
}

func (s reviewService) read(ctx context.Context) (reviews.ListResult, error) {
	snapshots, err := s.store.list(ctx)
	return reviews.ListResult{Reviews: snapshots}, err
}

func (s reviewService) analyze(ctx context.Context) (result reviews.Analysis, resultErr error) {
	current, err := s.store.list(ctx)
	if err != nil {
		return reviews.Analysis{}, err
	}
	if len(current) != 1 {
		return reviews.Analysis{}, fmt.Errorf("reviews: expected the owned example review")
	}
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return reviews.Analysis{}, err
	}
	ref, err := pluginmcp.CallbackFromContext(ctx)
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

func (s reviewService) updateFromModel(ctx context.Context, _ reviews.Update) (reviews.UpdateResult, error) {
	id, err := pluginsdk.InvocationID(ctx)
	if err != nil {
		return reviews.UpdateResult{}, rejected("bounded invocation identity is required")
	}
	admitted, err := pluginmcp.RequestInput(ctx)
	if err != nil {
		return reviews.UpdateResult{}, err
	}
	result, err := s.store.update(ctx, id, admitted)
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
	output := tool.NewTextOutput(string(encoded))
	output.Details = encoded
	failure, err := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindFailed, Output: output, Cause: fmt.Errorf("reviews: %s", result.Type)})
	if err != nil {
		return reviews.UpdateResult{}, err
	}
	return reviews.UpdateResult{}, failure
}

func (s reviewService) modelTools() ([]tool.Tool, error) {
	list, err := tool.NewFunc(tool.FuncConfig{Name: listReviewsTool, Description: s.contracts.List.Descriptor().Description}, func(ctx context.Context, _ struct{}) (reviews.ListResult, error) {
		return s.read(ctx)
	})
	if err != nil {
		return nil, err
	}
	update, err := tool.NewFunc(tool.FuncConfig{Name: updateReviewTool, Description: s.contracts.Update.Descriptor().Description + " Runtime supplies the invocation identity; do not add a retry key to arguments."}, s.updateFromModel)
	if err != nil {
		return nil, err
	}
	analyze, err := tool.NewFunc(tool.FuncConfig{Name: analyzeReviewTool, Description: s.contracts.Analyzers.Descriptor().Description}, func(ctx context.Context, _ struct{}) (reviews.Analysis, error) {
		return s.analyze(ctx)
	})
	if err != nil {
		return nil, err
	}
	return []tool.Tool{list, update, analyze}, nil
}

func (s reviewService) register(backend *pluginmcp.Backend) error {
	receipts, err := pluginmcp.ReceiptQuery()
	if err != nil {
		return err
	}
	return errors.Join(
		receipts.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[pluginmcp.ReceiptRequest]) (pluginmcp.Receipt, error) {
			return s.store.receipt(ctx, command.Input, s.contracts.Update.Name())
		}),
		s.contracts.List.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, _ pluginmcp.Command[struct{}]) (reviews.ListResult, error) {
			return s.read(ctx)
		}),
		s.contracts.Update.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[reviews.Update]) (reviews.UpdateResult, error) {
			admitted, err := command.RequestInput()
			if err != nil {
				return reviews.UpdateResult{}, err
			}
			return s.store.update(ctx, string(command.ID), admitted)
		}),
		s.contracts.Analyzers.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, _ pluginmcp.Command[struct{}]) (reviews.Analysis, error) {
			return s.analyze(ctx)
		}),
	)
}

func admitSetup(_ context.Context, setup pluginmcp.Setup) (io.Closer, error) {
	values := setup.Contributions()
	required, err := reviews.NewAnalyzerContracts()
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		if value.Export != capabilityExport || len(value.Requires) != 1 || value.Requires[0].ID != analyzersDependency || !value.Requires[0].Optional {
			return nil, fmt.Errorf("reviews: named optional analyzers dependency is required")
		}
		actual, err := plugincontract.AdmitCapability(value.Requires[0].Contract)
		if err != nil {
			return nil, err
		}
		if actual.Identity() != required.Capability.Identity() {
			return nil, fmt.Errorf("reviews: analyzer setup contract differs from the consumer")
		}
	}
	return nil, nil
}

func rejected(message string) error {
	failure, err := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindRejected, Output: tool.NewTextOutput(message)})
	if err != nil {
		return err
	}
	return failure
}
