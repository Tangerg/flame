package cmd

import (
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/spf13/cobra"
)

func pageSizeFromFlag(command *cobra.Command, name string, rows int) (conversation.PageSize, error) {
	if !command.Flags().Changed(name) {
		return conversation.DefaultPageSize(), nil
	}
	return conversation.NewPageSize(rows)
}
