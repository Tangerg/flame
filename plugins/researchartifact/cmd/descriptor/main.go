package main

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/plugins/researchartifact"
)

func main() {
	plugincontract.Publish(func() (*plugincontract.Capability, error) {
		contract, _, err := researchartifact.Contract()
		return contract, err
	})
}
