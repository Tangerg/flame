package main

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	planmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/planworkflow/mcp"
)

func main() { plugincontract.Publish(planmcp.Contract) }
