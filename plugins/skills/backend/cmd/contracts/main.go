package main

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
)

func main() { plugincontract.Publish(skilllearning.Contract) }
