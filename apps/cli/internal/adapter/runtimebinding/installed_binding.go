package runtimebinding

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/portable"
)

// The accepted provider and complete callable contract are captured once for all
// pages. A later selection cannot splice another installation into this read.
func (r *Connection) selectedInstalledCall(ctx context.Context, requiredJSON, name string) (protocol.CallPluginRequest, error) {
	if r.plugins == nil {
		return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
	}
	required, err := portable.ParseCapability([]byte(requiredJSON))
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	if _, found := required.Method(name); !found {
		return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
	}
	providers, err := r.ListPluginProviders(ctx)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	var selected protocol.PluginProviderSelection
	found := false
	for _, provider := range providers {
		if provider.Role == required.Descriptor().Reference {
			selected = provider
			found = true
			break
		}
	}
	if !found {
		return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
	}
	installations, err := r.ListPlugins(ctx)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	for _, installation := range installations {
		if installation.ID != selected.InstallationID {
			continue
		}
		if !installation.Enabled {
			return protocol.CallPluginRequest{}, protocol.ErrPluginDisabled
		}
		for _, contribution := range installation.Selected.Contributions {
			if contribution.ID != selected.Contribution {
				continue
			}
			actual, err := portable.ParseCapability([]byte(contribution.Contract))
			if err != nil {
				return protocol.CallPluginRequest{}, errors.Join(protocol.ErrPluginUnavailable, err)
			}
			if actual.Identity() != required.Identity() {
				return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
			}
			return protocol.CallPluginRequest{InstallationID: installation.ID, Digest: installation.Selected.Digest, Contribution: contribution.ID, Contract: requiredJSON, Method: name, Target: portable.Target{Scope: required.Descriptor().Scope, Reference: installation.ID}}, nil
		}
	}
	return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
}
