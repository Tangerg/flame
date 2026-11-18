package main

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
)

func main() { plugincontract.Publish(modelprovider.Contract) }
