package runtime

import (
	"context"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"iter"
)

func (r *binding) OpenPluginResources(ctx context.Context, request protocol.OpenPluginResourcesRequest, options SubscriptionOptions) (*protocol.PluginResourceLease, iter.Seq2[protocol.PluginResourceEvent, error], error) {
	return r.invokeStream[protocol.OpenPluginResourcesRequest, *protocol.PluginResourceLease, protocol.PluginResourceEvent](ctx, delivery.PluginsOpenResources, request, subscriptionOptions(options))
}

func (r *binding) ReadPluginResource(ctx context.Context, request protocol.ReadPluginResourceRequest, options CallOptions) (*protocol.PluginResource, error) {
	return r.invoke[protocol.ReadPluginResourceRequest, *protocol.PluginResource](ctx, delivery.PluginsReadResource, request, callOptions(options))
}

func (r *binding) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, options PluginCallOptions) (*protocol.CallPluginResponse, error) {
	return r.invoke[protocol.CallPluginRequest, *protocol.CallPluginResponse](ctx, delivery.PluginsCall, request, pluginCallOptions(options))
}

// StreamPlugin opens an installed command stream. The opening response conveys
// no business outcome; only a terminal event or the original receipt does.
func (r *binding) StreamPlugin(ctx context.Context, request protocol.CallPluginRequest, options CommandOptions) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
	return r.invokeStream[protocol.CallPluginRequest, struct{}, protocol.PluginStreamEvent](ctx, delivery.PluginsStream, request, commandOptions(options))
}

func (r *binding) ResolvePluginRequirements(ctx context.Context, request protocol.ResolvePluginRequirementsRequest, options CallOptions) (*protocol.Page[protocol.PluginCollaborator], error) {
	return r.invoke[protocol.ResolvePluginRequirementsRequest, *protocol.Page[protocol.PluginCollaborator]](ctx, delivery.PluginsResolveRequirements, request, callOptions(options))
}

func (r *binding) ResolvePluginPoint(ctx context.Context, request protocol.ResolvePluginPointRequest, options CallOptions) (*protocol.Page[protocol.PluginPointBinding], error) {
	return r.invoke[protocol.ResolvePluginPointRequest, *protocol.Page[protocol.PluginPointBinding]](ctx, delivery.PluginsResolvePoint, request, callOptions(options))
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

func (r *binding) ListPluginCompositions(ctx context.Context, options CallOptions) (*protocol.Page[protocol.PluginComposition], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.PluginComposition]](ctx, delivery.PluginsListCompositions, struct{}{}, callOptions(options))
}

func (r *binding) SetPluginComposition(ctx context.Context, request protocol.SetPluginCompositionRequest, options CommandOptions) (*protocol.PluginComposition, error) {
	return r.invoke[protocol.SetPluginCompositionRequest, *protocol.PluginComposition](ctx, delivery.PluginsSetComposition, request, commandOptions(options))
}

func (r *binding) ResolvePluginComposition(ctx context.Context, request protocol.ResolvePluginCompositionRequest, options CallOptions) (*protocol.Page[protocol.PluginOperationBinding], error) {
	return r.invoke[protocol.ResolvePluginCompositionRequest, *protocol.Page[protocol.PluginOperationBinding]](ctx, delivery.PluginsResolveComposition, request, callOptions(options))
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

// NotifyPluginChange invalidates reads after work owned by a managed entry.
func (r *binding) NotifyPluginChange(ctx context.Context, request protocol.PluginChangeNotice, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.PluginsNotifyChange, request, commandOptions(options))
}
