// Command contracts projects the Reviews owner's current typed methods for
// contractgen. It does not open private data or execute a business operation.
package main

import (
	json "encoding/json/v2"
	"fmt"
	"os"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	contracts, err := reviews.NewContracts()
	if err != nil {
		return err
	}
	data, err := json.Marshal(contracts.Capability.Descriptor(), json.Deterministic(true))
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(data, '\n'))
	return err
}
