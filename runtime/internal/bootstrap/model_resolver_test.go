package bootstrap

import (
	"context"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chatclient"
)

type staticTestChatResolver struct {
	client *chatclient.Client
}

func testChatResolver(client chatclient.Client) ChatResolver {
	return staticTestChatResolver{client: &client}
}

func (r staticTestChatResolver) ResolveChat(
	_ context.Context,
	selection modelref.Selection,
) (modeladapter.ResolvedChat, error) {
	if err := selection.Validate(); err != nil {
		return modeladapter.ResolvedChat{}, err
	}
	return modeladapter.NewResolvedChat(r.client, nil)
}

func (r staticTestChatResolver) ValidateChatModel(
	ctx context.Context,
	providerID,
	model string,
) error {
	selection, err := modelref.New(providerID, model)
	if err != nil {
		return err
	}
	_, err = r.ResolveChat(ctx, selection)
	return err
}
