package plugins

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// ServiceBinding is a read projection of the installation owner. It is never
// retained as enablement, and a connection must still admit the exact source.
type ServiceBinding struct {
	InstallationID resourceid.InstallationID
	ServerID       mcpserver.ID
	Digest         fingerprint.Digest
	ServerName     mcpserver.ServerName
}

func ServiceOwner(ctx context.Context, store Store, kind plugin.ServiceKind) (ServiceBinding, bool, error) {
	snapshots, err := store.List(ctx)
	if err != nil {
		return ServiceBinding{}, false, err
	}
	var owner ServiceBinding
	found := false
	for _, snapshot := range snapshots {
		installation := snapshot.Installation
		service, declared := snapshot.Selected.Service(kind)
		if !declared || !installation.Active() || !installation.ServerEnabled(service.Server) {
			continue
		}
		if found {
			return ServiceBinding{}, false, fmt.Errorf("%w: more than one enabled %s owner", plugin.ErrInvalid, kind)
		}
		id, err := installation.ServerID(service.Server)
		if err != nil {
			return ServiceBinding{}, false, err
		}
		owner = ServiceBinding{InstallationID: installation.ID(), ServerID: id, Digest: snapshot.Selected.Digest(), ServerName: service.Server}
		found = true
	}
	return owner, found, nil
}
func admitServices(ctx context.Context, store Store, candidate *plugin.Installation, release plugin.Release) error {
	if !candidate.Active() {
		return nil
	}
	snapshots, err := store.List(ctx)
	if err != nil {
		return err
	}
	for _, service := range release.Declaration().Services {
		if !candidate.ServerEnabled(service.Server) {
			continue
		}
		for _, snapshot := range snapshots {
			other := snapshot.Installation
			declared, found := snapshot.Selected.Service(service.Kind)
			if other.ID() != candidate.ID() && other.Active() && found && other.ServerEnabled(declared.Server) {
				return fmt.Errorf("%w: %s already has enabled installation %s", plugin.ErrInvalid, service.Kind, other.ID())
			}
		}
	}
	return nil
}
