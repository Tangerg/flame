package delivery

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	PluginsNotifyChange        Name = "plugins.notifyChange"
	PluginsList                Name = "plugins.list"
	PluginsCall                Name = "plugins.call"
	PluginsGetCommand          Name = "plugins.getCommand"
	PluginsRetireCommand       Name = "plugins.retireCommand"
	PluginsStream              Name = "plugins.stream"
	PluginsResolveRequirements Name = "plugins.resolveRequirements"
	PluginsResolvePoint        Name = "plugins.resolvePoint"
	PluginsOpenResources       Name = "plugins.openResources"
	PluginsReadResource        Name = "plugins.readResource"
	PluginsListProviders       Name = "plugins.listProviders"
	PluginsSelectProvider      Name = "plugins.selectProvider"
	PluginsListCompositions    Name = "plugins.listCompositions"
	PluginsSetComposition      Name = "plugins.setComposition"
	PluginsResolveComposition  Name = "plugins.resolveComposition"
	PluginsInstall             Name = "plugins.install"
	PluginsStage               Name = "plugins.stage"
	PluginsSelect              Name = "plugins.select"
	PluginsConfigure           Name = "plugins.configure"
	PluginsSetEnablement       Name = "plugins.setEnablement"
	PluginsUninstall           Name = "plugins.uninstall"
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
		protocol.ErrPluginCommandRetired.Error(),
	}
}

