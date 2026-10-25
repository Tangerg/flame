package llm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	modelchoice "github.com/Tangerg/flame/runtime/model"
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

// ListModels uses the registered Scope binding for authentication, endpoint
// selection, pagination and bounded response admission. Flame owns the probe
// budget and the identities its product accepts.
func ListModels(ctx context.Context, spec ClientSpec) ([]string, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	profile, _ := providers.lookup(spec.provider)
	if !profile.chatModels.discoveredAtEndpoint() {
		return nil, errors.New("llm: provider uses the published model catalog")
	}
	endpoint, err := profile.endpoint.resolve(spec.endpoint)
	if err != nil {
		return nil, fmt.Errorf("llm: provider %q: %w", spec.provider, err)
	}
	spec = spec.withEndpoint(endpoint)
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	model, err := profile.chatBuilder(ctx, spec, chat.Options{})
	if err != nil {
		return nil, classifyModelError(err)
	}
	discovery, ok := model.(modelLister)
	if !ok {
		return nil, fmt.Errorf("llm: provider %q has no model discovery capability", spec.provider)
	}
	ids, err := discovery.ListModels(ctx, maximumRemoteModelCount, maximumModelProbeResponseBytes)
	if err != nil {
		return nil, classifyModelError(err)
	}
	for _, id := range ids {
		if _, err := modelchoice.NewModelIdentity(id); err != nil {
			return nil, fmt.Errorf("llm: advertised model identity %q: %w", id, err)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}
