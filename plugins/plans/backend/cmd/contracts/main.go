package main

import (
	json "encoding/json/v2"
	planmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/planworkflow/mcp"
	"log"
	"os"
)

func main() {
	contract, err := planmcp.Contract()
	if err == nil {
		err = json.MarshalWrite(os.Stdout, contract.Descriptor())
	}
	if err != nil {
		log.Fatal(err)
	}
}
