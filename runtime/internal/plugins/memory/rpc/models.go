package rpc

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"sync"

	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const maximumModelInput = 512 * 1024

// Six bytes per code point cover worst-case JSON escaping across both active targets.
const maximumEmbeddingInput = memory.MaxVisiblePerTarget*2*(memory.MaxContentCharacters*6+3) + 1

// Embedding JSON is encoded again as a string, so the outer envelope needs its escaped size.
const maximumModelEnvelope = maximumEmbeddingInput*2 + 64*1024
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
type Completion func(context.Context, Prompt) (string, error)

func (c Completion) Complete(ctx context.Context, prompt Prompt) (string, error) {
	return c(ctx, prompt)
}

func modelMethod() (*pluginsdk.Method[Prompt, string], error) {
	return pluginsdk.NewMethod[Prompt, string](pluginsdk.MethodConfig{
		Name: "flame.memory/model.v1", Kind: pluginsdk.Query, Target: "call",
		Description: "Use the model capability captured by this Memory invocation.",
	})
}

// Memory owns this two-call policy. The public callback endpoint has no model
// vocabulary or budget and other operations may bind their own collaborators.
func GrantModels(ctx context.Context, callbacks *pluginsdk.Callbacks, session *sdk.ClientSession, complete Completion, embed Embeddings) (pluginsdk.CallbackRef, func(), error) {
	if callbacks == nil {
		return pluginsdk.CallbackRef{}, nil, errors.New("memory: callback owner is required")
	}
	method, err := modelMethod()
	if err != nil {
		return pluginsdk.CallbackRef{}, nil, err
	}
	var mu sync.Mutex
	remaining := 2
	callback, err := pluginsdk.NewCallback(method, nil, func(ctx context.Context, command pluginsdk.Command[Prompt]) (string, error) {
		prompt := command.Input
		limit := maximumModelInput
		if prompt.Operation == "memory.embed" {
			limit = maximumEmbeddingInput
		}
		if prompt.MaxInputBytes <= 0 || prompt.MaxInputBytes > limit || prompt.MaxOutputTokens <= 0 || prompt.MaxOutputTokens > 2048 || len(prompt.SystemPrompt)+len(prompt.UserPrompt) > prompt.MaxInputBytes {
			return "", errors.New("memory: model envelope refused")
		}
		mu.Lock()
		if remaining == 0 {
			mu.Unlock()
			return "", errors.New("memory: model budget exhausted")
		}
		remaining--
		mu.Unlock()
		switch prompt.Operation {
		case "memory.extract", "memory.curate":
			if complete == nil {
				return "", errors.New("memory: completion was not granted")
			}
			return complete(ctx, prompt)
		case "memory.embed":
			if embed == nil {
				return "", errors.New("memory: embedding was not granted")
			}
			var texts []string
			if err := json.Unmarshal([]byte(prompt.UserPrompt), &texts, json.RejectUnknownMembers(true)); err != nil {
				return "", err
			}
			if len(texts) == 0 || len(texts) > memory.MaxVisiblePerTarget*2 {
				return "", errors.New("memory: embedding count refused")
			}
			vectors, err := embed.Embed(ctx, texts)
			if err != nil {
				return "", err
			}
			encoded, err := json.Marshal(vectors)
			return string(encoded), err
		default:
			return "", errors.New("memory: model operation refused")
		}
	})
	if err != nil {
		return pluginsdk.CallbackRef{}, nil, err
	}
	return callbacks.Open(ctx, session, pluginsdk.CallbackLimits{MaxRequestBytes: maximumModelEnvelope, MaxResponseBytes: maximumModelResult}, callback)
}

type RemoteModels struct {
	Grant pluginsdk.CallbackRef
	Space string
}

func (r RemoteModels) Complete(ctx context.Context, prompt Prompt) (string, error) {
	method, err := modelMethod()
	if err != nil {
		return "", err
	}
	return pluginsdk.CallHost(ctx, r.Grant, method, "", prompt)
}
func (r RemoteModels) ID() string { return r.Space }
func (r RemoteModels) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	encoded, err := json.Marshal(texts)
	if err != nil {
		return nil, err
	}
	text, err := r.Complete(ctx, Prompt{Operation: "memory.embed", UserPrompt: string(encoded), MaxInputBytes: maximumEmbeddingInput, MaxOutputTokens: 2048})
	if err != nil {
		return nil, err
	}
	var vectors [][]float32
	err = json.Unmarshal([]byte(text), &vectors)
	return vectors, err
}
