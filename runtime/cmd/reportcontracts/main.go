package main

import (
	json "encoding/json/v2"
	"fmt"
	"os"

	"github.com/Tangerg/flame/runtime/internal/plugins/usagereports/rpc"
	"github.com/Tangerg/flame/runtime/protocol"
)

func main() {
	contracts, err := rpc.NewContracts[protocol.UsageSummary]()
	if err == nil {
		err = json.MarshalWrite(os.Stdout, contracts.Capability.Descriptor())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
