package runtimebinding

import (
	"context"
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type pluginBinding interface {
	GetPluginCommand(context.Context, protocol.PluginCommandRequest, protocol.CallOptions) (*protocol.CallPluginRequest, error)
	RetirePluginCommand(context.Context, protocol.PluginCommandRequest, protocol.CommandOptions) error
	ListPluginCompositions(context.Context, protocol.CallOptions) (*protocol.Page[protocol.PluginComposition], error)
	SetPluginComposition(context.Context, protocol.SetPluginCompositionRequest, protocol.CommandOptions) (*protocol.PluginComposition, error)
	ResolvePluginComposition(context.Context, protocol.ResolvePluginCompositionRequest, protocol.CallOptions) (*protocol.Page[protocol.PluginOperationBinding], error)
	ResolvePluginPoint(context.Context, protocol.ResolvePluginPointRequest, protocol.CallOptions) (*protocol.Page[protocol.PluginPointBinding], error)
	ResolvePluginRequirements(context.Context, protocol.ResolvePluginRequirementsRequest, protocol.CallOptions) (*protocol.Page[protocol.PluginCollaborator], error)
	CallPlugin(context.Context, protocol.CallPluginRequest, protocol.PluginCallOptions) (*protocol.CallPluginResponse, error)
	ListPluginProviders(context.Context, protocol.CallOptions) (*protocol.Page[protocol.PluginProviderSelection], error)
	SelectPluginProvider(context.Context, protocol.SelectPluginProviderRequest, protocol.CommandOptions) (*protocol.PluginProviderSelection, error)
	ListPlugins(context.Context, protocol.CallOptions) (*protocol.Page[protocol.PluginInstallation], error)
	InstallPlugin(context.Context, protocol.InstallPluginRequest, protocol.CommandOptions) (*protocol.PluginInstallation, error)
	StagePlugin(context.Context, protocol.StagePluginRequest, protocol.CommandOptions) (*protocol.PluginInstallation, error)
	SelectPlugin(context.Context, protocol.PluginReleaseRequest, protocol.CommandOptions) (*protocol.PluginInstallation, error)
	ConfigurePlugin(context.Context, protocol.ConfigurePluginRequest, protocol.CommandOptions) (*protocol.PluginInstallation, error)
	SetPluginEnablement(context.Context, protocol.SetPluginEnablementRequest, protocol.CommandOptions) (*protocol.PluginInstallation, error)
	UninstallPlugin(context.Context, protocol.PluginRequest, protocol.CommandOptions) error
}

// Retained input remains evidence after producer withdrawal; inspection cannot re-admit or route it.
func (r *Connection) GetPluginCommand(ctx context.Context, request protocol.PluginCommandRequest) (*protocol.CallPluginRequest, error) {
	value, err := r.plugins.GetPluginCommand(ctx, request, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if value == nil {
		return nil, runtimeContractViolation("get plugin command returned nil")
	}
	if err := protocol.ValidateWireTree(value); err != nil {
		return nil, runtimeContractViolation("get plugin command returned invalid request: %v", err)
	}
	return value, nil
}

func (r *Connection) RetirePluginCommand(ctx context.Context, request protocol.PluginCommandRequest, commandID replay.CommandID) error {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return err
	}
	return classifyError(r.plugins.RetirePluginCommand(ctx, request, options))
}

func (r *Connection) ListPluginCompositions(ctx context.Context) ([]protocol.PluginComposition, error) {
	page, err := r.plugins.ListPluginCompositions(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list plugin compositions", page)
	if err != nil {
		return nil, err
	}
	if err := requireUniqueIdentities("list plugin compositions", values, func(value protocol.PluginComposition) string { return value.Contract }); err != nil {
		return nil, err
	}
	return values, nil
}

func (r *Connection) SetPluginComposition(ctx context.Context, request protocol.SetPluginCompositionRequest, commandID replay.CommandID) (*protocol.PluginComposition, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	expected, err := capability.Parse([]byte(request.Contract))
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.SetPluginComposition(ctx, request, options)
	if err != nil {
		return nil, classifyError(err)
	}
	if value == nil || !slices.Equal(value.Members, request.Members) {
		return nil, runtimeContractViolation("set plugin composition returned another member order")
	}
	actual, err := capability.Parse([]byte(value.Contract))
	if err != nil || actual.Identity() != expected.Identity() {
		return nil, runtimeContractViolation("set plugin composition returned another contract")
	}
	return value, nil
}

func (r *Connection) ResolvePluginComposition(ctx context.Context, request protocol.ResolvePluginCompositionRequest) ([]protocol.PluginOperationBinding, error) {
	page, err := r.plugins.ResolvePluginComposition(ctx, request, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	return requireCompletePage("resolve plugin composition", page)
}

func (r *Connection) ResolvePluginPoint(ctx context.Context, request protocol.ResolvePluginPointRequest) ([]protocol.PluginPointBinding, error) {
	page, err := r.plugins.ResolvePluginPoint(ctx, request, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	return requireCompletePage("resolve plugin point", page)
}

func (r *Connection) ResolvePluginRequirements(ctx context.Context, request protocol.ResolvePluginRequirementsRequest) ([]protocol.PluginCollaborator, error) {
	page, err := r.plugins.ResolvePluginRequirements(ctx, request, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	return requireCompletePage("resolve plugin requirements", page)
}

func (r *Connection) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, commandID replay.CommandID) (*protocol.CallPluginResponse, error) {
	options := protocol.PluginCallOptions{RequestMeta: r.callOptions().RequestMeta}
	if commandID != "" {
		command, err := r.commandOptionsFor(commandID)
		if err != nil {
			return nil, err
		}
		options.IdempotencyKey = command.IdempotencyKey
		options.IdempotencyNamespace = command.IdempotencyNamespace
	}
	result, err := r.plugins.CallPlugin(ctx, request, options)
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, runtimeContractViolation("plugin call returned nil")
	}
	if err := protocol.ValidateWireTree(*result); err != nil {
		return nil, runtimeContractViolation("plugin call returned invalid response: %v", err)
	}
	if result.Failure != nil {
		return nil, &capability.Refusal{Code: result.Failure.Code, Message: result.Failure.Message}
	}
	contract, err := capability.Parse([]byte(request.Contract))
	if err != nil {
		return nil, err
	}
	method, found := contract.Method(request.Method)
	if !found {
		return nil, runtimeContractViolation("plugin call method is absent from the required contract")
	}
	if err := method.ValidateOutput([]byte(result.Output)); err != nil {
		return nil, runtimeContractViolation("plugin call returned invalid output: %v", err)
	}
	return result, nil
}

func (r *Connection) ListPluginProviders(ctx context.Context) ([]protocol.PluginProviderSelection, error) {
	page, err := r.plugins.ListPluginProviders(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list plugin providers", page)
	if err != nil {
		return nil, err
	}
	if err := requireUniqueIdentities("list plugin providers", values, func(value protocol.PluginProviderSelection) string { return value.Role }); err != nil {
		return nil, err
	}
	return values, nil
}

func (r *Connection) SelectPluginProvider(ctx context.Context, request protocol.SelectPluginProviderRequest, commandID replay.CommandID) (*protocol.PluginProviderSelection, error) {
	options, err := r.commandOptionsFor(commandID)
	if err != nil {
		return nil, err
	}
	value, err := r.plugins.SelectPluginProvider(ctx, request, options)
	if err != nil {
		return nil, classifyError(err)
	}
	if value == nil || value.Role != request.Role || value.InstallationID != request.InstallationID || value.Contribution != request.Contribution {
		return nil, runtimeContractViolation("select plugin provider returned another binding")
	}
	return value, nil
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
