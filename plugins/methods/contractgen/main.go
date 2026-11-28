package main

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/plugins/methods/internal/contract"
)

func main() {
	plugincontract.Publish(func() (*plugincontract.Capability, error) {
		methods, err := contract.New()
		if err != nil {
			return nil, err
		}
		return methods.Capability()
	})
}
