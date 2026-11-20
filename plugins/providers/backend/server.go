package main

import (
	"context"
	"errors"
	"iter"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"

	framecall "github.com/Tangerg/frame/call"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func newServer(store *store, deployments azureDeployments) (*sdk.Server, error) {
	if store == nil || !store.db.Valid() {
		return nil, errors.New("model provider: private store is required")
	}
	contract, err := modelprovider.Contract()
	if err != nil {
		return nil, err
	}
	var errs []error
	profiles := lookupMethod(modelprovider.ProfilesMethod, &errs)
	remote := lookupMethod(modelprovider.RemoteModelsMethod, &errs)
	verify := lookupMethod(modelprovider.VerifyMethod, &errs)
	openChat := lookupMethod(modelprovider.OpenChatMethod, &errs)
	openEmbedding := lookupMethod(modelprovider.OpenEmbeddingMethod, &errs)
	reopen := lookupMethod(modelprovider.ReopenChatMethod, &errs)
	close := lookupMethod(modelprovider.CloseMethod, &errs)
	call := lookupMethod(modelprovider.CallMethod, &errs)
	count := lookupMethod(modelprovider.CountMethod, &errs)
	embed := lookupMethod(modelprovider.EmbedMethod, &errs)
	stream := lookupMethod(modelprovider.StreamMethod, &errs)
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.providers", Version: "1.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, store.db),
		profiles.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, pluginmcp.Command[struct{}]) ([]modelprovider.Profile, error) {
			return providers.published(), nil
		}),
		remote.RegisterServer(backend, nil, discovery{deployments: deployments}.remoteModels),
		verify.RegisterServer(backend, nil, discovery{deployments: deployments}.verify),
		openChat.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[modelprovider.Open]) (modelprovider.Chat, error) {
			return openClient(ctx, store, openChat.Name(), chatClient, command, func(handle modelprovider.Handle, client *client, input modelprovider.Open) modelprovider.Chat {
				return chatProjection(handle, client, input, command.ID)
			})
		}),
		reopen.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[modelprovider.Reopen]) (modelprovider.Chat, error) {
			return store.reopenChat(ctx, reopen.Name(), openChat.Name(), command)
		}),
		openEmbedding.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[modelprovider.Open]) (modelprovider.Embedder, error) {
			return openClient(ctx, store, openEmbedding.Name(), embeddingClient, command, func(handle modelprovider.Handle, _ *client, input modelprovider.Open) modelprovider.Embedder {
				return modelprovider.Embedder{Handle: handle, Provider: input.Connection.Provider, Model: input.Model, Space: embeddingSpaceID(input.Connection.Provider, input.Model, input.Connection.BaseURL)}
			})
		}),
		close.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[modelprovider.Handle]) (struct{}, error) {
			return execute(ctx, store, close.Name(), command, modelprovider.Handle.Validate, func(ctx context.Context, input modelprovider.Handle) (struct{}, func(), error) {
				return struct{}{}, nil, store.closeClient(ctx, input)
			})
		}),
		call.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[modelprovider.ChatRequest]) (modelprovider.ChatResult, error) {
			return execute(ctx, store, call.Name(), command, modelprovider.ChatRequest.Validate, store.call)
		}),
		count.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[modelprovider.ChatRequest]) (modelprovider.TokenCount, error) {
			return execute(ctx, store, count.Name(), command, modelprovider.ChatRequest.Validate, store.count)
		}),
		embed.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[modelprovider.EmbeddingRequest]) (modelprovider.EmbeddingResult, error) {
			return execute(ctx, store, embed.Name(), command, func(input modelprovider.EmbeddingRequest) error { _, err := input.Native(); return err }, store.embed)
		}),
		stream.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[modelprovider.ChatRequest]) iter.Seq2[pluginmcp.StreamValue[scopewire.ResponseDelta, modelprovider.StreamReceipt], error] {
			return store.stream(ctx, stream.Descriptor().Name, command)
		}),
		backend.BindCapability("model-provider", contract),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}

// lookupMethod resolves one published method, collecting a failure so every
// method can be declared before checking once.
func lookupMethod[M any](resolve func() (M, error), errs *[]error) M {
	method, err := resolve()
	if err != nil {
		*errs = append(*errs, err)
	}
	return method
}

// discovery answers connection checks without opening a client handle.
type discovery struct{ deployments azureDeployments }

func (d discovery) remoteModels(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[modelprovider.Connection]) ([]string, error) {
	spec, err := newClientSpec(command.Input)
	if err != nil {
		return nil, invalidConnection()
	}
	values, err := listModels(ctx, spec, d.deployments)
	if err != nil {
		return nil, &framecall.Refusal{Code: string(failure(err).Kind), Message: "model provider: model discovery failed"}
	}
	return values, nil
}

func (d discovery) verify(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[modelprovider.Connection]) (struct{}, error) {
	spec, err := newClientSpec(command.Input)
	if err != nil {
		return struct{}{}, invalidConnection()
	}
	if err := verifyConnection(ctx, spec); err != nil {
		return struct{}{}, &framecall.Refusal{Code: string(failure(err).Kind), Message: "model provider: connection verification failed"}
	}
	return struct{}{}, nil
}

func invalidConnection() error {
	return &framecall.Refusal{Code: pluginmcp.InvalidInputCode, Message: "model provider: invalid connection"}
}
