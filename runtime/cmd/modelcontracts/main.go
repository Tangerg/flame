package main

import (
	json "encoding/json/v2"
	"fmt"
	"os"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/runtime/capability"
)

// modelcontracts projects a plugin-sdk model-role contract descriptor as JSON on
// stdout. First-party model-role backends embed exactly one kind, so the kind is a
// required positional argument over a closed set.
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: modelcontracts <context|input|selection>")
	}
	var contract *capability.Contract
	var err error
	switch args[0] {
	case "context":
		contract, err = pluginsdk.ModelContextContract()
	case "input":
		contract, err = pluginsdk.ModelInputContract()
	case "selection":
		contract, err = pluginsdk.ModelSelectionContract()
	default:
		return fmt.Errorf("unknown model contract %q", args[0])
	}
	if err != nil {
		return err
	}
	return json.MarshalWrite(os.Stdout, contract.Descriptor())
}
