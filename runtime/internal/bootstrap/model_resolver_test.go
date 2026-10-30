package bootstrap

import (
	"context"

	"errors"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/models/catalog"
)

type staticTestChatResolver struct {
	client *chatclient.Client
}

func testChatResolver(client chatclient.Client) modelProviderAdapters {
	return staticTestChatResolver{client: &client}
}

func (r staticTestChatResolver) OpenChat(
	_ context.Context,
	selection modelchoice.Selection,
	_ modelprovider.Connection,
) (modeladapter.ResolvedChat, error) {
	return modeladapter.NewResolvedChat(r.client, nil, testsupport.ModelFacts(selection))
}

func (r staticTestChatResolver) RestoreChat(ctx context.Context, selection modelchoice.Selection, _ *modeladapter.ProviderCapture) (modeladapter.ResolvedChat, error) {
	return r.OpenChat(ctx, selection, modelprovider.Connection{})
}
func (r staticTestChatResolver) Profiles(context.Context) ([]modelprovider.Profile, error) {
	var result []modelprovider.Profile
	for _, id := range []string{"anthropic", "openai", "deepseek", "alibaba"} {
		entries := catalog.Default.Models(id)
		if len(entries) == 0 {
			continue
		}
		result = append(result, modelprovider.Profile{ID: id, Authentication: modelprovider.KeyOptional, ModelSource: modelprovider.BundledModels, DefaultModel: entries[0].ID, Models: entries})
	}
	return result, nil
}
func (staticTestChatResolver) RemoteModels(context.Context, modelprovider.Connection) ([]string, error) {
	return nil, errors.New("unexpected model discovery")
}
func (staticTestChatResolver) OpenEmbedding(context.Context, modelchoice.Selection, modelprovider.Connection) (modeladapter.ResolvedEmbedding, error) {
	return modeladapter.ResolvedEmbedding{}, errors.New("unexpected embedding")
}