func registerPlugins(r *Registry) {
	r.query(MethodMeta{Name: PluginsGetCommand, GoName: "GetPluginCommand", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		GetPluginCommand(context.Context, protocol.PluginCommandRequest) (*protocol.CallPluginRequest, error)
	}, ctx context.Context, in protocol.PluginCommandRequest) (*protocol.CallPluginRequest, error) {
		return s.GetPluginCommand(ctx, in)
	})
	r.commandAck(MethodMeta{Name: PluginsRetireCommand, GoName: "RetirePluginCommand", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors(), Idempotency: IdempotencyOwnerResponse}, func(s interface {
		RetirePluginCommand(context.Context, protocol.PluginCommandRequest) error
	}, ctx context.Context, in protocol.PluginCommandRequest) error {
		return s.RetirePluginCommand(ctx, in)
	})
	r.streamExtension(MethodMeta{Name: PluginsStream, GoName: "StreamPlugin", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, pluginCallOperation, func(s interface {
		PreparePluginStream(context.Context, protocol.CallPluginRequest) (OperationKind, func(context.Context) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error), error)
	}, ctx context.Context, in protocol.CallPluginRequest) (OperationKind, func(context.Context) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error), error) {
		return s.PreparePluginStream(ctx, in)
	})
	r.commandAck(MethodMeta{Name: PluginsNotifyChange, GoName: "NotifyPluginChange", CapabilityRules: requires(protocol.FeaturePlugins)}, func(service interface {
		NotifyPluginChange(context.Context, protocol.PluginChangeNotice) error
	}, ctx context.Context, request protocol.PluginChangeNotice) error {
		return service.NotifyPluginChange(ctx, request)
	})

	r.subscription(MethodMeta{Name: PluginsOpenResources, GoName: "OpenPluginResources", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		OpenPluginResources(context.Context, protocol.OpenPluginResourcesRequest) (*protocol.PluginResourceLease, iter.Seq2[protocol.PluginResourceEvent, error], error)
	}, ctx context.Context, in protocol.OpenPluginResourcesRequest) (*protocol.PluginResourceLease, iter.Seq2[protocol.PluginResourceEvent, error], error) {
		return s.OpenPluginResources(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsReadResource, GoName: "ReadPluginResource", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ReadPluginResource(context.Context, protocol.ReadPluginResourceRequest) (*protocol.PluginResource, error)
	}, ctx context.Context, in protocol.ReadPluginResourceRequest) (*protocol.PluginResource, error) {
		return s.ReadPluginResource(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsResolvePoint, GoName: "ResolvePluginPoint", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ResolvePluginPoint(context.Context, protocol.ResolvePluginPointRequest) (*protocol.Page[protocol.PluginPointBinding], error)
	}, ctx context.Context, in protocol.ResolvePluginPointRequest) (*protocol.Page[protocol.PluginPointBinding], error) {
		return s.ResolvePluginPoint(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsResolveRequirements, GoName: "ResolvePluginRequirements", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ResolvePluginRequirements(context.Context, protocol.ResolvePluginRequirementsRequest) (*protocol.Page[protocol.PluginCollaborator], error)
	}, ctx context.Context, in protocol.ResolvePluginRequirementsRequest) (*protocol.Page[protocol.PluginCollaborator], error) {
		return s.ResolvePluginRequirements(ctx, in)
	})
	r.extension(MethodMeta{Name: PluginsCall, GoName: "CallPlugin", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, pluginCallOperation, func(s interface {
		PreparePluginCall(context.Context, protocol.CallPluginRequest) (OperationKind, func(context.Context) (*protocol.CallPluginResponse, error), error)
	}, ctx context.Context, in protocol.CallPluginRequest) (OperationKind, func(context.Context) (*protocol.CallPluginResponse, error), error) {
		return s.PreparePluginCall(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsListProviders, GoName: "ListPluginProviders", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ListPluginProviders(context.Context) (*protocol.Page[protocol.PluginProviderSelection], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.PluginProviderSelection], error) {
		return s.ListPluginProviders(ctx)
	})
	r.command(MethodMeta{Name: PluginsSelectProvider, GoName: "SelectPluginProvider", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		SelectPluginProvider(context.Context, protocol.SelectPluginProviderRequest) (*protocol.PluginProviderSelection, error)
	}, ctx context.Context, in protocol.SelectPluginProviderRequest) (*protocol.PluginProviderSelection, error) {
		return s.SelectPluginProvider(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsListCompositions, GoName: "ListPluginCompositions", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ListPluginCompositions(context.Context) (*protocol.Page[protocol.PluginComposition], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.PluginComposition], error) {
		return s.ListPluginCompositions(ctx)
	})
	r.command(MethodMeta{Name: PluginsSetComposition, GoName: "SetPluginComposition", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		SetPluginComposition(context.Context, protocol.SetPluginCompositionRequest) (*protocol.PluginComposition, error)
	}, ctx context.Context, in protocol.SetPluginCompositionRequest) (*protocol.PluginComposition, error) {
		return s.SetPluginComposition(ctx, in)
	})
	r.query(MethodMeta{Name: PluginsResolveComposition, GoName: "ResolvePluginComposition", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ResolvePluginComposition(context.Context, protocol.ResolvePluginCompositionRequest) (*protocol.Page[protocol.PluginOperationBinding], error)
	}, ctx context.Context, in protocol.ResolvePluginCompositionRequest) (*protocol.Page[protocol.PluginOperationBinding], error) {
		return s.ResolvePluginComposition(ctx, in)
	})

	r.query(MethodMeta{Name: PluginsList, GoName: "ListPlugins", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ListPlugins(context.Context) (*protocol.Page[protocol.PluginInstallation], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.PluginInstallation], error) {
		return s.ListPlugins(ctx)
	})
	r.command(MethodMeta{Name: PluginsInstall, GoName: "InstallPlugin", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		InstallPlugin(context.Context, protocol.InstallPluginRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.InstallPluginRequest) (*protocol.PluginInstallation, error) {
		return s.InstallPlugin(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsStage, GoName: "StagePlugin", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		StagePlugin(context.Context, protocol.StagePluginRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.StagePluginRequest) (*protocol.PluginInstallation, error) {
		return s.StagePlugin(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsSelect, GoName: "SelectPlugin", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		SelectPlugin(context.Context, protocol.PluginReleaseRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.PluginReleaseRequest) (*protocol.PluginInstallation, error) {
		return s.SelectPlugin(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsConfigure, GoName: "ConfigurePlugin", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		ConfigurePlugin(context.Context, protocol.ConfigurePluginRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.ConfigurePluginRequest) (*protocol.PluginInstallation, error) {
		return s.ConfigurePlugin(ctx, in)
	})
	r.command(MethodMeta{Name: PluginsSetEnablement, GoName: "SetPluginEnablement", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		SetPluginEnablement(context.Context, protocol.SetPluginEnablementRequest) (*protocol.PluginInstallation, error)
	}, ctx context.Context, in protocol.SetPluginEnablementRequest) (*protocol.PluginInstallation, error) {
		return s.SetPluginEnablement(ctx, in)
	})
	r.commandAck(MethodMeta{Name: PluginsUninstall, GoName: "UninstallPlugin", CapabilityRules: requires(protocol.FeaturePlugins), Errors: pluginErrors()}, func(s interface {
		UninstallPlugin(context.Context, protocol.PluginRequest) error
	}, ctx context.Context, in protocol.PluginRequest) error {
		return s.UninstallPlugin(ctx, in)
	})
}
