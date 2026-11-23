package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func (p *Pool) RestoreTools(ctx context.Context, refs []domaintool.Ref, activations []plugin.Activation, prepare ToolPreparation) (_ []toolcontract.Tool, _ func() error, err error) {
	if prepare == nil {
		return nil, nil, errors.New("mcp: restored tools require call preparation")
	}
	sources, requested, err := restoredSources(refs)
	if err != nil {
		return nil, nil, err
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
		use, descriptor, err := p.borrowOriginal(ctx, source, activations)
		if err != nil {
			return nil, nil, err
		}
		borrowed = append(borrowed, use)
		if err := p.restoreSourceTools(use, source, descriptor, requested, prepare, tools); err != nil {
			return nil, nil, err
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

func restoredSources(refs []domaintool.Ref) ([]mcpserver.ID, map[domaintool.Ref]struct{}, error) {
	var sources []mcpserver.ID
	requested := make(map[domaintool.Ref]struct{}, len(refs))
	for _, ref := range refs {
		source, _, valid := ref.MCP()
		if _, managed := source.Origin().Installation(); !valid || !managed {
			return nil, nil, plugin.ErrInvalid
		}
		if _, duplicate := requested[ref]; duplicate {
			return nil, nil, plugin.ErrInvalid
		}
		requested[ref] = struct{}{}
		if !slices.Contains(sources, source) {
			sources = append(sources, source)
		}
	}
	return sources, requested, nil
}

func (p *Pool) borrowOriginal(ctx context.Context, source mcpserver.ID, activations []plugin.Activation) (*BorrowedSession, mcpserver.Server, error) {
	descriptor, found, err := p.registry.Dispatchable(ctx, source)
	if err != nil || !found || !descriptor.Enabled {
		return nil, mcpserver.Server{}, errors.Join(ErrUnavailable, err)
	}
	installation, _ := source.Origin().Installation()
	release, _, _ := descriptor.Source.Release()
	activation, err := plugins.OriginalActivation(activations, installation, release, source.Name())
	if err != nil {
		return nil, mcpserver.Server{}, err
	}
	use, err := p.BorrowActivation(ctx, source, activation)
	return use, descriptor, err
}

func (p *Pool) restoreSourceTools(use *BorrowedSession, source mcpserver.ID, descriptor mcpserver.Server, requested map[domaintool.Ref]struct{}, prepare ToolPreparation, tools map[domaintool.Ref]toolcontract.Tool) error {
	if _, err := p.SessionActor(use.Context(), source, use.Session()); err != nil {
		return err
	}
	config := ServerConfig{Server: descriptor}
	fingerprint, err := use.SourceFingerprint()
	if err != nil {
		return err
	}
	if fingerprint != config.AuthorityFingerprint() {
		return plugin.ErrStale
	}
	candidates, err := sourceTools(use.Context(), nil, config, use.Session())
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		ref, found, err := IdentifyTool(candidate)
		if err != nil || !found {
			return errors.Join(ErrUnavailable, err)
		}
		if _, wanted := requested[ref]; !wanted {
			continue
		}
		candidate.held, candidate.owner = use, p.inner
		tools[ref] = authorizedTool{source: candidate, registry: p.registry, pool: p, prepare: prepare}
	}
	return nil
}
