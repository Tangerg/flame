package main

import (
	json "encoding/json/v2"
	"fmt"
	"os"

	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

func main() {
	contract, err := pluginsdk.ModelSelectionContract()
	if err == nil {
		err = json.MarshalWrite(os.Stdout, contract.Descriptor())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
