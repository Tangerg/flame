package main

import (
	json "encoding/json/v2"
	"fmt"
	"os"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
)

func main() {
	contract, err := pluginsdk.ModelInputContract()
	if err == nil {
		err = json.MarshalWrite(os.Stdout, contract.Descriptor())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
