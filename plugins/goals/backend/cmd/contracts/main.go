package main

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	goalmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/goalworkflow/mcp"
)

func main() { plugincontract.Publish(goalmcp.Contract) }
