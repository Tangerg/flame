package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
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

// listModels uses the registered Scope binding for authentication, endpoint
// selection, pagination and bounded response admission. Flame owns the probe
// budget and the identities its product accepts.
func listModels(ctx context.Context, spec clientSpec, deployments azureDeployments) ([]string, error) {
	if Provider(spec.connection.Provider) == providerAzureOpenAI {
		return deployments.forEndpoint(spec.connection.BaseURL), nil
	}
	profile, _ := providers.lookup(Provider(spec.connection.Provider))
	if profile.profile.ModelSource != modelprovider.EndpointModels {
		return nil, errors.New("model: provider uses the published model catalog")
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	model, err := profile.chatBuilder(ctx, spec, chat.Options{})
	if err != nil {
		return nil, err
	}
	discovery, ok := model.(modelLister)
	if !ok {
		return nil, fmt.Errorf("model: provider %q has no model discovery capability", spec.connection.Provider)
	}
	return advertisedModels(ctx, discovery)
}

// verifyConnection proves the credentials and endpoint without generating
// where the binding reads a model listing, because a capped generation is
// refused by reasoning models whose minimum output exceeds the cap. A binding
// without a listing is verified by one ordinary default-model exchange.
func verifyConnection(ctx context.Context, spec clientSpec) error {
	profile, _ := providers.lookup(Provider(spec.connection.Provider))
	model, err := profile.chatBuilder(ctx, spec, chat.Options{Model: profile.profile.DefaultModel})
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
	ids, err := advertisedModels(ctx, discovery)
	if err != nil {
		return err
	}
	if profile.profile.ModelSource == modelprovider.EndpointModels && Provider(profile.profile.ID) != providerAzureOpenAI && len(ids) == 0 {
		return errors.New("model: provider advertised no models")
	}
	return nil
}

func advertisedModels(ctx context.Context, discovery modelLister) ([]string, error) {
	ids, err := discovery.ListModels(ctx, maximumRemoteModelCount, maximumModelProbeResponseBytes)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, err := modelchoice.NewModelIdentity(id); err != nil {
			return nil, fmt.Errorf("model: advertised model identity %q: %w", id, err)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}
