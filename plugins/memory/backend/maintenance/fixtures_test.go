package maintenance

import (
	"context"
	"errors"

	rpc "github.com/Tangerg/flame/plugins/memory/backend/rpc"
	modeltext "github.com/Tangerg/flame/packages/go-sdk/model/text"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func intPointer(value int) *int { return &value }

type conversationFixture struct{ messages map[string][]chat.Message }

func newConversationFixture() *conversationFixture {
	return &conversationFixture{messages: make(map[string][]chat.Message)}
}

func (h *conversationFixture) Write(_ context.Context, id string, messages ...chat.Message) error {
	h.messages[id] = append(h.messages[id], messages...)
	return nil
}

func (h *conversationFixture) Read(_ context.Context, id string) ([]chat.Message, error) {
	return h.messages[id], nil
}

type capturedHistory struct{ source *conversationFixture }

func (h capturedHistory) Read(ctx context.Context, id string) (Conversation, error) {
	messages, err := h.source.Read(ctx, id)
	return Conversation{len(messages), modeltext.RenderTranscript(messages)}, err
}

type capturedUtility struct{ model chat.Model }

func capturedCompletion(client chat.Model) capturedUtility {
	return capturedUtility{model: client}
}

func (c capturedUtility) Complete(ctx context.Context, prompt rpc.Prompt) (string, error) {
	if len(prompt.SystemPrompt)+len(prompt.UserPrompt) > prompt.MaxInputBytes {
		return "", errors.New("fixture: prompt exceeds its captured input bound")
	}
	client, err := chatclient.New(c.model, chatclient.Config{})
	if err != nil {
		return "", err
	}
	return client.Output(ctx, &chat.Request{Messages: []chat.Message{chat.NewSystemMessage(prompt.SystemPrompt), chat.NewUserMessage(chat.NewTextPart(prompt.UserPrompt))}, Options: chat.Options{MaxOutputTokens: &prompt.MaxOutputTokens}}, chatclient.Text())
}
