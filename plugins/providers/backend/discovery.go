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
	maximumModelProbeResponseBytes = 1 << 20
	maximumRemoteModelCount        = 4096
)

type modelLister interface {
	ListModels(context.Context, int, int64) ([]string, error)
}

// listModels uses the registered Scope binding for authentication, endpoint
// selection, pagination and bounded response admission. Flame owns the probe
// budget and the identities its product accepts.
func listModels(ctx context.Context, spec clientSpec) ([]string, error) {
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
