package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/delivery/cmd/render"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/spf13/cobra"
)

type pluginRuntime interface {
	ListPlugins(context.Context) ([]protocol.PluginInstallation, error)
	InstallPlugin(context.Context, protocol.InstallPluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	StagePlugin(context.Context, protocol.StagePluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	SelectPlugin(context.Context, protocol.PluginReleaseRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	ApprovePlugin(context.Context, protocol.PluginReleaseRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	ConfigurePlugin(context.Context, protocol.ConfigurePluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	SetPluginEnablement(context.Context, protocol.SetPluginEnablementRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	RevokePlugin(context.Context, protocol.PluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	UninstallPlugin(context.Context, protocol.PluginRequest, replay.CommandID) error
	RenamePluginSession(context.Context, protocol.RenamePluginSessionRequest, replay.CommandID) (*protocol.Session, error)
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
		pluginMutationCommand(provider, "rename-session", "Submit a declared action to rename an exact Session revision", pluginRuntime.RenamePluginSession),
		pluginMutationCommand(provider, "install", "Install an immutable directory or ZIP on the Runtime", pluginRuntime.InstallPlugin),
		pluginMutationCommand(provider, "stage", "Stage a release without changing active code", pluginRuntime.StagePlugin),
		pluginMutationCommand(provider, "select", "Select a staged release when dependencies are quiescent", pluginRuntime.SelectPlugin),
		pluginMutationCommand(provider, "approve", "Approve the exact selected release digest", pluginRuntime.ApprovePlugin),
		pluginMutationCommand(provider, "configure", "Change inputs and enable or disable components of an exact selected release", pluginRuntime.ConfigurePlugin),
		pluginMutationCommand(provider, "set-enablement", "Enable or disable an installation with an explicit enabled value", pluginRuntime.SetPluginEnablement),
		pluginMutationCommand(provider, "revoke", "Revoke trust and retire backend resources", pluginRuntime.RevokePlugin),
		pluginCommand(provider, "uninstall", "Remove installation while retaining private data", func(runtime pluginRuntime, cmd *cobra.Command, request protocol.PluginRequest, commandID replay.CommandID) error {
			return runtime.UninstallPlugin(cmd.Context(), request, commandID)
		}),
	)
	return root
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
