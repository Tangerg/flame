package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func pageSizeFromFlag(command *cobra.Command, name string, rows int) (agent.PageSize, error) {
	if !command.Flags().Changed(name) {
		return agent.DefaultPageSize(), nil
	}
	return agent.NewPageSize(rows)
}
