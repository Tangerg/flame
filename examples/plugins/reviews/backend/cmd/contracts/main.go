// Command contracts projects the Reviews owner's current typed methods for
// contractgen. It does not open private data or execute a business operation.
package main

import (
	json "encoding/json/v2"
	"flag"
	"fmt"
	"os"
	"strconv"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend"
	"github.com/Tangerg/flame/packages/go-sdk/capability"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	analyzer := flag.Bool("analyzers", false, "Emit Reviews' consumer-owned analyzer point")
	analyzerTypeScript := flag.Bool("analyzers-ts", false, "Emit the exact analyzer point for TypeScript consumers")
	analyzerCapability := flag.Bool("analyzer-capability", false, "Emit Reviews' callable analyzer contract")
	flag.Parse()
	if *analyzerCapability && (*analyzer || *analyzerTypeScript) || *analyzer && *analyzerTypeScript {
		return fmt.Errorf("select one contract projection")
	}
	if *analyzer || *analyzerTypeScript {
		contract, err := reviews.NewAnalyzerPoint()
		if err != nil {
			return err
		}
		if *analyzerTypeScript {
			_, err := fmt.Fprintf(os.Stdout, "// Code generated from Reviews' Go-owned analyzer point. DO NOT EDIT.\nexport const ANALYZERS_JSON = %s;\n", strconv.Quote(string(contract.Identity().JSON())))
			return err
		}
		data, err := json.Marshal(contract.Definition(), json.Deterministic(true))
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(append(data, '\n'))
		return err
	}
	var contract *capability.Contract
	if *analyzerCapability {
		contracts, err := reviews.NewAnalyzerContracts()
		if err != nil {
			return err
		}
		contract = contracts.Capability
	} else {
		contracts, err := reviews.NewContracts()
		if err != nil {
			return err
		}
		contract = contracts.Capability
	}
	data, err := json.Marshal(contract.Descriptor(), json.Deterministic(true))
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(data, '\n'))
	return err
}
