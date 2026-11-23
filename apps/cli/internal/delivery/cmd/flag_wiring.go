package cmd

import "github.com/spf13/cobra"

const (
	commandIDFlag    = "command-id"
	configFlag       = "config"
	cursorFlag       = "cursor"
	cwdFlag          = "cwd"
	favoriteFlag     = "favorite"
	fileFlag         = "file"
	jsonFlag         = "json"
	modelFlag        = "model"
	outputFormatFlag = "output-format"
	providerFlag     = "provider"
	requestFlag      = "request"
	sessionFlag      = "session"
	statusFlag       = "status"
	titleFlag        = "title"
	workspaceFlag    = "workspace"
	yesFlag          = "yes"
)

// Cobra fails here only for a renamed or missing flag; discarding that error would
// silently drop the declared constraint, such as turning a compare-and-set update
// into an unconditional one.

func requireFlag(command *cobra.Command, name string) {
	if err := command.MarkFlagRequired(name); err != nil {
		panic("cmd: require flag: " + err.Error())
	}
}

func completeFlag(command *cobra.Command, name string, complete cobra.CompletionFunc) {
	if err := command.RegisterFlagCompletionFunc(name, complete); err != nil {
		panic("cmd: register flag completion: " + err.Error())
	}
}

func addRevisionFlag(command *cobra.Command, revision *uint64) {
	const name = "revision"
	command.Flags().Uint64Var(revision, name, 0, "Revision previously read from sessions ls/show")
	requireFlag(command, name)
}
