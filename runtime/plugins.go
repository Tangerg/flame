package runtime

import (
	"context"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (r *binding) ListPlugins(ctx context.Context, options CallOptions) (*protocol.Page[protocol.PluginInstallation], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.PluginInstallation]](ctx, delivery.PluginsList, struct{}{}, callOptions(options))
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
