package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	sdk "github.com/Tangerg/go-sdk/mcp"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
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
	commands      *CommandCalls
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

func NewSchedules(store plugins.Store, pool *mcpconnection.Pool, commands *CommandCalls) (*Schedules, error) {
	if dependency.Missing(store) || pool == nil {
		return nil, errors.New("schedules: installation store and connections are required")
	}
	if commands == nil {
		return nil, errors.New("schedules: command routing is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	return &Schedules{contracts: contracts, installations: store, pool: pool, commands: commands}, nil
}
func (s *Schedules) Create(ctx context.Context, input schedule.Draft) (schedule.Schedule, error) {
	value, err := invokeCommand[schedule.Draft, schedule.Snapshot](ctx, s.commands, s.contracts.Capability, rpc.Create, input, func(value schedule.Snapshot) error { _, err := schedule.Restore(value); return err })
	if err != nil {
		return schedule.Schedule{}, rpc.DecodeError(err)
	}
	saved, err := schedule.Restore(value.value)
	return saved, err
}
func (s *Schedules) Update(ctx context.Context, input app.UpdateCommand) (schedule.Schedule, error) {
	value, err := invokeCommand[rpc.UpdateInput, schedule.Snapshot](ctx, s.commands, s.contracts.Capability, rpc.Update, rpc.UpdateInput{ID: input.ID, ExpectedRevision: input.ExpectedRevision, Title: input.Patch.Title, Instructions: input.Patch.Instructions, CWD: input.Patch.CWD, Cron: input.Patch.Cron, Enabled: input.Patch.Enabled, ModelSelection: rpc.EncodeModelPatch(input.Patch.ModelSelection)}, func(value schedule.Snapshot) error { _, err := schedule.Restore(value); return err })
	if err != nil {
		return schedule.Schedule{}, rpc.DecodeError(err)
	}
	saved, err := schedule.Restore(value.value)
	return saved, err
}
func (s *Schedules) RunNow(ctx context.Context, id string) (app.RunReference, error) {
	reply, err := invokeResolvedCommand[rpc.ItemInput, app.RunReference](ctx, s.commands, rpc.RunNow, rpc.ItemInput{ID: id}, func(value app.RunReference) error {
		return errors.Join(resourceid.ValidateSession(value.SessionID), resourceid.ValidateRun(value.RunID))
	}, func(ctx context.Context) (plugins.ContributionBinding, error) {
		binding, found, err := plugins.SelectedContribution(ctx, s.installations, s.commands.selections, s.contracts.Capability)
		if err == nil && !found {
			err = plugin.ErrUnavailable
		}
		return binding, err
	}, func(ctx context.Context, binding plugins.ContributionBinding, session *sdk.ClientSession) (sdk.ParamsBase, func(), error) {
		return s.pool.OpenRPC(ctx, binding.ServerID, session)
	})
	return reply.value, rpc.DecodeError(err)
}
