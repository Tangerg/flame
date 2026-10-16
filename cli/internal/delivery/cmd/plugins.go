package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/delivery/cmd/render"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/spf13/cobra"
)

type pluginRuntime interface {
	GetPluginCommand(context.Context, protocol.PluginCommandRequest) (*protocol.CallPluginRequest, error)
	RetirePluginCommand(context.Context, protocol.PluginCommandRequest, replay.CommandID) error
	ListPluginCompositions(context.Context) ([]protocol.PluginComposition, error)
	SetPluginComposition(context.Context, protocol.SetPluginCompositionRequest, replay.CommandID) (*protocol.PluginComposition, error)
	ResolvePluginComposition(context.Context, protocol.ResolvePluginCompositionRequest) ([]protocol.PluginOperationBinding, error)
	ResolvePluginPoint(context.Context, protocol.ResolvePluginPointRequest) ([]protocol.PluginPointBinding, error)
	ResolvePluginRequirements(context.Context, protocol.ResolvePluginRequirementsRequest) ([]protocol.PluginCollaborator, error)
	CallPlugin(context.Context, protocol.CallPluginRequest, replay.CommandID) (*protocol.CallPluginResponse, error)
	ListPluginProviders(context.Context) ([]protocol.PluginProviderSelection, error)
	SelectPluginProvider(context.Context, protocol.SelectPluginProviderRequest, replay.CommandID) (*protocol.PluginProviderSelection, error)
	ListPlugins(context.Context) ([]protocol.PluginInstallation, error)
	InstallPlugin(context.Context, protocol.InstallPluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	StagePlugin(context.Context, protocol.StagePluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	SelectPlugin(context.Context, protocol.PluginReleaseRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	ConfigurePlugin(context.Context, protocol.ConfigurePluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	SetPluginEnablement(context.Context, protocol.SetPluginEnablementRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	UninstallPlugin(context.Context, protocol.PluginRequest, replay.CommandID) error
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
	root.AddCommand(pluginCommand(provider, "retire-command", "Retire an original command only if it was never dispatched", func(runtime pluginRuntime, cmd *cobra.Command, request protocol.PluginCommandRequest, commandID replay.CommandID) error {
		return runtime.RetirePluginCommand(cmd.Context(), request, commandID)
	}))
	root.AddCommand(newPluginCallCommand(provider))
	root.AddCommand(newPluginRequirementsCommand(provider))
	root.AddCommand(newPluginPointCommand(provider))
	root.AddCommand(newPluginResolveCompositionCommand(provider))
	root.AddCommand(&cobra.Command{Use: "compositions", Short: "Inspect explicit outer-to-inner plugin compositions", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		values, err := runtime.ListPluginCompositions(cmd.Context())
		if err != nil {
			return err
		}
		if values == nil {
			values = []protocol.PluginComposition{}
		}
		return render.WriteJSONLine(cmd.OutOrStdout(), protocol.Page[protocol.PluginComposition]{Data: values})
	}})
	root.AddCommand(&cobra.Command{Use: "providers", Short: "Inspect explicit plugin provider choices", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		values, err := runtime.ListPluginProviders(cmd.Context())
		if err != nil {
			return err
		}
		if values == nil {
			values = []protocol.PluginProviderSelection{}
		}
		return render.WriteJSONLine(cmd.OutOrStdout(), protocol.Page[protocol.PluginProviderSelection]{Data: values})
	}})
	root.AddCommand(&cobra.Command{Use: "list", Short: "Inspect releases, installation state and component diagnostics", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		installations, err := runtime.ListPlugins(cmd.Context())
		if err != nil {
			return err
		}
		if installations == nil {
			installations = []protocol.PluginInstallation{}
		}
		return render.WriteJSONLine(cmd.OutOrStdout(), protocol.Page[protocol.PluginInstallation]{Data: installations})
	}})
	root.AddCommand(
		pluginMutationCommand(provider, "set-composition", "Set explicit outer-to-inner members for a consumer contract", pluginRuntime.SetPluginComposition),
		pluginMutationCommand(provider, "select-provider", "Select a declared provider without changing its enablement or private data", pluginRuntime.SelectPluginProvider),
		pluginMutationCommand(provider, "install", "Install an immutable directory or ZIP on the Runtime", pluginRuntime.InstallPlugin),
		pluginMutationCommand(provider, "stage", "Stage a release without changing active code", pluginRuntime.StagePlugin),
		pluginMutationCommand(provider, "select", "Select a staged release when dependencies are quiescent", pluginRuntime.SelectPlugin),
		pluginMutationCommand(provider, "configure", "Change inputs and enable or disable components of an exact selected release", pluginRuntime.ConfigurePlugin),
		pluginMutationCommand(provider, "set-enablement", "Enable or disable an installation with an explicit enabled value", pluginRuntime.SetPluginEnablement),
		pluginCommand(provider, "uninstall", "Remove installation while retaining private data", func(runtime pluginRuntime, cmd *cobra.Command, request protocol.PluginRequest, commandID replay.CommandID) error {
			return runtime.UninstallPlugin(cmd.Context(), request, commandID)
		}),
	)
	return root
}

func newPluginGetCommand(provider runtimeProvider) *cobra.Command {
	var input string
	command := &cobra.Command{Use: "get-command", Short: "Read an original admitted plugin request by its retained namespace and key", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if len(input) > 1<<20 {
			return errors.New("plugin input exceeds 1 MiB")
		}
		var request protocol.PluginCommandRequest
		if err := protocol.DecodeRequest([]byte(input), &request); err != nil {
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
		return render.WriteJSONLine(cmd.OutOrStdout(), original)
	}}
	command.Flags().StringVar(&input, "request", "{}", "Original Runtime store namespace and command key as JSON")
	return command
}

func newPluginResolveCompositionCommand(provider runtimeProvider) *cobra.Command {
	var input string
	command := &cobra.Command{Use: "resolve-composition", Short: "Resolve current bindings in the explicit composition order", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if len(input) > 1<<20 {
			return errors.New("plugin input exceeds 1 MiB")
		}
		var request protocol.ResolvePluginCompositionRequest
		if err := protocol.DecodeRequest([]byte(input), &request); err != nil {
			return err
		}
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		values, err := runtime.ResolvePluginComposition(cmd.Context(), request)
		if err != nil {
			return err
		}
		if values == nil {
			values = []protocol.PluginOperationBinding{}
		}
		return render.WriteJSONLine(cmd.OutOrStdout(), protocol.Page[protocol.PluginOperationBinding]{Data: values})
	}}
	command.Flags().StringVar(&input, "request", "{}", "Consumer contract as a JSON string inside the request")
	return command
}

func newPluginRequirementsCommand(provider runtimeProvider) *cobra.Command {
	var input string
	command := &cobra.Command{Use: "requirements", Short: "Resolve one contribution's declared collaborators", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if len(input) > 1<<20 {
			return errors.New("plugin input exceeds 1 MiB")
		}
		var request protocol.ResolvePluginRequirementsRequest
		if err := protocol.DecodeRequest([]byte(input), &request); err != nil {
			return err
		}
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		values, err := runtime.ResolvePluginRequirements(cmd.Context(), request)
		if err != nil {
			return err
		}
		if values == nil {
			values = []protocol.PluginCollaborator{}
		}
		return render.WriteJSONLine(cmd.OutOrStdout(), protocol.Page[protocol.PluginCollaborator]{Data: values})
	}}
	command.Flags().StringVar(&input, "request", "{}", "Exact installation, release digest and contribution as JSON")
	return command
}

