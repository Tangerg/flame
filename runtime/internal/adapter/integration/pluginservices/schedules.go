package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/tools"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

type Schedules struct {
	contracts     rpc.Contracts
	installations plugins.Store
	pool          *mcpconnection.Pool
}

func (s *Schedules) OwnsCreateSource(ctx context.Context, ref tool.Ref, release fingerprint.Digest) (bool, error) {
	_, found, err := s.createSource(ctx, ref, release)
	return found, err
}

func (s *Schedules) createSource(ctx context.Context, ref tool.Ref, release fingerprint.Digest) (plugins.ContributionBinding, bool, error) {
	var zero plugins.ContributionBinding
	server, remote, found := ref.MCP()
	if !found || remote.String() != "create_schedule" {
		return zero, false, nil
	}
	installation, managed := server.Origin().Installation()
	if !managed {
		return zero, false, nil
	}
	snapshot, found, err := s.installations.Get(ctx, installation)
	if err != nil || !found {
		return zero, false, err
	}
	if snapshot.Selected.Digest() != release {
		return zero, false, nil
	}
	for _, contribution := range snapshot.Selected.Declaration().Contributions {
		if contribution.Contract != s.contracts.Capability.Identity() || !snapshot.Installation.ServerEnabled(contribution.Server) {
			continue
		}
		id, err := snapshot.Installation.ServerID(contribution.Server)
		if err != nil {
			return zero, false, err
		}
		if id == server {
			return plugins.ContributionBinding{InstallationID: installation, ContributionID: contribution.ID, Digest: release, ServerName: contribution.Server, ServerID: server, Export: contribution.Export, Contract: contribution.Contract}, true, nil
		}
	}
	return zero, false, nil
}

func (s *Schedules) OriginalCreateResult(ctx context.Context, ref tool.Ref, release, authority fingerprint.Digest, callID, arguments string) (*chat.ToolOutput, error) {
	original, err := s.originalCreateBinding(ctx, ref, release, authority)
	if err != nil {
		return nil, err
	}
	// The Tool receipt owns its admitted arguments, before Draft translation.
	admitted, err := jsoninput.Parse([]byte(arguments))
	if err != nil {
		return nil, err
	}
	binding, err := original.Method(rpc.Create)
	if err != nil {
		return nil, err
	}
	receipt, found, err := binding.ReadReceipt(ctx, "tool:"+callID, admitted)
	if err != nil || !found {
		return nil, rpc.DecodeError(err)
	}
	var snapshot schedule.Snapshot
	if err := json.Unmarshal(receipt.JSON(), &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	created, err := schedule.Restore(snapshot)
	if err != nil {
		return nil, err
	}
	output, err := tools.CreateOutput(created)
	if err != nil {
		return nil, err
	}
	return &output, nil
}

func (s *Schedules) AwaitOriginalCreateSource(ctx context.Context, ref tool.Ref, release, authority fingerprint.Digest) error {
	_, err := s.originalCreateBinding(ctx, ref, release, authority)
	return err
}

func (s *Schedules) originalCreateBinding(ctx context.Context, ref tool.Ref, release, authority fingerprint.Digest) (*pluginsdk.CapabilityBinding, error) {
	source, found, err := s.createSource(ctx, ref, release)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, plugin.ErrUnavailable
	}
	session, err := s.pool.BoundServiceSession(ctx, source.ServerID, authority)
	if err != nil {
		return nil, err
	}
	return pluginsdk.NewCapabilityBinding(session, source.Export, s.contracts.Capability)
}

func NewSchedules(store plugins.Store, pool *mcpconnection.Pool) (*Schedules, error) {
	if dependency.Missing(store) || pool == nil {
		return nil, errors.New("schedules: installation store and connections are required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	return &Schedules{contracts: contracts, installations: store, pool: pool}, nil
}
