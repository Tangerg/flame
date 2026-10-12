package plugins

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/jsoninput"
)

// ServiceBinding projects installation authority and, after discovery, its
// captured method contract. Neither projection can advance installation state.
type ServiceBinding struct {
	InstallationID resourceid.InstallationID
	ServerID       mcpserver.ID
	Digest         fingerprint.Digest
	ServerName     mcpserver.ServerName
	Contract       jsoninput.Input
}

func ServiceOwner(ctx context.Context, store Store, selections SelectionStore, kind plugin.ServiceKind) (ServiceBinding, bool, error) {
	choice, found, err := selections.Get(ctx, string(kind))
	if err != nil {
		return ServiceBinding{}, false, err
	}
	if !found {
		return ServiceBinding{}, false, nil
	}
	record := choice.Snapshot()
	snapshot, found, err := store.Get(ctx, record.Installation)
	if err != nil {
		return ServiceBinding{}, false, err
	}
	if !found {
		return ServiceBinding{}, false, nil
	}
	service, declared := snapshot.Selected.Service(kind)
	if !declared || service.Server != record.Contribution || !snapshot.Installation.ServerEnabled(service.Server) {
		return ServiceBinding{}, false, nil
	}
	id, err := snapshot.Installation.ServerID(service.Server)
	if err != nil {
		return ServiceBinding{}, false, err
	}
	return ServiceBinding{InstallationID: record.Installation, ServerID: id, Digest: snapshot.Selected.Digest(), ServerName: service.Server}, true, nil
}
