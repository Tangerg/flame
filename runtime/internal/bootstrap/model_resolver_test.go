package bootstrap

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chatclient"
)

type staticTestChatResolver struct {
	client *chatclient.Client
}

func testChatResolver(client *chatclient.Client) ChatResolver {
	return staticTestChatResolver{client: client}
}

func (r staticTestChatResolver) ResolveChat(
	_ context.Context,
	selection modelref.Selection,
) (*chatclient.Client, error) {
	if err := selection.Validate(); err != nil {
		return nil, err
	}
	if r.client == nil {
		return nil, errors.New("bootstrap test chat resolver has no client")
	}
	return r.client, nil
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
