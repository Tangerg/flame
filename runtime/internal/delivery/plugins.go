package delivery

import (
	"context"
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	PluginsList          Name = "plugins.list"
	PluginsInstall       Name = "plugins.install"
	PluginsStage         Name = "plugins.stage"
	PluginsSelect        Name = "plugins.select"
	PluginsApprove       Name = "plugins.approve"
	PluginsConfigure     Name = "plugins.configure"
	PluginsSetEnablement Name = "plugins.setEnablement"
	PluginsRevoke        Name = "plugins.revoke"
	PluginsUninstall     Name = "plugins.uninstall"
)

// pluginErrors is every problem wirePluginError can raise. Each plugins method
// reaches the plugin owner through that one projection, so they share the set.
func pluginErrors() []string {
	return []string{
		protocol.ErrPluginUnavailable.Error(),
		protocol.ErrPluginNotFound.Error(),
		protocol.ErrPluginInUse.Error(),
		protocol.ErrPluginUnapproved.Error(),
		protocol.ErrPluginStale.Error(),
	}
}

func registerPlugins(r *Registry) {
	r.query(MethodMeta{Name: PluginsList, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ListPlugins(context.Context) (*protocol.Page[protocol.PluginInstallation], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.PluginInstallation], error) {
		return s.ListPlugins(ctx)
	})
	r.command(MethodMeta{Name: PluginsInstall, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		InstallPlugin(context.Context, protocol.InstallPluginRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.InstallPluginRequest) (*protocol.PluginInstallation, error) {
		return s.InstallPlugin(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsStage, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		StagePlugin(context.Context, protocol.StagePluginRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.StagePluginRequest) (*protocol.PluginInstallation, error) {
		return s.StagePlugin(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsSelect, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		SelectPlugin(context.Context, protocol.PluginReleaseRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.PluginReleaseRequest) (*protocol.PluginInstallation, error) {
		return s.SelectPlugin(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsApprove, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ApprovePlugin(context.Context, protocol.PluginReleaseRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.PluginReleaseRequest) (*protocol.PluginInstallation, error) {
		return s.ApprovePlugin(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsConfigure, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ConfigurePlugin(context.Context, protocol.ConfigurePluginRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.ConfigurePluginRequest) (*protocol.PluginInstallation, error) {
		return s.ConfigurePlugin(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsSetEnablement, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		SetPluginEnablement(context.Context, protocol.SetPluginEnablementRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.SetPluginEnablementRequest) (*protocol.PluginInstallation, error) {
		return s.SetPluginEnablement(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsRevoke, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		RevokePlugin(context.Context, protocol.PluginRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.PluginRequest) (*protocol.PluginInstallation, error) {
		return s.RevokePlugin(ctx, in)
	})
	r.commandAck(MethodMeta{Name: PluginsUninstall, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		UninstallPlugin(context.Context, protocol.PluginRequest) error
	}, ctx context.Context, in protocol.PluginRequest) error {
		return s.UninstallPlugin(ctx, in)
	})
}
