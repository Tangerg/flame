package main

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	hookmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider/mcp"
)

func main() { plugincontract.Publish(hookmcp.Contract) }
