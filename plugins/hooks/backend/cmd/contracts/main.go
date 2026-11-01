package main

import (
	json "encoding/json/v2"
	hookmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider/mcp"
	"log"
	"os"
)

func main() {
	contract, err := hookmcp.Contract()
	if err == nil {
		err = json.MarshalWrite(os.Stdout, contract.Descriptor())
	}
	if err != nil {
		log.Fatal(err)
	}
}
