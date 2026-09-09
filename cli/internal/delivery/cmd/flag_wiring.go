package cmd

import "github.com/spf13/cobra"

// Cobra reports a flag that does not exist, so both helpers below can only fail
// on a wiring defect in this package: a renamed or missing flag. Discarding that
// error drops the constraint the call declares — for a revision flag, silently
// turning a compare-and-set update into an unconditional one — so the command
// tree refuses to be built instead. Every command-tree test exercises this.

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
