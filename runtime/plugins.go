package runtime

import (
	"context"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (r *binding) ListPlugins(ctx context.Context, options CallOptions) (*protocol.Page[protocol.PluginInstallation], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.PluginInstallation]](ctx, delivery.PluginsList, struct{}{}, callOptions(options))
}

func (r *binding) RenamePluginSession(ctx context.Context, request protocol.RenamePluginSessionRequest, options CommandOptions) (*protocol.Session, error) {
	return r.invoke[protocol.RenamePluginSessionRequest, *protocol.Session](ctx, delivery.PluginsRenameSession, request, commandOptions(options))
}

func (r *binding) ReadPluginView(ctx context.Context, request protocol.ReadPluginViewRequest, options CallOptions) (*protocol.PluginViewResource, error) {
	return r.invoke[protocol.ReadPluginViewRequest, *protocol.PluginViewResource](ctx, delivery.PluginsReadView, request, callOptions(options))
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
func (r *binding) ApprovePlugin(ctx context.Context, request protocol.PluginReleaseRequest, options CommandOptions) (*protocol.PluginInstallation, error) {
	return r.invoke[protocol.PluginReleaseRequest, *protocol.PluginInstallation](ctx, delivery.PluginsApprove, request, commandOptions(options))
}
func (r *binding) ConfigurePlugin(ctx context.Context, request protocol.ConfigurePluginRequest, options CommandOptions) (*protocol.PluginInstallation, error) {
	return r.invoke[protocol.ConfigurePluginRequest, *protocol.PluginInstallation](ctx, delivery.PluginsConfigure, request, commandOptions(options))
}
func (r *binding) SetPluginEnablement(ctx context.Context, request protocol.SetPluginEnablementRequest, options CommandOptions) (*protocol.PluginInstallation, error) {
	return r.invoke[protocol.SetPluginEnablementRequest, *protocol.PluginInstallation](ctx, delivery.PluginsSetEnablement, request, commandOptions(options))
}
func (r *binding) RevokePlugin(ctx context.Context, request protocol.PluginRequest, options CommandOptions) (*protocol.PluginInstallation, error) {
	return r.invoke[protocol.PluginRequest, *protocol.PluginInstallation](ctx, delivery.PluginsRevoke, request, commandOptions(options))
}
func (r *binding) UninstallPlugin(ctx context.Context, request protocol.PluginRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.PluginsUninstall, request, commandOptions(options))
}
