package main

import (
	"context"
	"errors"
	"strings"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type remoteInputs struct{}

func (remoteInputs) Conversation(ctx context.Context) (skilllearning.Conversation, error) {
	grant, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return skilllearning.Conversation{}, err
	}
	method, err := skilllearning.ConversationMethod()
	if err != nil {
		return skilllearning.Conversation{}, err
	}
	return pluginmcp.CallHost(ctx, grant, method, "", struct{}{})
}

func (remoteInputs) ReadSkill(ctx context.Context, name string) (skilllearning.ReadResult, error) {
	grant, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return skilllearning.ReadResult{}, err
	}
	method, err := skilllearning.ReadSkillMethod()
	if err != nil {
		return skilllearning.ReadResult{}, err
	}
	return pluginmcp.CallHost(ctx, grant, method, "", skilllearning.ReadSkill{Name: name})
}

func (remoteInputs) Complete(ctx context.Context, system, user string) (string, error) {
	if len(system)+len(user) > skilllearning.MaxModelInputBytes {
		return "", errors.New("skill learning: model input exceeds its limit")
	}
	grant, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return "", err
	}
	client, err := chatclient.New(chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		return pluginsdk.CallModel(ctx, grant, pluginmcp.NewCommandID(), request)
	}))
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

func admitMined(ctx context.Context, proposal skilllearning.Proposal) (bool, error) {
	grant, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return false, err
	}
	method, err := skilllearning.AdmitMethod()
	if err != nil {
		return false, err
	}
	admission, err := pluginmcp.CallHost(ctx, grant, method, "", proposal)
	return admission.Accepted, err
}
