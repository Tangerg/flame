// Command contracts projects the Reviews owner's current typed methods for
// contractgen. It does not open private data or execute a business operation.
package main

import (
	json "encoding/json/v2"
	"flag"
	"fmt"
	"os"
	"strconv"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	analyzer := flag.Bool("analyzers", false, "Emit Reviews' consumer-owned analyzer point")
	board := flag.Bool("board-ts", false, "Emit the Reviews board bindings")
	analyzerCapability := flag.Bool("analyzer-capability", false, "Emit Reviews' callable analyzer contract")
	flag.Parse()
	if *analyzerCapability && (*analyzer || *board) || *analyzer && *board {
		return fmt.Errorf("select one contract projection")
	}
	if *analyzer || *board {
		contract, err := reviews.NewAnalyzerPoint()
		if err != nil {
			return err
		}
		if *board {
			methods, err := reviews.NewContracts()
			if err != nil {
				return err
			}
			statuses, err := json.Marshal(reviews.Status("").JSONSchemaEnum())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(os.Stdout, "// Code generated from the Reviews contract. DO NOT EDIT.\nexport const ANALYZERS_JSON = %s;\nexport const listMethod = %q;\nexport const updateMethod = %q;\nexport const analyzeMethod = %q;\nexport const reviewStatuses = %s as const;\n", strconv.Quote(string(contract.Identity().Bytes())), methods.List.Name(), methods.Update.Name(), methods.Analyzers.Name(), statuses)
			return err
		}
		data, err := json.Marshal(contract.Definition(), json.Deterministic(true))
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(append(data, '\n'))
		return err
	}
	var contract *plugincontract.Capability
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
