package main

import (
	json "encoding/json/v2"
	"log"
	"os"

	rpc "github.com/Tangerg/flame/plugins/a2a/internal/contract"
)

func main() {
	contracts, err := rpc.NewContracts()
	if err == nil {
		err = json.MarshalWrite(os.Stdout, contracts.Capability.Descriptor())
	}
	if err != nil {
		log.Fatal(err)
	}
}
