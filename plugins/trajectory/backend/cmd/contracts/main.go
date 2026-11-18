package main

import (
	"github.com/Tangerg/flame/packages/go-sdk/plugin"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
)

func main() { plugincontract.Publish(plugin.SessionMarkdownContract) }
