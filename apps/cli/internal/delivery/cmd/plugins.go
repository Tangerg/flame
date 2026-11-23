package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/frame"
	"github.com/spf13/cobra"
)

type pluginRuntime interface {
	GetPluginCommand(context.Context, protocol.PluginCommandRequest) (*protocol.CallPluginRequest, error)
	RetirePluginCommand(context.Context, protocol.PluginCommandRequest, authoring.CommandID) error
	ListPluginCompositions(context.Context) ([]protocol.PluginComposition, error)
	SetPluginComposition(context.Context, protocol.SetPluginCompositionRequest, authoring.CommandID) (*protocol.PluginComposition, error)
	ResolvePluginComposition(context.Context, protocol.ResolvePluginCompositionRequest) ([]protocol.PluginOperationBinding, error)
	ResolvePluginPoint(context.Context, protocol.ResolvePluginPointRequest) ([]protocol.PluginPointBinding, error)
	ResolvePluginRequirements(context.Context, protocol.ResolvePluginRequirementsRequest) ([]protocol.PluginCollaborator, error)
	CallPlugin(context.Context, protocol.CallPluginRequest, authoring.CommandID) (*protocol.CallPluginResponse, error)
	ListPluginProviders(context.Context) ([]protocol.PluginProviderSelection, error)
	SelectPluginProvider(context.Context, protocol.SelectPluginProviderRequest, authoring.CommandID) (*protocol.PluginProviderSelection, error)
	ListPlugins(context.Context) ([]protocol.PluginInstallation, error)
	InstallPlugin(context.Context, protocol.InstallPluginRequest, authoring.CommandID) (*protocol.PluginInstallation, error)
	StagePlugin(context.Context, protocol.StagePluginRequest, authoring.CommandID) (*protocol.PluginInstallation, error)
	SelectPlugin(context.Context, protocol.PluginReleaseRequest, authoring.CommandID) (*protocol.PluginInstallation, error)
	ConfigurePlugin(context.Context, protocol.ConfigurePluginRequest, authoring.CommandID) (*protocol.PluginInstallation, error)
	SetPluginEnablement(context.Context, protocol.SetPluginEnablementRequest, authoring.CommandID) (*protocol.PluginInstallation, error)
	UninstallPlugin(context.Context, protocol.PluginRequest, authoring.CommandID) error
}

func requirePluginRuntime(cmd *cobra.Command, provider runtimeProvider) (pluginRuntime, error) {
	runtime, profile, err := provider.Open(cmd)
	if err != nil {
		return nil, err
	}
	if !profile.Supports(protocol.FeaturePlugins) {
		return nil, errors.New("runtime plugins are unavailable")
	}
	supported, ok := runtime.(pluginRuntime)
	if !ok {
		return nil, errors.New("runtime plugin operations are unavailable")
	}
	return supported, nil
}

func newPluginsCommand(provider runtimeProvider) *cobra.Command {
	root := &cobra.Command{Use: "plugins", Short: "Manage Runtime-owned portable plugins"}
	root.AddCommand(newPluginGetCommand(provider))
	root.AddCommand(pluginCommand(provider, "retire-command", "Retire an original command only if it was never dispatched", func(runtime pluginRuntime, cmd *cobra.Command, request protocol.PluginCommandRequest, commandID authoring.CommandID) error {
		return runtime.RetirePluginCommand(cmd.Context(), request, commandID)
	}))
	root.AddCommand(newPluginCallCommand(provider))
	root.AddCommand(pluginResolveCommand(provider, "requirements", "Resolve one contribution's declared collaborators", "Exact installation, release digest and contribution as JSON", pluginRuntime.ResolvePluginRequirements))
	root.AddCommand(pluginResolveCommand(provider, "point", "Resolve values of a consumer-defined point", "Point request as JSON; contract and attributes contain JSON strings", pluginRuntime.ResolvePluginPoint))
	root.AddCommand(pluginResolveCommand(provider, "resolve-composition", "Resolve current bindings in the explicit composition order", "Consumer contract as a JSON string inside the request", pluginRuntime.ResolvePluginComposition))
	root.AddCommand(pluginListCommand(provider, "compositions", "Inspect explicit outer-to-inner plugin compositions", pluginRuntime.ListPluginCompositions))
	root.AddCommand(pluginListCommand(provider, "providers", "Inspect explicit plugin provider choices", pluginRuntime.ListPluginProviders))
	root.AddCommand(pluginListCommand(provider, "list", "Inspect releases, installation state and component diagnostics", pluginRuntime.ListPlugins))
	root.AddCommand(
		pluginMutationCommand(provider, "set-composition", "Set explicit outer-to-inner members for a consumer contract", pluginRuntime.SetPluginComposition),
		pluginMutationCommand(provider, "select-provider", "Select a declared provider without changing its enablement or private data", pluginRuntime.SelectPluginProvider),
		pluginMutationCommand(provider, "install", "Install an immutable directory or ZIP on the Runtime", pluginRuntime.InstallPlugin),
		pluginMutationCommand(provider, "stage", "Stage a release without changing active code", pluginRuntime.StagePlugin),
		pluginMutationCommand(provider, "select", "Select a staged release when dependencies are quiescent", pluginRuntime.SelectPlugin),
		pluginMutationCommand(provider, "configure", "Change inputs and enable or disable components of an exact selected release", pluginRuntime.ConfigurePlugin),
		pluginMutationCommand(provider, "set-enablement", "Enable or disable an installation with an explicit enabled value", pluginRuntime.SetPluginEnablement),
		pluginCommand(provider, "uninstall", "Remove installation while retaining private data", func(runtime pluginRuntime, cmd *cobra.Command, request protocol.PluginRequest, commandID authoring.CommandID) error {
			return runtime.UninstallPlugin(cmd.Context(), request, commandID)
		}),
	)
	return root
}

