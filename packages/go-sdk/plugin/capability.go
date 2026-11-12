package plugin

import (
	"sync"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
)

func publishedCapability(create func() (*plugincontract.Capability, error)) func() (*plugincontract.Capability, error) {
	admitted := sync.OnceValues(create)
	return func() (*plugincontract.Capability, error) {
		contract, err := admitted()
		if err != nil {
			return nil, err
		}
		return new(*contract), nil
	}
}
