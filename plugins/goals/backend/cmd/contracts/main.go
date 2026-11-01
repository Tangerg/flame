package main

import (
	json "encoding/json/v2"
	goalmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/goalworkflow/mcp"
	"log"
	"os"
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
