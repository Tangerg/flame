package pluginservices

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

type memoryModelAdmission struct {
	mu          sync.Mutex
	invocations map[string]memoryModelInvocation
}

type memoryModelInvocation struct {
	method string
	input  jsonvalue.Value
}

func (a *memoryModelAdmission) admit(id, method string, input jsonvalue.Value) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if original, found := a.invocations[id]; found {
		if original != (memoryModelInvocation{method: method, input: input}) {
			return &extension.Refusal{Code: "command_conflict", Message: extension.ErrInvocationConflict.Error()}
		}
		return &extension.Refusal{Code: "model_redelivery", Message: "memory: original model invocation cannot be redelivered"}
	}
	if len(a.invocations) == memorycontract.MaxModelCalls {
		return &extension.Refusal{Code: "model_budget", Message: "memory: model budget exhausted"}
	}
	// Providers have no owner receipt. Fence each identity before external I/O.
	a.invocations[id] = memoryModelInvocation{method: method, input: input}
	return nil
}

// Memory retains its own external-call budget; the model exchange is the public
// Scope request/response contract, while its corpus embedding is a separate method.
func GrantMemoryModels(ctx context.Context, callbacks *pluginsdk.Callbacks, session *sdk.ClientSession, model chat.Model, embed models.TextEmbedder) (pluginsdk.CallbackRef, func(), error) {
	if callbacks == nil {
		return pluginsdk.CallbackRef{}, nil, errors.New("memory: callback owner is required")
	}
	admission := &memoryModelAdmission{invocations: make(map[string]memoryModelInvocation)}
	completion, err := pluginsdk.NewModelCallback(func(ctx context.Context, command pluginsdk.Command[chat.Request]) (*chat.Response, error) {
		request := command.Input
		if lo.IsNil(model) || !memoryModelRequest(request) {
			return nil, &extension.Refusal{Code: "model_envelope", Message: "memory: model envelope refused"}
		}
		input, err := command.RequestInput()
		if err != nil {
			return nil, err
		}
		if err := admission.admit(command.ID, "model", input); err != nil {
			return nil, err
		}
		return model.Call(ctx, request.Clone())
	})
	if err != nil {
		return pluginsdk.CallbackRef{}, nil, err
	}
	embedding, err := pluginsdk.NewTextEmbeddingCallback(func(ctx context.Context, command pluginsdk.Command[[]string]) (pluginsdk.TextEmbeddings, error) {
		texts := command.Input
		input, err := command.RequestInput()
		if err != nil {
			return pluginsdk.TextEmbeddings{}, err
		}
		if lo.IsNil(embed) || len(texts) == 0 || len(texts) > memorycontract.MaxEmbeddingTexts || int64(len(input.JSON())) > memorycontract.MaxEmbeddingInputBytes {
			return pluginsdk.TextEmbeddings{}, &extension.Refusal{Code: "embedding_envelope", Message: "memory: embedding envelope refused"}
		}
		if err := admission.admit(command.ID, "embedding", input); err != nil {
			return pluginsdk.TextEmbeddings{}, err
		}
		vectors, err := embed.Embed(ctx, texts)
		return pluginsdk.TextEmbeddings{Space: embed.ID(), Vectors: vectors}, err
	})
	if err != nil {
		return pluginsdk.CallbackRef{}, nil, err
	}
	return callbacks.Open(ctx, session, pluginsdk.CallbackLimits{MaxRequestBytes: memorycontract.MaxCallbackInputBytes, MaxResponseBytes: memorycontract.MaxCallbackOutputBytes}, completion, embedding)
}

func memoryModelRequest(request chat.Request) bool {
	options := request.Options
	if options.MaxOutputTokens == nil || *options.MaxOutputTokens <= 0 || *options.MaxOutputTokens > memorycontract.MaxModelOutputTokens || len(request.Messages) != 2 || request.Messages[0].Role != chat.RoleSystem || request.Messages[1].Role != chat.RoleUser || len(request.Tools) != 0 || request.ToolChoice != nil || options.Model != "" {
		return false
	}
	if options.OutputFormat == nil || options.OutputFormat.Type != chat.OutputFormatText {
		return false
	}
	options.MaxOutputTokens, options.OutputFormat = nil, nil
	if !reflect.DeepEqual(options, chat.Options{}) {
		return false
	}
	for _, message := range request.Messages {
		for _, part := range message.Parts {
			if part.Kind != chat.PartText {
				return false
			}
		}
	}
	return len(request.Messages[0].Text())+len(request.Messages[1].Text()) <= memorycontract.MaxModelInputBytes
}
