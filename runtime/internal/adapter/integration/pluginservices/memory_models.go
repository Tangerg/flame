package pluginservices

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"

	framecall "github.com/Tangerg/frame/call"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

// Memory retains its own external-call budget; the model exchange is the public
// Scope request/response contract, while its corpus embedding is a separate method.
func GrantMemoryModels(ctx context.Context, callbacks *pluginmcp.Callbacks, session *sdk.ClientSession, model chat.Model, embed models.TextEmbedder) (pluginmcp.CallbackRef, func(), error) {
	if callbacks == nil {
		return pluginmcp.CallbackRef{}, nil, errors.New("memory: callback owner is required")
	}
	if model != nil && lo.IsNil(model) || embed != nil && lo.IsNil(embed) {
		return pluginmcp.CallbackRef{}, nil, errors.New("memory: model and embedding collaborators cannot be typed nil")
	}
	admission := newExternalCallLedger("memory", memorycontract.MaxModelCalls)
	completion, err := memoryModelCallback(model, admission)
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	embedding, err := memoryEmbeddingCallback(embed, admission)
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	return callbacks.Open(ctx, session, pluginmcp.CallbackLimits{MaxRequestBytes: memorycontract.MaxCallbackInputBytes, MaxResponseBytes: memorycontract.MaxCallbackOutputBytes}, completion, embedding)
}

// memoryModelCallback admits one model call per command within the memory
// envelope; without a model every call is refused.
func memoryModelCallback(model chat.Model, admission *externalCallLedger) (pluginmcp.Callback, error) {
	return pluginsdk.NewModelCallback(func(ctx context.Context, command pluginmcp.Command[scopewire.Request]) (*chat.Response, error) {
		request := chat.Request(command.Input)
		if lo.IsNil(model) || !memoryModelRequests.admits(request) {
			return nil, &framecall.Refusal{Code: refusalModelEnvelope, Message: "memory: model envelope refused"}
		}
		input, err := command.RequestInput()
		if err != nil {
			return nil, err
		}
		if err := admission.admit(string(command.ID), externalModelCall, input); err != nil {
			return nil, err
		}
		return model.Call(ctx, request.Clone())
	})
}

// memoryEmbeddingCallback admits bounded embedding batches; without an
// embedder every call is refused.
func memoryEmbeddingCallback(embed models.TextEmbedder, admission *externalCallLedger) (pluginmcp.Callback, error) {
	return pluginsdk.NewTextEmbeddingCallback(func(ctx context.Context, command pluginmcp.Command[[]string]) (pluginsdk.TextEmbeddings, error) {
		texts := command.Input
		input, err := command.RequestInput()
		if err != nil {
			return pluginsdk.TextEmbeddings{}, err
		}
		if lo.IsNil(embed) || len(texts) == 0 || len(texts) > memorycontract.MaxEmbeddingTexts || int64(len(input.Bytes())) > memorycontract.MaxEmbeddingInputBytes {
			return pluginsdk.TextEmbeddings{}, &framecall.Refusal{Code: refusalEmbeddingEnvelope, Message: "memory: embedding envelope refused"}
		}
		if err := admission.admit(string(command.ID), externalEmbeddingCall, input); err != nil {
			return pluginsdk.TextEmbeddings{}, err
		}
		vectors, err := embed.Embed(ctx, texts)
		return pluginsdk.TextEmbeddings{Space: embed.ID(), Vectors: vectors}, err
	})
}

var memoryModelRequests = auxiliaryTextRequest{maxOutputTokens: memorycontract.MaxModelOutputTokens, maxInputBytes: memorycontract.MaxModelInputBytes}
