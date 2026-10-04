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
	ApprovePlugin(context.Context, protocol.ApprovePluginRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	ConfigurePlugin(context.Context, protocol.ConfigurePluginRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	SetPluginEnablement(context.Context, protocol.SetPluginEnablementRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	RevokePlugin(context.Context, protocol.PluginRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error)
	UninstallPlugin(context.Context, protocol.PluginRequest, flameruntime.CommandOptions) (*protocol.PluginRemoval, error)
}

func (r *Connection) ListPlugins(ctx context.Context) (*protocol.Page[protocol.PluginInstallation], error) {
	options := r.callOptions()
	value, err := r.plugins.ListPlugins(ctx, options)
	return value, classifyError(err)
}
func (r *Connection) InstallPlugin(ctx context.Context, request protocol.InstallPluginRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.InstallPlugin(ctx, request, options)
	return value, classifyError(err)
}
func (r *Connection) StagePlugin(ctx context.Context, request protocol.StagePluginRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.StagePlugin(ctx, request, options)
	return value, classifyError(err)
}
func (r *Connection) SelectPlugin(ctx context.Context, request protocol.PluginReleaseRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.SelectPlugin(ctx, request, options)
	return value, classifyError(err)
}
func (r *Connection) ApprovePlugin(ctx context.Context, request protocol.ApprovePluginRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.ApprovePlugin(ctx, request, options)
	return value, classifyError(err)
}
func (r *Connection) ConfigurePlugin(ctx context.Context, request protocol.ConfigurePluginRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.ConfigurePlugin(ctx, request, options)
	return value, classifyError(err)
}
func (r *Connection) SetPluginEnablement(ctx context.Context, request protocol.SetPluginEnablementRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.SetPluginEnablement(ctx, request, options)
	return value, classifyError(err)
}
func (r *Connection) RevokePlugin(ctx context.Context, request protocol.PluginRequest, commandID replay.CommandID) (*protocol.PluginInstallation, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.RevokePlugin(ctx, request, options)
	return value, classifyError(err)
}
func (r *Connection) UninstallPlugin(ctx context.Context, request protocol.PluginRequest, commandID replay.CommandID) (*protocol.PluginRemoval, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.UninstallPlugin(ctx, request, options)
	return value, classifyError(err)
}
