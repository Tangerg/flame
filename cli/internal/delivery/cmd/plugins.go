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
	ListPlugins(context.Context) (*protocol.Page[protocol.PluginInstallation], error)
	InstallPlugin(context.Context, protocol.InstallPluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	StagePlugin(context.Context, protocol.StagePluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	SelectPlugin(context.Context, protocol.PluginReleaseRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	ApprovePlugin(context.Context, protocol.ApprovePluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	ConfigurePlugin(context.Context, protocol.ConfigurePluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	SetPluginEnablement(context.Context, protocol.SetPluginEnablementRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	RevokePlugin(context.Context, protocol.PluginRequest, replay.CommandID) (*protocol.PluginInstallation, error)
	UninstallPlugin(context.Context, protocol.PluginRequest, replay.CommandID) (*protocol.PluginRemoval, error)
}

func requirePluginRuntime(cmd *cobra.Command, provider runtimeProvider) (pluginRuntime, error) {
	runtime, profile, err := provider.Open(cmd)
	if err != nil {
		return nil, err
	}
	if profile != nil && !profile.Supports(protocol.FeaturePlugins) {
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
	root.AddCommand(&cobra.Command{Use: "list", Short: "Inspect releases, grants and component diagnostics", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		runtime, err := requirePluginRuntime(cmd, provider)
		if err != nil {
			return err
		}
		result, err := runtime.ListPlugins(cmd.Context())
		if err != nil {
			return err
		}
		return render.WriteJSONLine(cmd.OutOrStdout(), result)
	}})
	root.AddCommand(pluginMutationCommand[protocol.InstallPluginRequest, *protocol.PluginInstallation](provider, "install", "Install an immutable directory or ZIP on the Runtime", func(ctx context.Context, runtime pluginRuntime, in protocol.InstallPluginRequest, id replay.CommandID) (*protocol.PluginInstallation, error) {
		return runtime.InstallPlugin(ctx, in, id)
	}))
	root.AddCommand(pluginMutationCommand[protocol.StagePluginRequest, *protocol.PluginInstallation](provider, "stage", "Stage a release without changing active code", func(ctx context.Context, runtime pluginRuntime, in protocol.StagePluginRequest, id replay.CommandID) (*protocol.PluginInstallation, error) {
		return runtime.StagePlugin(ctx, in, id)
	}))
	root.AddCommand(pluginMutationCommand[protocol.PluginReleaseRequest, *protocol.PluginInstallation](provider, "select", "Select a staged release when dependencies are quiescent", func(ctx context.Context, runtime pluginRuntime, in protocol.PluginReleaseRequest, id replay.CommandID) (*protocol.PluginInstallation, error) {
		return runtime.SelectPlugin(ctx, in, id)
	}))
	root.AddCommand(pluginMutationCommand[protocol.ApprovePluginRequest, *protocol.PluginInstallation](provider, "approve", "Trust an exact release and the supplied grant subset", func(ctx context.Context, runtime pluginRuntime, in protocol.ApprovePluginRequest, id replay.CommandID) (*protocol.PluginInstallation, error) {
		return runtime.ApprovePlugin(ctx, in, id)
	}))
	root.AddCommand(pluginMutationCommand[protocol.ConfigurePluginRequest, *protocol.PluginInstallation](provider, "configure", "Configure inputs and components for an exact selected release", func(ctx context.Context, runtime pluginRuntime, in protocol.ConfigurePluginRequest, id replay.CommandID) (*protocol.PluginInstallation, error) {
		return runtime.ConfigurePlugin(ctx, in, id)
	}))
	root.AddCommand(pluginMutationCommand[protocol.SetPluginEnablementRequest, *protocol.PluginInstallation](provider, "enable", "Set desired enablement", func(ctx context.Context, runtime pluginRuntime, in protocol.SetPluginEnablementRequest, id replay.CommandID) (*protocol.PluginInstallation, error) {
		return runtime.SetPluginEnablement(ctx, in, id)
	}))
	root.AddCommand(pluginMutationCommand[protocol.PluginRequest, *protocol.PluginInstallation](provider, "revoke", "Revoke trust and retire backend resources", func(ctx context.Context, runtime pluginRuntime, in protocol.PluginRequest, id replay.CommandID) (*protocol.PluginInstallation, error) {
		return runtime.RevokePlugin(ctx, in, id)
	}))
	root.AddCommand(pluginMutationCommand[protocol.PluginRequest, *protocol.PluginRemoval](provider, "uninstall", "Remove installation while retaining private data", func(ctx context.Context, runtime pluginRuntime, in protocol.PluginRequest, id replay.CommandID) (*protocol.PluginRemoval, error) {
		return runtime.UninstallPlugin(ctx, in, id)
	}))
	return root
}

func pluginMutationCommand[Input, Output any](provider runtimeProvider, name, description string, run func(context.Context, pluginRuntime, Input, replay.CommandID) (Output, error)) *cobra.Command {
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
		result, err := run(cmd.Context(), runtime, request, commandID)
		if err != nil {
			return err
		}
		return render.WriteJSONLine(cmd.OutOrStdout(), result)
	}}
	command.Flags().StringVar(&input, "request", "{}", "Exact Runtime request as JSON; source paths belong to the Runtime")
	command.Flags().StringVar(&key, "command-id", "", "Retain this exact identity and request for replay after an unknown acknowledgement")
	return command
}
