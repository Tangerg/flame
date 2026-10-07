package runtimebinding

import (
	"context"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type pluginBinding interface {
	ListPlugins(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.PluginInstallation], error)
	InstallPlugin(context.Context, protocol.InstallPluginRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	StagePlugin(context.Context, protocol.StagePluginRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	SelectPlugin(context.Context, protocol.PluginReleaseRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	ApprovePlugin(context.Context, protocol.PluginReleaseRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	ConfigurePlugin(context.Context, protocol.ConfigurePluginRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	SetPluginEnablement(context.Context, protocol.SetPluginEnablementRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	RevokePlugin(context.Context, protocol.PluginRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	UninstallPlugin(context.Context, protocol.PluginRequest, flameruntime.CommandOptions) error
}

func (r *Connection) ListPlugins(ctx context.Context) ([]protocol.PluginInstallation, error) {
	page, err := r.plugins.ListPlugins(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	installations, err := requireCompletePage("list plugins", page)
	if err != nil {
		return nil, err
	}
	if err := requireUniqueIdentities("list plugins", installations, func(installation protocol.PluginInstallation) string {
		return installation.ID
	}); err != nil {
		return nil, err
	}
	return installations, nil
}
func (r *Connection) InstallPlugin(ctx context.Context, request protocol.InstallPluginRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.InstallPlugin(ctx, request, options)
	return pluginResult("install plugin", "", value, err)
}
func (r *Connection) StagePlugin(ctx context.Context, request protocol.StagePluginRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.StagePlugin(ctx, request, options)
	return pluginResult("stage plugin", request.InstallationID, value, err)
}
func (r *Connection) SelectPlugin(ctx context.Context, request protocol.PluginReleaseRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.SelectPlugin(ctx, request, options)
	return pluginResult("select plugin", request.InstallationID, value, err)
}
func (r *Connection) ApprovePlugin(ctx context.Context, request protocol.PluginReleaseRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.ApprovePlugin(ctx, request, options)
	return pluginResult("approve plugin", request.InstallationID, value, err)
}
func (r *Connection) ConfigurePlugin(ctx context.Context, request protocol.ConfigurePluginRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.ConfigurePlugin(ctx, request, options)
	return pluginResult("configure plugin", request.InstallationID, value, err)
}
func (r *Connection) SetPluginEnablement(ctx context.Context, request protocol.SetPluginEnablementRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.SetPluginEnablement(ctx, request, options)
	return pluginResult("set plugin enablement", request.InstallationID, value, err)
}
func (r *Connection) RevokePlugin(ctx context.Context, request protocol.PluginRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.RevokePlugin(ctx, request, options)
	return pluginResult("revoke plugin", request.InstallationID, value, err)
}
func (r *Connection) UninstallPlugin(ctx context.Context, request protocol.PluginRequest, commandID replay.CommandID) error {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return err
	}
	return classifyError(r.plugins.UninstallPlugin(ctx, request, options))
}

// pluginResult binds a mutation's reported installation to the one the request
// named, so a command never prints another installation's state as its own.
func pluginResult(
	operation, expectedID string,
	result *protocol.PluginInstallation,
	err error,
) (*protocol.PluginInstallation, error) {
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, runtimeContractViolation("%s returned nil", operation)
	}
	if err := requireIdentity(operation, result.ID, expectedID); err != nil {
		return nil, err
	}
	return result, nil
}
