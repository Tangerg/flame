package rpc

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/idempotency"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/google/uuid"
)

const MaximumModelInput = 512 * 1024
const MaximumModelOutput int64 = 2048

// Six bytes per code point cover worst-case JSON escaping across both active targets.
const maximumEmbeddingInput = memory.MaxVisiblePerTarget*2*(memory.MaxContentCharacters*6+3) + 1

const maximumModelEnvelope = maximumEmbeddingInput + 64*1024
const maximumModelResult = 64 * 1024 * 1024

type Embeddings interface {
	ID() string
	Embed(context.Context, []string) ([][]float32, error)
}
type Prompt struct {
	Operation, SystemPrompt, UserPrompt string
	MaxInputBytes                       int
	MaxOutputTokens                     int64
}

type modelAdmission struct {
	mu          sync.Mutex
	invocations map[string]modelInvocation
}

type modelInvocation struct {
	method string
	input  jsoninput.Input
}

func (a *modelAdmission) admit(id, method string, input jsoninput.Input) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if original, found := a.invocations[id]; found {
		if original != (modelInvocation{method: method, input: input}) {
			return &capability.Refusal{Code: "command_conflict", Message: idempotency.ErrKeyConflict.Error()}
		}
		return &capability.Refusal{Code: "model_redelivery", Message: "memory: original model invocation cannot be redelivered"}
	}
	if len(a.invocations) == 2 {
		return &capability.Refusal{Code: "model_budget", Message: "memory: model budget exhausted"}
	}
	// Providers have no owner receipt. Fence each identity before external I/O.
	a.invocations[id] = modelInvocation{method: method, input: input}
	return nil
}

// Memory retains its own external-call budget; the model exchange is the public
// Scope request/response contract, while its corpus embedding is a separate method.
func GrantModels(ctx context.Context, callbacks *pluginsdk.Callbacks, session *sdk.ClientSession, model chat.Model, embed Embeddings) (pluginsdk.CallbackRef, func(), error) {
	if callbacks == nil {
		return pluginsdk.CallbackRef{}, nil, errors.New("memory: callback owner is required")
	}
	admission := &modelAdmission{invocations: make(map[string]modelInvocation)}
	completion, err := pluginsdk.NewModelCallback(func(ctx context.Context, command pluginsdk.Command[chat.Request]) (*chat.Response, error) {
		request := command.Input
		if dependency.Missing(model) || !memoryModelRequest(request) {
			return nil, &capability.Refusal{Code: "model_envelope", Message: "memory: model envelope refused"}
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
	embedding, err := pluginsdk.NewTextEmbeddingCallback(func(ctx context.Context, command pluginsdk.Command[[]string]) ([][]float32, error) {
		texts := command.Input
		input, err := command.RequestInput()
		if err != nil {
			return nil, err
		}
		if dependency.Missing(embed) || len(texts) == 0 || len(texts) > memory.MaxVisiblePerTarget*2 || len(input.JSON()) > maximumEmbeddingInput {
			return nil, &capability.Refusal{Code: "embedding_envelope", Message: "memory: embedding envelope refused"}
		}
		if err := admission.admit(command.ID, "embedding", input); err != nil {
			return nil, err
		}
		return embed.Embed(ctx, texts)
	})
	if err != nil {
		return pluginsdk.CallbackRef{}, nil, err
	}
	return callbacks.Open(ctx, session, pluginsdk.CallbackLimits{MaxRequestBytes: maximumModelEnvelope, MaxResponseBytes: maximumModelResult}, completion, embedding)
}

func memoryModelRequest(request chat.Request) bool {
	options := request.Options
	if options.MaxOutputTokens == nil || *options.MaxOutputTokens <= 0 || *options.MaxOutputTokens > MaximumModelOutput || len(request.Messages) != 2 || request.Messages[0].Role != chat.RoleSystem || request.Messages[1].Role != chat.RoleUser || len(request.Tools) != 0 || request.ToolChoice != nil || options.Model != "" {
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
	return len(request.Messages[0].Text())+len(request.Messages[1].Text()) <= MaximumModelInput
}

type RemoteModels struct {
	Grant pluginsdk.CallbackRef
	Space string
}

func (r RemoteModels) Complete(ctx context.Context, prompt Prompt) (string, error) {
	if prompt.Operation != "memory.extract" && prompt.Operation != "memory.curate" || prompt.MaxInputBytes <= 0 || prompt.MaxInputBytes > MaximumModelInput || prompt.MaxOutputTokens <= 0 || prompt.MaxOutputTokens > MaximumModelOutput || len(prompt.SystemPrompt)+len(prompt.UserPrompt) > prompt.MaxInputBytes {
		return "", errors.New("memory: completion prompt refused")
	}
	client, err := chatclient.New(chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		return pluginsdk.CallModel(ctx, r.Grant, uuid.NewString(), request)
	}), chatclient.Config{})
	if err != nil {
		return "", err
	}
	text, err := client.Output(ctx, &chat.Request{Messages: []chat.Message{chat.NewSystemMessage(prompt.SystemPrompt), chat.NewUserMessage(chat.NewTextPart(prompt.UserPrompt))}, Options: chat.Options{MaxOutputTokens: &prompt.MaxOutputTokens}}, chatclient.Text())
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("memory: completion returned blank text")
	}
	return text, nil
}
func (r RemoteModels) ID() string { return r.Space }
func (r RemoteModels) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return pluginsdk.CallTextEmbedding(ctx, r.Grant, uuid.NewString(), texts)
}
