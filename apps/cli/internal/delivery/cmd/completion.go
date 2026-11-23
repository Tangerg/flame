package cmd

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

type completionScript struct {
	shell    string
	generate func(root *cobra.Command, out io.Writer) error
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	completionScripts := []completionScript{
		{shell: "bash", generate: (*cobra.Command).GenBashCompletion},
		{shell: "zsh", generate: (*cobra.Command).GenZshCompletion},
		{shell: "fish", generate: func(root *cobra.Command, out io.Writer) error { return root.GenFishCompletion(out, true) }},
		{shell: "powershell", generate: (*cobra.Command).GenPowerShellCompletionWithDesc},
	}
	shells := make([]string, len(completionScripts))
	for index, script := range completionScripts {
		shells[index] = script.shell
	}
	return &cobra.Command{
		Use:         "completion <" + strings.Join(shells, "|") + ">",
		Short:       "Generate a shell completion script",
		Args:        cobra.ExactArgs(1),
		ValidArgs:   shells,
		Annotations: map[string]string{configIndependentAnnotation: annotationSet},
		RunE: func(cmd *cobra.Command, args []string) error {
			index := slices.IndexFunc(completionScripts, func(script completionScript) bool { return script.shell == args[0] })
			if index < 0 {
				return fmt.Errorf("unsupported shell %q", args[0])
			}
			return completionScripts[index].generate(root, cmd.OutOrStdout())
		},
	}
}
