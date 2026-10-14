package rpc

import (
	"context"
	"errors"
	"strings"

	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/google/uuid"
)

type Prompt struct {
	Operation, SystemPrompt, UserPrompt string
	MaxInputBytes                       int
	MaxOutputTokens                     int64
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
