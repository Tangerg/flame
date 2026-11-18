package main

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	connectormcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcpconnector/mcp"
)

func main() {
	plugincontract.Publish(func() (*plugincontract.Capability, error) {
		methods, err := connectormcp.NewMethods()
		return methods.Contract, err
	})
}
