package main

import (
	json "encoding/json/v2"
	"flag"
	"fmt"
	"os"
	"strconv"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type projection string

const (
	capabilityProjection         projection = "capability"
	analyzersProjection          projection = "analyzers"
	boardProjection              projection = "board-ts"
	analyzerCapabilityProjection projection = "analyzer-capability"
)

func run() error {
	selected := flag.String("projection", string(capabilityProjection), "Contract artifact to emit: capability, analyzers, board-ts or analyzer-capability")
	flag.Parse()
	switch projection(*selected) {
	case capabilityProjection:
		contracts, err := reviews.NewContracts()
		if err != nil {
			return err
		}
		return writeJSON(contracts.Capability.Descriptor())
	case analyzerCapabilityProjection:
		contracts, err := reviews.NewAnalyzerContracts()
		if err != nil {
			return err
		}
		return writeJSON(contracts.Capability.Descriptor())
	case analyzersProjection:
		contract, err := reviews.NewAnalyzerPoint()
		if err != nil {
			return err
		}
		return writeJSON(contract.Definition())
	case boardProjection:
		return writeBoard()
	default:
		return fmt.Errorf("unknown contract projection %q", *selected)
	}
}

func writeJSON(value any) error {
	data, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(data, '\n'))
	return err
}

func writeBoard() error {
	contract, err := reviews.NewAnalyzerPoint()
	if err != nil {
		return err
	}
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
