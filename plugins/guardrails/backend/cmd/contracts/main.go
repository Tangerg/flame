package main

import (
	json "encoding/json/v2"
	"fmt"
	"os"

	rpc "github.com/Tangerg/flame/plugins/guardrails/internal/contract"
)

func main() {
	contracts, err := rpc.NewContracts()
	if err == nil {
		err = json.MarshalWrite(os.Stdout, contracts.Capability.Descriptor())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
