package main

import (
	json "encoding/json/v2"
	"flag"
	"fmt"
	"github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/qimu/portable"
	"os"
)

func runContract(args []string) error {
	flags := flag.NewFlagSet("flame-sdk contract", flag.ContinueOnError)
	role := flags.String("model-contract", "", "Flame model role: context, input or selection")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *role == "" {
		return fmt.Errorf("one model-contract is required")
	}
	return writeModelContract(*role)
}
func writeModelContract(role string) error {
	var contract *portable.Capability
	var err error
	switch role {
	case "context":
		contract, err = plugin.ModelContextContract()
	case "input":
		contract, err = plugin.ModelInputContract()
	case "selection":
		contract, err = plugin.ModelSelectionContract()
	default:
		return fmt.Errorf("unknown model contract %q", role)
	}
	if err != nil {
		return err
	}
	return json.MarshalWrite(os.Stdout, contract.Descriptor())
}
