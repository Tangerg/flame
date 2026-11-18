package main

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
)

func main() {
	plugincontract.Publish(func() (*plugincontract.Capability, error) {
		contracts, err := rpc.NewContracts()
		return contracts.Capability, err
	})
}
