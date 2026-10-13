package main

import (
	json "encoding/json/v2"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	"log"
	"os"
)

func main() {
	contracts, err := rpc.NewContracts()
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.Marshal(contracts.Capability.Descriptor())
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		log.Fatal(err)
	}
}
