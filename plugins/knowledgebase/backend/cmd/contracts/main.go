package main

import (
	json "encoding/json/v2"
	"fmt"
	"os"

	"github.com/Tangerg/flame/plugins/knowledgebase/backend/rpc"
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
