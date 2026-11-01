package main

import (
	json "encoding/json/v2"
	"log"
	"os"

	connectormcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcpconnector/mcp"
)

func main() {
	methods, err := connectormcp.NewMethods()
	if err == nil {
		err = json.MarshalWrite(os.Stdout, methods.Contract.Descriptor())
	}
	if err != nil {
		log.Fatal(err)
	}
}
