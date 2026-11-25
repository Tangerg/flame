package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/scope/core/chat"
)

const (
	probeTimeout                   = 4 * time.Second
	verificationExchangeTimeout    = 30 * time.Second
	maximumModelProbeResponseBytes = 1 << 20
	maximumRemoteModelCount        = 4096
)

type modelLister interface {
	ListModels(context.Context, int, int64) ([]string, error)
}

func (s clientSpec) listModels(ctx context.Context, deployments azureDeployments) ([]string, error) {
	if s.profile.metadata.ID == string(providerAzureOpenAI) {
		return deployments.forEndpoint(s.baseURL)
	}
	if s.profile.metadata.ModelSource != modelprovider.EndpointModels {
		return nil, errors.New("model: provider uses the published model catalog")
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	model, err := s.profile.chatBuilder(ctx, s, chat.Options{})
	if err != nil {
		return nil, err
	}
	discovery, ok := model.(modelLister)
	if !ok {
		return nil, fmt.Errorf("model: provider %q has no model discovery capability", s.profile.metadata.ID)
	}
	return advertisedModels(ctx, discovery)
}

// A capped generation is refused by reasoning models whose minimum output exceeds
// the cap, so a model listing proves the connection where the binding has one.
func (s clientSpec) verify(ctx context.Context) error {
	model, err := s.profile.chatBuilder(ctx, s, chat.Options{Model: s.profile.metadata.DefaultModel})
	if err != nil {
		return err
	}
	discovery, ok := model.(modelLister)
	if !ok {
		ctx, cancel := context.WithTimeout(ctx, verificationExchangeTimeout)
		defer cancel()
		_, err := model.Call(ctx, &chat.Request{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("ping"))}})
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	_, err = advertisedModels(ctx, discovery)
	return err
}

func advertisedModels(ctx context.Context, discovery modelLister) ([]string, error) {
	ids, err := discovery.ListModels(ctx, maximumRemoteModelCount, maximumModelProbeResponseBytes)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, err := flamemodel.NewModelIdentity(id); err != nil {
			return nil, fmt.Errorf("model: advertised model identity %q: %w", id, err)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}
