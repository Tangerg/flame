package main

import (
	"context"
	"errors"
	"strings"

	app "github.com/Tangerg/flame/plugins/memory/internal/application"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/google/uuid"
)

type remoteModels struct {
	grant pluginsdk.CallbackRef
	space string
}

func (r remoteModels) Complete(ctx context.Context, prompt app.Prompt) (string, error) {
	if err := prompt.Validate(); err != nil {
		return "", err
	}
	client, err := chatclient.New(chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		return pluginsdk.CallModel(ctx, r.grant, uuid.NewString(), request)
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
func (r remoteModels) ID() string { return r.space }
func (r remoteModels) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	result, err := pluginsdk.CallTextEmbedding(ctx, r.grant, uuid.NewString(), texts)
	if err != nil {
		return nil, err
	}
	if result.Space != r.space {
		return nil, errors.New("memory: embedding space differs from the captured model")
	}
	return result.Vectors, nil
}
