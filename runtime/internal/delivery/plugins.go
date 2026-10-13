package delivery

import (
	"context"
	"github.com/Tangerg/flame/runtime/protocol"
	"iter"
)

const (
	PluginsNotifyChange        Name = "plugins.notifyChange"
	PluginsList                Name = "plugins.list"
	PluginsCall                Name = "plugins.call"
	PluginsResolveRequirements Name = "plugins.resolveRequirements"
	PluginsResolvePoint        Name = "plugins.resolvePoint"
	PluginsOpenResources       Name = "plugins.openResources"
	PluginsReadResource        Name = "plugins.readResource"
	PluginsListProviders       Name = "plugins.listProviders"
	PluginsSelectProvider      Name = "plugins.selectProvider"
	PluginsInstall             Name = "plugins.install"
	PluginsStage               Name = "plugins.stage"
	PluginsSelect              Name = "plugins.select"
	PluginsConfigure           Name = "plugins.configure"
	PluginsSetEnablement       Name = "plugins.setEnablement"
	PluginsUninstall           Name = "plugins.uninstall"
	PluginsRenameSession       Name = "plugins.renameSession"
)

// pluginErrors is every problem wirePluginError can raise. Each plugins method
// reaches the plugin owner through that one projection, so they share the set.
func pluginErrors() []string {
	return []string{
		protocol.ErrPluginUnavailable.Error(),
		protocol.ErrPluginNotFound.Error(),
		protocol.ErrPluginInUse.Error(),
		protocol.ErrPluginDisabled.Error(),
		protocol.ErrPluginStale.Error(),
	}
}

func registerPlugins(r *Registry) {
	r.commandAck(MethodMeta{Name: PluginsNotifyChange, CapabilityRules: requires(protocol.FeaturePlugins)}, func(service interface {
		NotifyPluginChange(context.Context, protocol.PluginChangeNotice) error
	}, ctx context.Context, request protocol.PluginChangeNotice) error {
		return service.NotifyPluginChange(ctx, request)
	})

	r.subscription(MethodMeta{Name: PluginsOpenResources, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		OpenPluginResources(context.Context, protocol.OpenPluginResourcesRequest) (*protocol.PluginResourceLease, iter.Seq2[protocol.PluginResourceEvent, error], error)
	}, ctx context.Context, in protocol.OpenPluginResourcesRequest) (*protocol.PluginResourceLease, iter.Seq2[protocol.PluginResourceEvent, error], error) {
		return s.OpenPluginResources(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsReadResource, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ReadPluginResource(context.Context, protocol.ReadPluginResourceRequest) (*protocol.PluginResource, error)
	}, ctx context.Context, in protocol.ReadPluginResourceRequest) (*protocol.PluginResource, error) {
		return s.ReadPluginResource(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsResolvePoint, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ResolvePluginPoint(context.Context, protocol.ResolvePluginPointRequest) (*protocol.Page[protocol.PluginPointBinding], error)
	}, ctx context.Context, in protocol.ResolvePluginPointRequest) (*protocol.Page[protocol.PluginPointBinding], error) {
		return s.ResolvePluginPoint(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsResolveRequirements, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ResolvePluginRequirements(context.Context, protocol.ResolvePluginRequirementsRequest) (*protocol.Page[protocol.PluginCollaborator], error)
	}, ctx context.Context, in protocol.ResolvePluginRequirementsRequest) (*protocol.Page[protocol.PluginCollaborator], error) {
		return s.ResolvePluginRequirements(ctx, in)
	})
	r.extension(MethodMeta{Name: PluginsCall, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, pluginCallOperation, func(s interface {
		PreparePluginCall(context.Context, protocol.CallPluginRequest) (OperationKind, func(context.Context) (*protocol.CallPluginResponse, error), error)
	}, ctx context.Context, in protocol.CallPluginRequest) (OperationKind, func(context.Context) (*protocol.CallPluginResponse, error), error) {
		return s.PreparePluginCall(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsListProviders, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ListPluginProviders(context.Context) (*protocol.Page[protocol.PluginProviderSelection], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.PluginProviderSelection], error) {
		return s.ListPluginProviders(ctx)
	})
	r.command(MethodMeta{Name: PluginsSelectProvider, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		SelectPluginProvider(context.Context, protocol.SelectPluginProviderRequest) (*protocol.PluginProviderSelection, error)
	}, ctx context.Context, in protocol.SelectPluginProviderRequest) (*protocol.PluginProviderSelection, error) {
		return s.SelectPluginProvider(ctx, in)
	})

	r.command(MethodMeta{Name: PluginsRenameSession, CapabilityRules: requires(protocol.FeaturePlugins), Errors: append(pluginErrors(), protocol.ErrSessionNotFound.Error(), protocol.ErrRevisionConflict.Error(), protocol.ErrSessionBusy.Error())}, func(s interface {
		RenamePluginSession(context.Context, protocol.RenamePluginSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, in protocol.RenamePluginSessionRequest) (*protocol.Session, error) {
		return s.RenamePluginSession(ctx, in)
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
	r.commandAck(MethodMeta{Name: PluginsUninstall, CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		UninstallPlugin(context.Context, protocol.PluginRequest) error
	}, ctx context.Context, in protocol.PluginRequest) error {
		return s.UninstallPlugin(ctx, in)
	})
}
