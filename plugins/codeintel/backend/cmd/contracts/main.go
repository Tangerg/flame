package main

import (
	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
)

func main() { plugincontract.Publish(codeintel.Contract) }
