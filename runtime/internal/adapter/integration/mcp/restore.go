package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// RestoreTools acquires the original checkpoint graphs before discovering their
// tools. The returned release belongs to the restored Run manifest; none of
// these sessions are published as today's catalog for new executions.
func (p *Pool) RestoreTools(ctx context.Context, refs []domaintool.Ref, activations []plugin.Activation) (_ []toolcontract.Tool, _ func() error, err error) {
	groups := make(map[mcpserver.ID]struct{})
	var sources []mcpserver.ID
	seen := make(map[domaintool.Ref]struct{}, len(refs))
	for _, ref := range refs {
		source, _, valid := ref.MCP()
		if _, managed := source.Origin().Installation(); !valid || !managed {
			return nil, nil, plugin.ErrInvalid
		}
		if _, duplicate := seen[ref]; duplicate {
			return nil, nil, plugin.ErrInvalid
		}
		seen[ref] = struct{}{}
		if _, found := groups[source]; !found {
			sources = append(sources, source)
		}
		groups[source] = struct{}{}
	}
	var borrowed []*BorrowedSession
	var once sync.Once
	var closeErr error
	release := func() error {
		once.Do(func() {
			for index := len(borrowed) - 1; index >= 0; index-- {
				closeErr = errors.Join(closeErr, borrowed[index].Close())
			}
		})
		return closeErr
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, release())
		}
	}()
	tools := make(map[domaintool.Ref]toolcontract.Tool, len(refs))
	for _, source := range sources {
		descriptor, found, err := p.registry.Dispatchable(ctx, source)
		if err != nil || !found || !descriptor.Enabled {
			return nil, nil, errors.Join(ErrUnavailable, err)
		}
		installation, _ := source.Origin().Installation()
		release, _, _ := descriptor.Source.Release()
		activation, err := plugins.OriginalActivation(activations, installation, release, source.Name())
		if err != nil {
			return nil, nil, err
		}
		use, err := p.BorrowActivation(ctx, source, activation)
		if err != nil {
			return nil, nil, err
		}
		borrowed = append(borrowed, use)
		_, err = p.SessionActor(use.Context(), source, use.Session())
		if err != nil {
			return nil, nil, err
		}
		config := ServerConfig{Server: descriptor}
		fingerprint, err := use.SourceFingerprint()
		if err != nil {
			return nil, nil, err
		}
		if fingerprint != config.AuthorityFingerprint() {
			return nil, nil, plugin.ErrStale
		}
		candidates, err := sourceTools(use.Context(), nil, config, use.Session())
		if err != nil {
			return nil, nil, err
		}
		for _, candidate := range candidates {
			ref, found, err := IdentifyTool(candidate)
			if err != nil || !found {
				return nil, nil, errors.Join(ErrUnavailable, err)
			}
			if _, requested := seen[ref]; !requested {
				continue
			}
			candidate.held, candidate.owner = use, p.inner
			tools[ref] = authorizedTool{Tool: candidate, pool: p, registry: p.registry, source: candidate,
				config: config, prepare: p.prepare, host: p.toolHost(candidate)}
		}
	}
	result := make([]toolcontract.Tool, 0, len(refs))
	for _, ref := range refs {
		value, found := tools[ref]
		if !found {
			return nil, nil, fmt.Errorf("%w: original tool %q is absent", plugin.ErrStale, ref)
		}
		result = append(result, value)
	}
	return result, release, nil
}
