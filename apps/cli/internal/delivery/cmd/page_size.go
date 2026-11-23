package cmd

import (
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/spf13/cobra"
)

const pageSizeFlag = "limit"

func addPageSizeFlag(command *cobra.Command, rows *int, noun string) {
	command.Flags().IntVarP(rows, pageSizeFlag, "n", conversation.DefaultPageRows, fmt.Sprintf("Maximum %s to return (up to %d)", noun, conversation.MaximumPageRows))
}

func pageSizeFromFlag(command *cobra.Command, rows int) (conversation.PageSize, error) {
	if !command.Flags().Changed(pageSizeFlag) {
		return conversation.DefaultPageSize(), nil
	}
	return conversation.NewPageSize(rows)
}
