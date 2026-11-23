package main

import (
	json "encoding/json/v2"
	"flag"
	"fmt"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/plugin"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
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
	return writeModelContract(modelContractRole(*role))
}

type modelContractRole string

const (
	modelContextRole   modelContractRole = "context"
	modelInputRole     modelContractRole = "input"
	modelSelectionRole modelContractRole = "selection"
)

func writeModelContract(role modelContractRole) error {
	var contract *plugincontract.Capability
	var err error
	switch role {
	case modelContextRole:
		contract, err = plugin.ModelContextContract()
	case modelInputRole:
		contract, err = plugin.ModelInputContract()
	case modelSelectionRole:
		contract, err = plugin.ModelSelectionContract()
	default:
		return fmt.Errorf("unknown model contract %q", role)
	}
	if err != nil {
		return err
	}
	return json.MarshalWrite(os.Stdout, contract.Descriptor())
}
