package cmd

import (
	"time"

	"github.com/Tangerg/acp"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	acpdelivery "github.com/Tangerg/flame/apps/cli/internal/delivery/acp"
	"github.com/spf13/cobra"
)

func newACPCommand(provider runtimeProvider, transport acp.Transport) *cobra.Command {
	return &cobra.Command{
		Use:   "acp",
		Short: "Serve the Agent Client Protocol over stdin and stdout",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			configured, err := readSettings(provider.configuration)
			if err != nil {
				return err
			}
			options, err := configured.RunOptions()
			if err != nil {
				return err
			}
			runtime, profile, err := provider.Open(command)
			if err != nil {
				return err
			}
			replayPolicy, err := mutation.NewReplayPolicy(profile.IdempotencyLimits(), time.Now)
			if err != nil {
				return err
			}
			return acpdelivery.Serve(command.Context(), transport, runtime, options, replayPolicy, Version(), command.ErrOrStderr())
		},
	}
}