func pluginListCommand[T any](provider runtimeProvider, use, short string, list func(pluginRuntime, context.Context) ([]T, error)) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		values, err := list(runtime, cmd.Context())
		if err != nil {
			return err
		}
		return writePage(cmd, values)
	}}
}

func pluginResolveCommand[R, T any](provider runtimeProvider, use, short, requestHelp string, resolve func(pluginRuntime, context.Context, R) ([]T, error)) *cobra.Command {
	var input string
	command := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		request, err := decodePluginRequest[R](input)
		if err != nil {
			return err
		}
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		values, err := resolve(runtime, cmd.Context(), request)
		if err != nil {
			return err
		}
		return writePage(cmd, values)
	}}
	command.Flags().StringVar(&input, requestFlag, "{}", requestHelp)
	return command
}

func writePage[T any](cmd *cobra.Command, values []T) error {
	if values == nil {
		values = []T{}
	}
	return writeJSONLine(cmd.OutOrStdout(), protocol.Page[T]{Data: values})
}

func newPluginGetCommand(provider runtimeProvider) *cobra.Command {
	var input string
	command := &cobra.Command{Use: "get-command", Short: "Read an original admitted plugin request by its retained namespace and key", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		request, err := decodePluginRequest[protocol.PluginCommandRequest](input)
		if err != nil {
			return err
		}
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		original, err := runtime.GetPluginCommand(cmd.Context(), request)
		if err != nil {
			return err
		}
		return writeJSONLine(cmd.OutOrStdout(), original)
	}}
	command.Flags().StringVar(&input, requestFlag, "{}", "Original Runtime store namespace and command key as JSON")
	return command
}

func newPluginCallCommand(provider runtimeProvider) *cobra.Command {
	var input, key string
	command := &cobra.Command{Use: "call", Short: "Invoke an installed contribution through its required contract", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		request, err := decodePluginRequest[protocol.CallPluginRequest](input)
		if err != nil {
			return err
		}
		contract, err := plugincontract.ParseCapability([]byte(request.Contract))
		if err != nil {
			return err
		}
		method, found := contract.Method(request.Method)
		if !found {
			return errors.New("plugin method is absent from the required contract")
		}
		commandID := authoring.CommandID(key)
		if method.Kind() == frame.Command {
			if commandID == "" {
				commandID = mutation.NewCommandID()
			}
			if err := commandID.Validate(); err != nil {
				return err
			}
		} else if commandID != "" {
			return errors.New("plugin query cannot carry a command identity")
		}
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		if commandID != "" {
			if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "command-id:", commandID); err != nil {
				return err
			}
		}
		result, err := runtime.CallPlugin(cmd.Context(), request, commandID)
		if err != nil {
			return err
		}
		return writeJSONLine(cmd.OutOrStdout(), result)
	}}
	command.Flags().StringVar(&input, requestFlag, "{}", "Installed operation request as JSON; contract and input contain JSON strings")
	command.Flags().StringVar(&key, commandIDFlag, "", "Retain this identity and request to reconcile a command; queries forbid it")
	return command
}

func pluginMutationCommand[Input, Output any](provider runtimeProvider, name, description string, run func(pluginRuntime, context.Context, Input, authoring.CommandID) (Output, error)) *cobra.Command {
	return pluginCommand(provider, name, description, func(runtime pluginRuntime, cmd *cobra.Command, request Input, commandID authoring.CommandID) error {
		result, err := run(runtime, cmd.Context(), request, commandID)
		if err != nil {
			return err
		}
		return writeJSONLine(cmd.OutOrStdout(), result)
	})
}

func pluginCommand[Input any](provider runtimeProvider, name, description string, run func(pluginRuntime, *cobra.Command, Input, authoring.CommandID) error) *cobra.Command {
	var input, key string
	command := &cobra.Command{Use: name, Short: description, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		request, err := decodePluginRequest[Input](input)
		if err != nil {
			return err
		}
		commandID := authoring.CommandID(key)
		if commandID == "" {
			commandID = mutation.NewCommandID()
		}
		if err := commandID.Validate(); err != nil {
			return err
		}
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "command-id:", commandID); err != nil {
			return err
		}
		return run(runtime, cmd, request, commandID)
	}}
	command.Flags().StringVar(&input, requestFlag, "{}", "Exact Runtime request as JSON; source paths belong to the Runtime")
	command.Flags().StringVar(&key, commandIDFlag, "", "Retain this exact identity and request for replay after an unknown acknowledgement")
	return command
}

const maximumPluginRequestBytes = 1 << 20

func decodePluginRequest[T any](input string) (T, error) {
	var request T
	if len(input) > maximumPluginRequestBytes {
		return request, fmt.Errorf("plugin input exceeds %d bytes", maximumPluginRequestBytes)
	}
	if err := protocol.DecodeRequest([]byte(input), &request); err != nil {
		return request, fmt.Errorf("plugin request: %w", err)
	}
	return request, nil
}
