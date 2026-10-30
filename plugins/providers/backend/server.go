package main

import (
	"context"
	"errors"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func newServer(store *store) (*sdk.Server, error) {
	if store == nil || store.db == nil {
		return nil, errors.New("model provider: private store is required")
	}
	contract, err := modelprovider.Contract()
	if err != nil {
		return nil, err
	}
	profiles, err := modelprovider.ProfilesMethod()
	if err != nil {
		return nil, err
	}
	remote, err := modelprovider.RemoteModelsMethod()
	if err != nil {
		return nil, err
	}
	openChat, err := modelprovider.OpenChatMethod()
	if err != nil {
		return nil, err
	}
	openEmbedding, err := modelprovider.OpenEmbeddingMethod()
	if err != nil {
		return nil, err
	}
	reopen, err := modelprovider.ReopenChatMethod()
	if err != nil {
		return nil, err
	}
	close, err := modelprovider.CloseMethod()
	if err != nil {
		return nil, err
	}
	call, err := modelprovider.CallMethod()
	if err != nil {
		return nil, err
	}
	count, err := modelprovider.CountMethod()
	if err != nil {
		return nil, err
	}
	embed, err := modelprovider.EmbedMethod()
	if err != nil {
		return nil, err
	}
	stream, err := modelprovider.StreamMethod()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.providers", Version: "1.0.0"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, store.db),
		profiles.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, qimumcp.Command[struct{}]) ([]modelprovider.Profile, error) {
			return providers.published(), nil
		}),
		remote.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[modelprovider.Connection]) ([]string, error) {
			spec, err := newClientSpec(command.Input)
			if err != nil {
				return nil, &portable.Refusal{Code: "invalid_connection", Message: "model provider: invalid connection"}
			}
			values, err := listModels(ctx, spec)
			if err != nil {
				return nil, &portable.Refusal{Code: string(failure(err).Kind), Message: "model provider: model discovery failed"}
			}
			return values, nil
		}),
		openChat.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[modelprovider.Open]) (modelprovider.Chat, error) {
			return openClient(ctx, store, openChat.Name(), "chat", command, func(handle modelprovider.Handle, client *client, input modelprovider.Open) modelprovider.Chat {
				return chatProjection(handle, client, input, command.ID)
			})
		}),
		reopen.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[modelprovider.Reopen]) (modelprovider.Chat, error) {
			return store.reopenChat(ctx, reopen.Name(), openChat.Name(), command)
		}),
		openEmbedding.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[modelprovider.Open]) (modelprovider.Embedder, error) {
			return openClient(ctx, store, openEmbedding.Name(), "embedding", command, func(handle modelprovider.Handle, _ *client, input modelprovider.Open) modelprovider.Embedder {
				return modelprovider.Embedder{Handle: handle, Provider: input.Connection.Provider, Model: input.Model, Space: embeddingSpaceID(input.Connection.Provider, input.Model, input.Connection.BaseURL)}
			})
		}),
		close.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[modelprovider.Handle]) (struct{}, error) {
			return execute(ctx, store, close.Name(), command, modelprovider.Handle.Validate, func(ctx context.Context, input modelprovider.Handle) (struct{}, func(), error) {
				return struct{}{}, nil, store.closeClient(ctx, input)
			})
		}),
		call.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[modelprovider.ChatRequest]) (modelprovider.ChatResult, error) {
			return execute(ctx, store, call.Name(), command, modelprovider.ChatRequest.Validate, store.call)
		}),
		count.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[modelprovider.ChatRequest]) (modelprovider.TokenCount, error) {
			return execute(ctx, store, count.Name(), command, modelprovider.ChatRequest.Validate, store.count)
		}),
		embed.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[modelprovider.EmbeddingRequest]) (modelprovider.EmbeddingResult, error) {
			return execute(ctx, store, embed.Name(), command, func(input modelprovider.EmbeddingRequest) error { _, err := input.Native(); return err }, store.embed)
		}),
		stream.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[modelprovider.ChatRequest]) iter.Seq2[qimumcp.StreamValue[chat.ResponseDelta, modelprovider.ChatResult], error] {
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
