package delivery

import (
	"context"
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	PluginsList           Name = "plugins.list"
	PluginsInstall        Name = "plugins.install"
	PluginsStage          Name = "plugins.stage"
	PluginsSelect         Name = "plugins.select"
	PluginsApprove        Name = "plugins.approve"
	PluginsConfigure      Name = "plugins.configure"
	PluginsSetEnablement  Name = "plugins.setEnablement"
	PluginsRevoke         Name = "plugins.revoke"
	PluginsUninstall      Name = "plugins.uninstall"
	PluginsReadView       Name = "plugins.readView"
	PluginsReadTrajectory Name = "plugins.readTrajectory"
	PluginsReadMemory     Name = "plugins.readMemory"
	PluginsReadSchedules  Name = "plugins.readSchedules"
	PluginsReadUsage      Name = "plugins.readUsage"
	PluginsRenameSession  Name = "plugins.renameSession"
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
	r.query(MethodMeta{Name: PluginsReadUsage, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors(), Materializes: []Name{UsageSummary}}, func(s interface {
		ReadPluginUsage(context.Context, protocol.ReadPluginUsageRequest) (*protocol.UsageSummary, error)
	}, ctx context.Context, in protocol.ReadPluginUsageRequest) (*protocol.UsageSummary, error) {
		return s.ReadPluginUsage(ctx, in)
	})

	r.query(MethodMeta{Name: PluginsReadSchedules, CapabilityRules: requires(protocol.FeaturePlugins, protocol.FeatureSchedules), Errors: pluginErrors(), Materializes: []Name{SchedulesList}}, func(s interface {
		ReadPluginSchedules(context.Context, protocol.ReadPluginSchedulesRequest) (*protocol.Page[protocol.Schedule], error)
	}, ctx context.Context, in protocol.ReadPluginSchedulesRequest) (*protocol.Page[protocol.Schedule], error) {
		return s.ReadPluginSchedules(ctx, in)
	})

	r.query(MethodMeta{Name: PluginsReadMemory, CapabilityRules: requires(protocol.FeaturePlugins, protocol.FeatureAgentMemory), Errors: pluginErrors(), Materializes: []Name{AgentMemoryList}}, func(s interface {
		ReadPluginMemory(context.Context, protocol.ReadPluginMemoryRequest) (*protocol.Page[protocol.AgentMemoryItem], error)
	}, ctx context.Context, in protocol.ReadPluginMemoryRequest) (*protocol.Page[protocol.AgentMemoryItem], error) {
		return s.ReadPluginMemory(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsRenameSession, CapabilityRules: requires(protocol.FeaturePlugins), Errors: append(pluginErrors(), protocol.ErrSessionNotFound.Error(), protocol.ErrRevisionConflict.Error(), protocol.ErrSessionBusy.Error())}, func(s interface {
		RenamePluginSession(context.Context, protocol.RenamePluginSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, in protocol.RenamePluginSessionRequest) (*protocol.Session, error) {
		return s.RenamePluginSession(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsReadView, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ReadPluginView(context.Context, protocol.ReadPluginViewRequest) (*protocol.PluginViewResource, error)
	}, ctx context.Context, in protocol.ReadPluginViewRequest) (*protocol.PluginViewResource, error) {
		return s.ReadPluginView(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsReadTrajectory, CapabilityRules: requires(protocol.FeaturePlugins), Errors: append(pluginErrors(), protocol.ErrSessionNotFound.Error()), Materializes: []Name{SessionsTrajectory}}, func(s interface {
		ReadPluginTrajectory(context.Context, protocol.ReadPluginTrajectoryRequest) (*protocol.Page[protocol.TrajectoryEntry], error)
	}, ctx context.Context, in protocol.ReadPluginTrajectoryRequest) (*protocol.Page[protocol.TrajectoryEntry], error) {
		return s.ReadPluginTrajectory(ctx, in)
	})
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
