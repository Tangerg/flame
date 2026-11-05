package main

import (
	json "encoding/json/v2"
	"log"
	"os"

	goalmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/goalworkflow/mcp"
)

func main() {
	contract, err := goalmcp.Contract()
	if err == nil {
		err = json.MarshalWrite(os.Stdout, contract.Descriptor())
	}
	if err != nil {
		log.Fatal(err)
	}
}