func newPluginPointCommand(provider runtimeProvider) *cobra.Command {
	var input string
	command := &cobra.Command{Use: "point", Short: "Resolve values of a consumer-defined point", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if len(input) > 1<<20 {
			return errors.New("plugin input exceeds 1 MiB")
		}
		var request protocol.ResolvePluginPointRequest
		if err := protocol.DecodeRequest([]byte(input), &request); err != nil {
			return err
		}
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		values, err := runtime.ResolvePluginPoint(cmd.Context(), request)
		if err != nil {
			return err
		}
		if values == nil {
			values = []protocol.PluginPointBinding{}
		}
		return render.WriteJSONLine(cmd.OutOrStdout(), protocol.Page[protocol.PluginPointBinding]{Data: values})
	}}
	command.Flags().StringVar(&input, "request", "{}", "Point request as JSON; contract and attributes contain JSON strings")
	return command
}

func newPluginCallCommand(provider runtimeProvider) *cobra.Command {
	var input, key string
	command := &cobra.Command{Use: "call", Short: "Invoke an installed contribution through its required contract", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if len(input) > 1<<20 {
			return errors.New("plugin input exceeds 1 MiB")
		}
		var request protocol.CallPluginRequest
		if err := protocol.DecodeRequest([]byte(input), &request); err != nil {
			return fmt.Errorf("plugin request: %w", err)
		}
		contract, err := capability.Parse([]byte(request.Contract))
		if err != nil {
			return err
		}
		method, found := contract.Method(request.Method)
		if !found {
			return errors.New("plugin method is absent from the required contract")
		}
		commandID := replay.CommandID(key)
		if method.Kind() == capability.Mutation {
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
		return render.WriteJSONLine(cmd.OutOrStdout(), result)
	}}
	command.Flags().StringVar(&input, "request", "{}", "Installed operation request as JSON; contract and input contain JSON strings")
	command.Flags().StringVar(&key, "command-id", "", "Retain this identity and request to reconcile a command; queries forbid it")
	return command
}

// pluginMutationCommand prints the canonical result the Runtime reports.
func pluginMutationCommand[Input, Output any](provider runtimeProvider, name, description string, run func(pluginRuntime, context.Context, Input, replay.CommandID) (Output, error)) *cobra.Command {
	return pluginCommand(provider, name, description, func(runtime pluginRuntime, cmd *cobra.Command, request Input, commandID replay.CommandID) error {
		result, err := run(runtime, cmd.Context(), request, commandID)
		if err != nil {
			return err
		}
		return render.WriteJSONLine(cmd.OutOrStdout(), result)
	})
}

func pluginCommand[Input any](provider runtimeProvider, name, description string, run func(pluginRuntime, *cobra.Command, Input, replay.CommandID) error) *cobra.Command {
	var input, key string
	command := &cobra.Command{Use: name, Short: description, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if len(input) > 1<<20 {
			return errors.New("plugin input exceeds 1 MiB")
		}
		var request Input
		if err := protocol.DecodeRequest([]byte(input), &request); err != nil {
			return fmt.Errorf("plugin request: %w", err)
		}
		commandID := replay.CommandID(key)
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
	command.Flags().StringVar(&input, "request", "{}", "Exact Runtime request as JSON; source paths belong to the Runtime")
	command.Flags().StringVar(&key, "command-id", "", "Retain this exact identity and request for replay after an unknown acknowledgement")
	return command
}
