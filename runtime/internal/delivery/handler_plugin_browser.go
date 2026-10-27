package delivery

import (
	"context"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/control"
	"github.com/Tangerg/flame/runtime/internal/application/integration/pluginbrowser"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (s *Handler) OpenPluginBrowserRealm(ctx context.Context, in protocol.OpenPluginBrowserRealmRequest) (*protocol.PluginBrowserSession, error) {
	if s.pluginBrowser == nil {
		return nil, protocol.ErrPluginUnavailable
	}
	var resume *control.Realm
	if in.Resume != nil {
		resume = new(browserRealm(*in.Resume))
	}
	value, err := s.pluginBrowser.Open(ctx, pluginbrowser.OpenRequest{OperationID: in.OperationID, ResumeToken: in.ResumeToken, Resume: resume})
	if err != nil {
		return nil, wirePluginError(err)
	}
	return &protocol.PluginBrowserSession{
		Authority: wireBrowserAuthority(value.Authority), Realm: wireBrowserRealm(value.Realm), Credential: value.Credential,
		ControlEndpoint: value.ControlEndpoint, MaximumMessageBytes: value.MaximumMessageBytes,
		ProofCapacity: value.ProofCapacity, PollIntervalMilliseconds: int(value.PollInterval / time.Millisecond),
		ComponentContract: wireBrowserContract(value.ComponentContract),
	}, nil
}

func (s *Handler) AcquirePluginComponent(ctx context.Context, in protocol.AcquirePluginComponentRequest) (*protocol.PluginBrowserUseGrant, error) {
	if s.pluginBrowser == nil {
		return nil, protocol.ErrPluginUnavailable
	}
	installation, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, err
	}
	component, err := plugin.ParseContributionID(in.ComponentID)
	if err != nil {
		return nil, protocol.InvalidParameters(err)
	}
	value, err := s.pluginBrowser.Acquire(ctx, pluginbrowser.ComponentRequest{OperationID: in.OperationID, ParentUseID: in.ParentUseID, Realm: browserRealm(in.Realm), Installation: installation, Digest: digest, Component: component})
	if err != nil {
		return nil, wirePluginError(err)
	}
	return new(wireBrowserUse(value)), nil
}

func (s *Handler) ClosePluginBrowserRealm(ctx context.Context, in protocol.ClosePluginBrowserRealmRequest) error {
	if s.pluginBrowser == nil {
		return protocol.ErrPluginUnavailable
	}
	return wirePluginError(s.pluginBrowser.Close(ctx, browserRealm(in.Realm)))
}

func browserRealm(value protocol.PluginBrowserRealm) control.Realm {
	return control.Realm{ID: value.ID, Epoch: value.Epoch}
}

func wireBrowserRealm(value control.Realm) protocol.PluginBrowserRealm {
	return protocol.PluginBrowserRealm{ID: value.ID, Epoch: value.Epoch}
}

func wireBrowserAuthority(value control.Authority) protocol.PluginBrowserAuthority {
	return protocol.PluginBrowserAuthority{ID: value.ID, Epoch: value.Epoch}
}

func wireBrowserContract(value qimu.ContractRef) protocol.PluginBrowserContractRef {
	return protocol.PluginBrowserContractRef{
		Name: value.Name, Version: value.Version,
		Digest: protocol.PluginBrowserDigest(value.Digest),
		Kind:   protocol.PluginBrowserContractKind(value.Kind),
		Mode:   protocol.PluginBrowserContractMode(value.Mode),
	}
}

func wireBrowserUse(value control.UseGrant) protocol.PluginBrowserUseGrant {
	objects := make([]protocol.PluginBrowserObjectRef, 0, len(value.Objects))
	for _, object := range value.Objects {
		objects = append(objects, protocol.PluginBrowserObjectRef{
			ObjectID: object.ObjectID,
			Export: protocol.PluginBrowserLocalExport{
				InstanceID: object.Export.InstanceID, Name: object.Export.Name,
				Contract: wireBrowserContract(object.Export.Contract),
			},
		})
	}
	return protocol.PluginBrowserUseGrant{
		ID: value.ID, ApprovalToken: value.ApprovalToken,
		Authority: wireBrowserAuthority(value.Authority), Realm: wireBrowserRealm(value.Realm),
		Execution: protocol.PluginBrowserExecution{ID: value.Execution.ID, Generation: value.Execution.Generation},
		ScopeID:   value.ScopeID, ScopeGeneration: value.ScopeGeneration, Objects: objects,
	}
}
