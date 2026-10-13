package runtime

import (
	"context"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (r *binding) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, options PluginCallOptions) (*protocol.CallPluginResponse, error) {
	return r.invoke[protocol.CallPluginRequest, *protocol.CallPluginResponse](ctx, delivery.PluginsCall, request, pluginCallOptions(options))
}

func (r *binding) ListPlugins(ctx context.Context, options CallOptions) (*protocol.Page[protocol.PluginInstallation], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.PluginInstallation]](ctx, delivery.PluginsList, struct{}{}, callOptions(options))
}

func (r *binding) ListPluginProviders(ctx context.Context, options CallOptions) (*protocol.Page[protocol.PluginProviderSelection], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.PluginProviderSelection]](ctx, delivery.PluginsListProviders, struct{}{}, callOptions(options))
}

func (r *binding) SelectPluginProvider(ctx context.Context, request protocol.SelectPluginProviderRequest, options CommandOptions) (*protocol.PluginProviderSelection, error) {
	return r.invoke[protocol.SelectPluginProviderRequest, *protocol.PluginProviderSelection](ctx, delivery.PluginsSelectProvider, request, commandOptions(options))
}

func (r *binding) RenamePluginSession(ctx context.Context, request protocol.RenamePluginSessionRequest, options CommandOptions) (*protocol.Session, error) {
	return r.invoke[protocol.RenamePluginSessionRequest, *protocol.Session](ctx, delivery.PluginsRenameSession, request, commandOptions(options))
}

func (r *binding) ReadPluginView(ctx context.Context, request protocol.ReadPluginViewRequest, options CallOptions) (*protocol.PluginViewResource, error) {
	return r.invoke[protocol.ReadPluginViewRequest, *protocol.PluginViewResource](ctx, delivery.PluginsReadView, request, callOptions(options))
}

func (r *binding) ReadPluginRenderer(ctx context.Context, request protocol.ReadPluginRendererRequest, options CallOptions) (*protocol.PluginViewResource, error) {
	return r.invoke[protocol.ReadPluginRendererRequest, *protocol.PluginViewResource](ctx, delivery.PluginsReadRenderer, request, callOptions(options))
}

func (r *binding) ReadPluginTrajectory(ctx context.Context, request protocol.ReadPluginTrajectoryRequest, options CallOptions) (*protocol.Page[protocol.TrajectoryEntry], error) {
	return r.invoke[protocol.ReadPluginTrajectoryRequest, *protocol.Page[protocol.TrajectoryEntry]](ctx, delivery.PluginsReadTrajectory, request, callOptions(options))
}

func (r *binding) ReadPluginMemory(ctx context.Context, request protocol.ReadPluginMemoryRequest, options CallOptions) (*protocol.Page[protocol.AgentMemoryItem], error) {
	return r.invoke[protocol.ReadPluginMemoryRequest, *protocol.Page[protocol.AgentMemoryItem]](ctx, delivery.PluginsReadMemory, request, callOptions(options))
}
func (r *binding) InstallPlugin(ctx context.Context, request protocol.InstallPluginRequest, options CommandOptions) (*protocol.PluginInstallation, error) {
	return r.invoke[protocol.InstallPluginRequest, *protocol.PluginInstallation](ctx, delivery.PluginsInstall, request, commandOptions(options))
}
func (r *binding) StagePlugin(ctx context.Context, request protocol.StagePluginRequest, options CommandOptions) (*protocol.PluginInstallation, error) {
	return r.invoke[protocol.StagePluginRequest, *protocol.PluginInstallation](ctx, delivery.PluginsStage, request, commandOptions(options))
}
func (r *binding) SelectPlugin(ctx context.Context, request protocol.PluginReleaseRequest, options CommandOptions) (*protocol.PluginInstallation, error) {
	return r.invoke[protocol.PluginReleaseRequest, *protocol.PluginInstallation](ctx, delivery.PluginsSelect, request, commandOptions(options))
}
func (r *binding) ConfigurePlugin(ctx context.Context, request protocol.ConfigurePluginRequest, options CommandOptions) (*protocol.PluginInstallation, error) {
	return r.invoke[protocol.ConfigurePluginRequest, *protocol.PluginInstallation](ctx, delivery.PluginsConfigure, request, commandOptions(options))
}
func (r *binding) SetPluginEnablement(ctx context.Context, request protocol.SetPluginEnablementRequest, options CommandOptions) (*protocol.PluginInstallation, error) {
	return r.invoke[protocol.SetPluginEnablementRequest, *protocol.PluginInstallation](ctx, delivery.PluginsSetEnablement, request, commandOptions(options))
}
func (r *binding) UninstallPlugin(ctx context.Context, request protocol.PluginRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.PluginsUninstall, request, commandOptions(options))
}

func (r *binding) ReadPluginSchedules(ctx context.Context, request protocol.ReadPluginSchedulesRequest, options CallOptions) (*protocol.Page[protocol.Schedule], error) {
	return r.invoke[protocol.ReadPluginSchedulesRequest, *protocol.Page[protocol.Schedule]](ctx, delivery.PluginsReadSchedules, request, callOptions(options))
}

func (r *binding) ReadPluginUsage(ctx context.Context, request protocol.ReadPluginUsageRequest, options CallOptions) (*protocol.UsageSummary, error) {
	return r.invoke[protocol.ReadPluginUsageRequest, *protocol.UsageSummary](ctx, delivery.PluginsReadUsage, request, callOptions(options))
}

func (r *binding) ReadPluginSkills(ctx context.Context, request protocol.ReadPluginSkillsRequest, options CallOptions) (*protocol.SkillDiscovery, error) {
	return r.invoke[protocol.ReadPluginSkillsRequest, *protocol.SkillDiscovery](ctx, delivery.PluginsReadSkills, request, callOptions(options))
}

func (r *binding) ReadPluginSkill(ctx context.Context, request protocol.ReadPluginSkillRequest, options CallOptions) (*protocol.SkillDetail, error) {
	return r.invoke[protocol.ReadPluginSkillRequest, *protocol.SkillDetail](ctx, delivery.PluginsReadSkill, request, callOptions(options))
}

func (r *binding) ReadPluginSkillLibrary(ctx context.Context, request protocol.ReadPluginViewRequest, options CallOptions) (*protocol.Page[protocol.ManagedSkill], error) {
	return r.invoke[protocol.ReadPluginViewRequest, *protocol.Page[protocol.ManagedSkill]](ctx, delivery.PluginsReadSkillLibrary, request, callOptions(options))
}

func (r *binding) ReadPluginSkillProposals(ctx context.Context, request protocol.ReadPluginSkillsRequest, options CallOptions) (*protocol.Page[protocol.SkillProposal], error) {
	return r.invoke[protocol.ReadPluginSkillsRequest, *protocol.Page[protocol.SkillProposal]](ctx, delivery.PluginsReadSkillProposals, request, callOptions(options))
}

func (r *binding) ReadPluginDiff(ctx context.Context, request protocol.ReadPluginDiffRequest, options CallOptions) (*protocol.Diff, error) {
	return r.invoke[protocol.ReadPluginDiffRequest, *protocol.Diff](ctx, delivery.PluginsReadDiff, request, callOptions(options))
}
