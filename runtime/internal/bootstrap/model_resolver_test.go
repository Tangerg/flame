package bootstrap

import (
	"context"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/scope/core/chatclient"
)

type staticTestChatResolver struct {
	client *chatclient.Client
}

func testChatResolver(client chatclient.Client) chatResolver {
	return staticTestChatResolver{client: &client}
}

func (r staticTestChatResolver) ResolveChat(
	_ context.Context,
	selection modelchoice.Selection,
) (modeladapter.ResolvedChat, error) {
	return modeladapter.NewResolvedChat(r.client, nil)
}

func (r staticTestChatResolver) ValidateChatModel(
	ctx context.Context,
	providerID,
	model string,
) error {
	selection, err := modelchoice.New(providerID, model)
	if err != nil {
		return err
	}
	_, err = r.ResolveChat(ctx, selection)
	return err
}
