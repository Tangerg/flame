package main

import (
	"context"
	"errors"
	"strings"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/google/uuid"
)

type remoteInputs struct{}

func (remoteInputs) Conversation(ctx context.Context) (skilllearning.Conversation, error) {
	grant, err := qimumcp.CallbackFromContext(ctx)
	if err != nil {
		return skilllearning.Conversation{}, err
	}
	method, err := skilllearning.ConversationMethod()
	if err != nil {
		return skilllearning.Conversation{}, err
	}
	return qimumcp.CallHost(ctx, grant, method, "", struct{}{})
}

func (remoteInputs) ReadSkill(ctx context.Context, name string) (*skilllearning.Skill, error) {
	grant, err := qimumcp.CallbackFromContext(ctx)
	if err != nil {
		return nil, err
	}
	method, err := skilllearning.ReadSkillMethod()
	if err != nil {
		return nil, err
	}
	result, err := qimumcp.CallHost(ctx, grant, method, "", skilllearning.ReadSkill{Name: name})
	return result.Skill, err
}

func (remoteInputs) Complete(ctx context.Context, system, user string) (string, error) {
	if len(system)+len(user) > skilllearning.MaxModelInputBytes {
		return "", errors.New("skill learning: model input exceeds its limit")
	}
	grant, err := qimumcp.CallbackFromContext(ctx)
	if err != nil {
		return "", err
	}
	client, err := chatclient.New(chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		return pluginsdk.CallModel(ctx, grant, uuid.NewString(), request)
	}), chatclient.Config{})
	if err != nil {
		return "", err
	}
	outputTokens := skilllearning.MaxModelOutputTokens
	text, err := client.Output(ctx, &chat.Request{Messages: []chat.Message{chat.NewSystemMessage(system), chat.NewUserMessage(chat.NewTextPart(user))}, Options: chat.Options{MaxOutputTokens: &outputTokens}}, chatclient.Text())
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("skill learning: model returned blank text")
	}
	return strings.TrimSpace(text), nil
}
